package advpggen

import (
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"strconv"

	"github.com/ettle/strcase"
)

// parseTypeSpecs parses type declarations for types mentioned in ModelsByName.
func (f *File) parseTypeSpecs(fset FileSet, genDecl *ast.GenDecl) error {
	for _, spec := range genDecl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok || typeSpec.Name == nil {
			continue
		}

		goName := typeSpec.Name.Name

		model, ok := f.ModelsByName[goName]
		if !ok {
			continue
		}

		if typeSpec.Assign != token.NoPos {
			return fmt.Errorf("adv-pg: %s: %s: type alias cannot be used as a table schema", fset.Pos(genDecl), goName)
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return fmt.Errorf("adv-pg: %s: %s: got %T, but a struct type declaration is expected", fset.Pos(genDecl), goName, typeSpec.Type)
		}

		if structType.Fields == nil || len(structType.Fields.List) == 0 {
			return fmt.Errorf("adv-pg: %s: %s: the struct has no fields", fset.Pos(genDecl), goName)
		}

		if model.IsImplicit {
			return fmt.Errorf("adv-pg: %s: %s: explicitly declared table models must be specified by a struct literal syntax (i.e. Model: %s{}), not as a string (Model: %q)", fset.Pos(genDecl), goName, goName, goName)
		}

		cols, err := parseColumns(fset, goName, structType.Fields.List)
		if err != nil {
			return err
		}

		model.Columns = cols
		model.ColumnsByGoName = make(map[string]*Column, len(cols))
		model.ColumnsByName = make(map[string]*Column, len(cols))

		for _, col := range cols {
			model.ColumnsByGoName[col.GoName] = col
			model.ColumnsByName[col.ColumnName] = col
		}

		f.Models = append(f.Models, model)
	}

	return nil
}

// parseColumns sets everything but the *advpg.Field
func parseColumns(fset FileSet, tableTypeName string, fields []*ast.Field) ([]*Column, error) {
	ret := make([]*Column, 0, len(fields))

	for _, field := range fields {
		dbTag, err := parseTag(field.Tag, "db")
		if err != nil {
			return nil, fmt.Errorf("adv-pg: %s: %w", fset.Pos(field), err)
		}

		if dbTag == "-" {
			continue
		}

		if len(field.Names) == 0 {
			return nil, fmt.Errorf("adv-pg: %s: %s: embedded types aren't supported yet", fset.Pos(field), tableTypeName)
		}

		if len(field.Names) > 1 {
			return nil, fmt.Errorf("adv-pg: %s: %s %v: each field must be declared separately", fset.Pos(field), tableTypeName, field.Names)
		}

		col := &Column{
			GoName:     field.Names[0].Name,
			ColumnName: dbTag,
		}

		if dbTag == "" {
			col.ColumnName = strcase.ToSnake(col.GoName)
		}

		col.GoType, err = printNode(fset, field.Type)
		if err != nil {
			return nil, fmt.Errorf("adv-pg: %s: failed to print a go type name: %w", fset.Pos(field), err)
		}

		ret = append(ret, col)
	}

	return ret, nil
}

func parseTag(tag *ast.BasicLit, key string) (string, error) {
	if tag == nil {
		return "", nil
	}

	if tag.Kind != token.STRING {
		return "", fmt.Errorf("parseTag: got %v, but a string is expected", tag.Kind)
	}

	val, err := strconv.Unquote(tag.Value)
	if err != nil {
		return "", fmt.Errorf("parseTag: %w", err)
	}

	return reflect.StructTag(val).Get(key), nil
}
