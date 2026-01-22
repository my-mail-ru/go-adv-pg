// isn't supported right now:
//  - struct embedding (must have)
//  - DELETE RETURNING

package advpggen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ettle/strcase"

	advpg "github.com/my-mail-ru/go-adv-pg"
)

const (
	advPgImport         = `"github.com/my-mail-ru/go-adv-pg"`
	advPgDefaultPkgName = "advpg"
	testSuffix          = "_test"
	generatedSuffix     = "_generated"
)

func Parse(fsys fs.FS, fname string) (File, error) {
	src, err := fs.ReadFile(fsys, fname)
	if err != nil {
		return File{}, fmt.Errorf("adv-pg: error reading %s: %w", fname, err)
	}

	fset := NewFileSet()

	f, err := parser.ParseFile(fset.FileSet, fname, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return File{}, err
	}

	destFileName := generatedFileName(fname)

	daoInfo, err := ParseDAO(fset, NewExcludeFS(fsys, destFileName))
	if err != nil {
		return File{}, err
	}

	ret := File{
		DestFileName: destFileName,
		Directives:   getPackageDirectives(f.Comments, f.Package),
		Package:      f.Name.Name,
		Models:       make([]*TableModel, 0, 1),
		ModelsByName: make(map[string]*TableModel, 1),
	}

	if err := ret.processImports(fset, f.Imports); err != nil {
		return File{}, err
	}

	vars := newIndexFieldVars(ret.AdvPgPkg)

	// parse vars before parsing types
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}

		if err := vars.Parse(fset, genDecl.Specs); err != nil {
			return File{}, err
		}
	}

	tables := vars.Vars["Table"]
	if len(tables) == 0 {
		return File{}, errors.New("adv-pg: no Table is declared in file")
	}

	tablesByGoName := make(map[string]*advpg.Table, len(tables)) // key is TableModel.GoName
	for _, tableI := range tables {
		table, ok := tableI.(*advpg.Table)
		if !ok {
			return File{}, fmt.Errorf("adv-pg: internal error: got %T, but advpg.Table is expected", tableI)
		}

		goName, ok := table.Model.(string)
		if !ok {
			return File{}, fmt.Errorf("adv-pg: internal error: Model name is %T, but string is expected", table.Model)
		}

		tablesByGoName[goName] = table
	}

	// parse types
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		if err := ret.parseTypeSpecs(fset, tablesByGoName, genDecl); err != nil {
			return File{}, err
		}
	}

	if err := ret.fillModels(daoInfo); err != nil {
		return File{}, err
	}

	return ret, nil
}

func newIndexFieldVars(pkg string) StructVarParser {
	return NewStructVarParser([]VarSpec{{
		TypePkg:  pkg,
		TypeName: "Table",
		New: func() any {
			return &advpg.Table{}
		},
	}})
}

func (f *File) processImports(fset FileSet, imports []*ast.ImportSpec) error {
	f.Imports = make([]ImportSpec, len(imports), len(imports)+1)

	for i, imp := range imports {
		if imp.Path.Kind != token.STRING {
			return fmt.Errorf("adv-pg: %s: invalid import kind: got %s, but token.STRING is expected", fset.Pos(imp), imp.Path.Kind)
		}

		pkgName := ""
		if imp.Name != nil && imp.Name.Name != "_" {
			pkgName = imp.Name.Name
		}

		f.Imports[i] = ImportSpec{
			PkgName: pkgName, // TODO support dot-imports
			PkgPath: imp.Path.Value,
		}

		if imp.Path.Value == advPgImport {
			if pkgName != "" && pkgName != "_" {
				f.AdvPgPkg = pkgName
			} else {
				f.AdvPgPkg = advPgDefaultPkgName
			}
		}
	}

	if f.AdvPgPkg == "" { // missing adv-pg import - add it explicitly
		f.AdvPgPkg = advPgDefaultPkgName

		f.Imports = append(f.Imports, ImportSpec{
			PkgName: advPgDefaultPkgName,
			PkgPath: advPgImport,
		})
	}

	return nil
}

func (f *File) fillModels(daoInfo DAOInfo) (err error) {
	for _, model := range f.Models {
		if model.Table.Table == "" {
			model.Table.Table = strcase.ToSnake(model.GoName)
		}

		if model.UpdateOnConflict && model.OnConflictDoNothing {
			return fmt.Errorf("adv-pg: %s: UpdateOnConflict and OnConflictDoNothing are mutually exclusive", model.GoName)
		}

		if err = model.setFields(model.Fields); err != nil {
			return err
		}

		model.DAO, model.NeedGeneratedDAO, model.HasPackageDAO, err = daoInfo.Get(model.DAO, model.GoName)
		if err != nil {
			return err
		}

		if err = model.linkIndicesToColumns(); err != nil {
			return err
		}

		if err = model.fillColumns(); err != nil {
			return err
		}

		if len(model.SetterColumns) > 64 {
			return fmt.Errorf("adv-pg: %s: up to 64 updateable columns are currently supported", model.GoName)
		}

		if (model.UpdateOnConflict || model.OnConflictDoNothing) && len(model.PrimaryKeyColumns) == 0 {
			return fmt.Errorf("adv-pg: %s: UpdateOnConflict and OnConflictDoNothing require a primary key", model.GoName)
		}

		if model.UpdateOnConflict && len(model.UpdateValueColumns) == 0 && len(model.MutatorColumns) == 0 {
			model.UpdateOnConflict = false
			model.OnConflictDoNothing = true
		}
	}

	return nil
}

func (tm *TableModel) setFields(fields []advpg.Field) error {
	for i, field := range fields {
		col, err := tm.colByName(field.Field)
		if err != nil {
			return err
		}

		col.Field = &fields[i]
	}

	return nil
}

// as with fields, indices can be referenced by a GoName or a ColumnName.
// advpg.Index.IsPrimaryKey is promoted to all the [Column]s referenced by an index.
func (tm *TableModel) linkIndicesToColumns() error {
	dupMethods := make(dupMethodMap, 2*len(tm.Indices))

	for i, idx := range tm.Indices {
		idxCols := make([]*Column, len(idx.Keys))

		for j, key := range idx.Keys {
			col, err := tm.colByName(key)
			if err != nil {
				return err
			}

			idxCols[j] = col
		}

		if idx.IsPrimaryKey {
			if len(tm.PrimaryKeyColumns) != 0 {
				return fmt.Errorf("adv-pg: %s: multiple primary key indices are specified", tm.GoName)
			}

			tm.PrimaryKeyColumns = idxCols
			idx.IsUniq = true

			for _, col := range idxCols {
				col.IsPrimaryKey = true
			}
		}

		if idx.Name == "" {
			idx.Name = getDefaultIndexName(&idx, idxCols) // passing by pointer here doesn't require modern go for semantic
		}

		if idx.DisableSelector {
			idx.Selector = ""
		} else if idx.Selector == "" {
			idx.Selector = tm.indexMethodName("Select", &idx)
		}

		if idx.DisableDeleter {
			idx.Deleter = ""
		} else if idx.Deleter == "" {
			idx.Deleter = tm.indexMethodName("Delete", &idx)
		}

		if err := dupMethods.checkAndSet(idx.Keys, idx.Selector, idx.Deleter); err != nil {
			return err
		}

		tm.Indices[i] = idx
	}

	return nil
}

type dupMethodMap map[string][]string

func (d dupMethodMap) checkAndSet(keys []string, methods ...string) error {
	for _, method := range methods {
		if method == "" {
			continue
		}

		if oldKeys, ok := d[method]; ok {
			return fmt.Errorf("adv-pg: method name %s is already used for index %#v", method, oldKeys)
		}

		d[method] = keys
	}

	return nil
}

func (tm *TableModel) fillColumns() error {
	columnConflict := make(map[string]struct{}, len(tm.Columns))

	for _, col := range tm.Columns {
		if col.Field == nil {
			col.Field = &advpg.Field{}
		}

		if tm.DisableActiveRecord {
			col.GoExpr = "model." + col.GoName
		} else {
			col.GoExpr = "model.data." + col.GoName
		}

		if col.EnableMutators {
			if len(tm.PrimaryKeyColumns) == 0 {
				return fmt.Errorf("adv-pg: %s.%s: mutators require a primary key to be defined", tm.GoName, col.GoName)
			}

			if tm.DisableActiveRecord {
				return fmt.Errorf("adv-pg: %s.%s: mutators require ActiveRecord but it is disabled for the table", tm.GoName, col.GoName)
			}

			if col.DisableUpdate {
				return fmt.Errorf("adv-pg: %s.%s: DisableUpdate is incompatible with EnableMutators", tm.GoName, col.GoName)
			}

			if col.IsPrimaryKey {
				return fmt.Errorf("adv-pg: %s.%s: IsPrimaryKey is incompatible with EnableMutators", tm.GoName, col.GoName)
			}

			col.UpdateByStorage = true
			tm.MutatorColumns = append(tm.MutatorColumns, col)
		}

		if tm.UpdateOnConflict && col.IsPrimaryKey && (col.InitByStorage || col.UpdateByStorage) {
			return fmt.Errorf("adv-pg: %s.%s: UpdateOnConflict may not be used with tables with InitByStorage primary keys", tm.GoName, col.GoName)
		}

		if col.SQLValue == "" { // when SQLValue is enabled, DB column names are allowed to repeat
			if _, ok := columnConflict[col.ColumnName]; ok {
				return fmt.Errorf("adv-pg: %s.%s: column name %q may be specified multiple times only when SQLValue is used", tm.GoName, col.GoName, col.ColumnName)
			}

			columnConflict[col.ColumnName] = struct{}{}
		} else if col.InitByStorage && col.DisableUpdate {
			return fmt.Errorf("adv-pg: %s.%s: SQLValue is useless when InitByStorage and DisableUpdate are both on", tm.GoName, col.GoName)
		}

		needSetter := false

		if !col.DisableInsert {
			if col.InitByStorage {
				tm.InsertResultColumns = append(tm.InsertResultColumns, col)
			} else {
				if col.EnableMutators && tm.UpdateOnConflict {
					tm.InsertResultColumns = append(tm.InsertResultColumns, col)
				}

				if !col.EnableMutators {
					// while a setter isn't generated for a mutator column,
					// initial insert is allowed, if InitByStorage is off.
					needSetter = true
				}

				tm.InsertValueColumns = append(tm.InsertValueColumns, col)
			}
		}

		if len(tm.PrimaryKeyColumns) != 0 && !col.DisableUpdate && !col.IsPrimaryKey {
			if col.UpdateByStorage {
				tm.UpdateResultColumns = append(tm.UpdateResultColumns, col)
			} else {
				tm.UpdateValueColumns = append(tm.UpdateValueColumns, col)
				needSetter = true
			}
		}

		if needSetter && !tm.DisableActiveRecord && !col.EnableMutators {
			tm.SetterColumns = append(tm.SetterColumns, col)
		}
	}

	return nil
}

func (tm *TableModel) colByName(name string) (*Column, error) {
	if col, ok := tm.ColumnsByGoName[name]; ok {
		return col, nil
	}

	if col, ok := tm.ColumnsByName[name]; ok {
		return col, nil
	}

	return nil, fmt.Errorf("adv-pg: %s: unknown column name %q", tm.GoName, name)
}

func getDefaultIndexName(idx *advpg.Index, idxCols []*Column) string {
	var sb strings.Builder

	if idx.IsPrimaryKey && len(idx.Keys) > 1 {
		sb.WriteString("PrimaryKey")
	} else {
		for _, col := range idxCols {
			sb.WriteString(strcase.ToGoPascal(col.GoName))
		}
	}

	return sb.String()
}

func (tm *TableModel) indexMethodName(prefix string, idx *advpg.Index) string {
	var sb strings.Builder

	sb.WriteString(prefix)

	if idx.IsMulti {
		sb.WriteString("Multi")
	}

	if tm.HasPackageDAO {
		sb.WriteString(tm.GoName)
	}

	sb.WriteString("By")
	sb.WriteString(idx.Name)

	return sb.String()
}

type FileSet struct {
	*token.FileSet
}

func NewFileSet() FileSet {
	return FileSet{FileSet: token.NewFileSet()}
}

func (f FileSet) Pos(n ast.Node) string {
	return f.Position(n.Pos()).String()
}

// relaxed version of ast.isDirective
var directiveRegexp = regexp.MustCompile(`^//\S`)

func getPackageDirectives(groups []*ast.CommentGroup, limit token.Pos) []string {
	ret := []string{}

	for _, comments := range groups {
		for _, comment := range comments.List {
			if comment.Slash >= limit {
				return ret
			}

			if directiveRegexp.MatchString(comment.Text) && !strings.HasPrefix(comment.Text, "//go:generate") {
				ret = append(ret, comment.Text)
			}
		}
	}

	return ret
}

// matchSelector matches fully qualified type name like pkg.Type.
// NB "selector" here is a dot-separated ("fully-qualified") name inside a package,
// not a "SelectBy..." method.
func matchSelector(expr ast.Expr, pkg, name string) bool {
	typ, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	ident, ok := typ.X.(*ast.Ident)
	if !ok {
		return false
	}

	return ident.Name == pkg && typ.Sel.Name == name
}

func printNode(fset FileSet, node any) (string, error) {
	buf := &strings.Builder{}

	if err := printer.Fprint(buf, fset.FileSet, node); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func generatedFileName(fname string) string {
	suffix := len(fname) - len(filepath.Ext(fname))

	if strings.HasSuffix(fname[:suffix], testSuffix) {
		suffix -= len(testSuffix)
	}

	return fname[:suffix] + generatedSuffix + fname[suffix:]
}
