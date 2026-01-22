package advpggen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strconv"
)

const ConstPackageDAO = "PackageDAO"

type excludeFS struct {
	fs.FS
	exclude string
}

var _ fs.GlobFS = &excludeFS{}

func NewExcludeFS(fsys fs.FS, exclude string) fs.FS {
	return &excludeFS{FS: fsys, exclude: exclude}
}

func (efs *excludeFS) Glob(pattern string) ([]string, error) {
	matches, err := fs.Glob(efs.FS, pattern)
	if err != nil {
		return nil, err
	}

	i := 0

	for i < len(matches) {
		if matches[i] == efs.exclude {
			l := len(matches) - 1
			if i == l {
				return matches[:l], nil
			}

			matches[i] = matches[l]
			matches = matches[:l]
		} else {
			i++
		}
	}

	return matches, nil
}

type AllTypes map[string]bool

type DAOInfo struct {
	AllTypes   AllTypes
	PackageDAO string
}

func ParseDAO(fset FileSet, fsys fs.FS) (DAOInfo, error) {
	goFiles, err := fs.Glob(fsys, "*.go")
	if err != nil {
		return DAOInfo{}, fmt.Errorf(`adv-pg: Glob("*.go"): %w`, err)
	}

	ret := DAOInfo{
		AllTypes: make(AllTypes),
	}

	for _, goFile := range goFiles {
		src, err := fs.ReadFile(fsys, goFile)
		if err != nil {
			return DAOInfo{}, fmt.Errorf("adv-pg: error reading %s: %w", goFile, err)
		}

		if err = ret.parseDAOFile(fset, goFile, src); err != nil {
			return DAOInfo{}, err
		}
	}

	return ret, nil
}

func (di *DAOInfo) parseDAOFile(fset FileSet, goFile string, src []byte) error {
	f, err := parser.ParseFile(fset.FileSet, goFile, src, parser.SkipObjectResolution)
	if err != nil {
		return err
	}

	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}

		switch genDecl.Tok {
		case token.TYPE:
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					return fmt.Errorf("adv-pg: %s: internal error: got %T, but *ast.TypeSpec was expected", fset.Pos(spec), spec)
				}

				if typeSpec.Name == nil || typeSpec.Assign != token.NoPos {
					continue
				}

				_, isStruct := typeSpec.Type.(*ast.StructType)
				di.AllTypes[typeSpec.Name.Name] = isStruct
			}
		case token.CONST:
			for _, spec := range genDecl.Specs {
				found, err := di.parseDAOConst(fset, spec)
				if err != nil {
					return err
				}

				if found {
					break
				}
			}
		}
	}

	return nil
}

func (di *DAOInfo) parseDAOConst(fset FileSet, spec ast.Spec) (bool, error) {
	valSpec, ok := spec.(*ast.ValueSpec)

	if !ok {
		return false, fmt.Errorf("adv-pg: %s: internal error: got %T, but *ast.ValueSpec was expected", fset.Pos(spec), spec)
	}

	for i, name := range valSpec.Names {
		if name.Name != ConstPackageDAO {
			continue
		}

		if len(valSpec.Values) <= i {
			return false, fmt.Errorf("adv-pg: %s: const "+ConstPackageDAO+" should have a value", fset.Pos(valSpec))
		}

		expr := valSpec.Values[i]

		blit, ok := expr.(*ast.BasicLit)
		if !ok {
			return false, fmt.Errorf("adv-pg: %s: const "+ConstPackageDAO+" must be a simple string literal, not %T", fset.Pos(valSpec), expr)
		}

		if blit.Kind != token.STRING {
			return false, fmt.Errorf("adv-pg: %s: const "+ConstPackageDAO+" must be a simple string literal, not %v", fset.Pos(valSpec), blit.Kind)
		}

		str, err := strconv.Unquote(blit.Value)
		if err != nil {
			return false, fmt.Errorf("adv-pg: %s: const "+ConstPackageDAO+" must be a quoted string", fset.Pos(valSpec))
		}

		di.PackageDAO = str

		return true, nil
	}

	return false, nil
}

func (di DAOInfo) Get(dao, goName string) (daoName string, needGenerate, isPackageDAO bool, err error) {
	if dao == "" {
		if di.PackageDAO != "" {
			dao = di.PackageDAO
			isPackageDAO = true
		} else {
			dao = goName + "DAO"
		}
	}

	isStruct, hasDAO := di.AllTypes[dao]
	if !hasDAO {
		return dao, true, isPackageDAO, nil
	}

	if !isStruct {
		return "", false, false, errors.New("adv-pg: " + dao + " type isn't a struct")
	}

	return dao, false, isPackageDAO, nil
}
