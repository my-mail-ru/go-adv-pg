package advpggen

import (
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"strconv"

	advpg "github.com/my-mail-ru/go-adv-pg"
)

// supported types:
//  - string:                      ast.BasicLit{Kind: token.STRING}
//  - int or uint of any bitsize:  ast.BasicLit{Kind: token.INT}
//  - []string:                    ast.CompositeLit
//  - []AnyTypeInVarSpec:          ast.CompositeLit
//  - bool:                        ast.Ident{Name: "true or false"}
//  - advpg.OrderDirection:        ast.SelectorExpr{"advpg.OrderAsc or advpg.OrderDesc"}
//  - any:                         Type{} or &Type{}, the type name is stored as string
//
// advpg package name is determined dynamically

type StructVarParser struct {
	specs []VarSpec
	Vars  map[string][]any // varSpec.typeName -> varSpec.new()
}

type VarSpec struct {
	TypePkg  string
	TypeName string
	New      func() any
}

type Validator interface {
	Validate() error
}

func NewStructVarParser(specs []VarSpec) StructVarParser {
	return StructVarParser{
		specs: specs,
		Vars:  make(map[string][]any, len(specs)),
	}
}

func (p StructVarParser) Parse(fset FileSet, specs []ast.Spec) error {
	for _, spec := range specs {
		valSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			return fmt.Errorf("adv-pg: %s: got %T, but *ast.ValueSpec is expected", fset.Pos(spec), spec)
		}

		for _, varVal := range valSpec.Values {
			if un, ok := varVal.(*ast.UnaryExpr); ok && un.Op == token.AND { // pointer, e.g. &Index
				varVal = un.X
			}

			lit, ok := varVal.(*ast.CompositeLit)
			if !ok {
				continue
			}

			if err := p.parseStruct(fset, valSpec.Names, lit); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p StructVarParser) parseStruct(fset FileSet, names []*ast.Ident, lit *ast.CompositeLit) error {
	typ := lit.Type
	arr, isArray := lit.Type.(*ast.ArrayType)

	if isArray {
		typ = arr.Elt
		if star, ok := typ.(*ast.StarExpr); ok { // slice of pointers, e.g. []*Index
			typ = star.X
		}
	}

	for _, varSpec := range p.specs {
		if !matchSelector(typ, varSpec.TypePkg, varSpec.TypeName) {
			continue
		}

		if isArray {
			for _, eltExpr := range lit.Elts {
				elt, ok := eltExpr.(*ast.CompositeLit)
				if !ok {
					name := "UNKNOWN"
					if len(names) != 0 {
						name = names[0].Name
					}

					return fmt.Errorf("adv-pg: %s: parsing var %s: got %T, but a struct literal is expected", fset.Pos(lit), name, eltExpr)
				}

				if err := p.parseStructFields(fset, elt, varSpec.New(), varSpec.TypePkg, varSpec.TypeName); err != nil {
					return err
				}
			}
		} else {
			if err := p.parseStructFields(fset, lit, varSpec.New(), varSpec.TypePkg, varSpec.TypeName); err != nil {
				return err
			}
		}
	}

	return nil
}

var (
	orderDirectionT = reflect.TypeOf(advpg.OrderAsc)
	anyT            = reflect.TypeOf((*any)(nil)).Elem()
)

func (p StructVarParser) parseStructFields(fset FileSet, lit *ast.CompositeLit, valPtr any, typePkg, typeName string) error {
	if err := parseStructFields(fset, lit, reflect.ValueOf(valPtr).Elem(), typePkg, typeName); err != nil {
		return err
	}

	p.Vars[typeName] = append(p.Vars[typeName], valPtr)

	return nil
}

func parseStructFields(fset FileSet, lit *ast.CompositeLit, structVal reflect.Value, typePkg, typeName string) error {
	isKeyed := true // XXX this allows passing the field count check for empty struct literals,
	// because they are treated as keyed (but they are obviously not).
	// although empty structs are syntactically ok, this looks a bit strange.
	// maybe it's better to return an error here?
	//
	// change to "false" to return "count must match" for empty struct literals.

	if len(lit.Elts) != 0 {
		_, isKeyed = lit.Elts[0].(*ast.KeyValueExpr) // Struct{Field: Value}
	}

	if !isKeyed && len(lit.Elts) != structVal.NumField() { // Struct{Value1, Value2}
		return fmt.Errorf("adv-pg: %s: %s: non-keyed struct literal values count must match struct field count", fset.Pos(lit), typeName)
	}

	for i, elt := range lit.Elts {
		field, valExpr, identName, err := getStructField(fset, elt, structVal, typePkg, typeName, i, isKeyed)
		if err != nil {
			return err
		}

		switch kind := field.Kind(); kind {
		case reflect.String:
			strVal, err := getBasicLit(fset, valExpr, token.STRING, typeName, identName)
			if err != nil {
				return err
			}

			field.SetString(strVal)

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if err := parseInt(fset, valExpr, field, typeName, identName, "uint", strconv.ParseUint, reflect.Value.SetUint); err != nil {
				return err
			}

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if field.Type() == orderDirectionT {
				if err := parseDirection(fset, valExpr, field, typePkg, typeName, identName); err != nil {
					return err
				}

				continue
			}

			if err := parseInt(fset, valExpr, field, typeName, identName, "int", strconv.ParseInt, reflect.Value.SetInt); err != nil {
				return err
			}

		case reflect.Bool:
			if err := parseBool(fset, valExpr, field, typeName, identName); err != nil {
				return err
			}

		case reflect.Slice:
			if err := parseSlice(fset, valExpr, field, typePkg, typeName, identName); err != nil {
				return err
			}

		default:
			if field.Type() == anyT {
				if err := parseTypeName(fset, valExpr, field, typeName, identName); err != nil {
					return err
				}

				continue
			}

			return fmt.Errorf("adv-pg: %s: %s.%s: internal error: unknown field kind %v", fset.Pos(valExpr), typeName, identName, kind)
		}
	}

	if validator, ok := structVal.Addr().Interface().(Validator); ok {
		if err := validator.Validate(); err != nil {
			return fmt.Errorf("adv-pg: %s: %v: validation failed: %w", fset.Pos(lit), lit.Type, err)
		}
	}

	return nil
}

func getStructField(fset FileSet, expr ast.Expr, structVal reflect.Value, typePkg, typeName string, idx int, isKeyed bool) (field reflect.Value, valExpr ast.Expr, identName string, err error) {
	if isKeyed {
		return parseKeyVal(fset, expr, structVal, typePkg, typeName)
	}

	return structVal.Field(idx), expr, structVal.Type().Field(idx).Name, nil
}

func parseKeyVal(fset FileSet, expr ast.Expr, structVal reflect.Value, typePkg, typeName string) (field reflect.Value, valExpr ast.Expr, identName string, err error) {
	kv, ok := expr.(*ast.KeyValueExpr)
	if !ok {
		return reflect.Value{}, nil, "", fmt.Errorf("adv-pg: %s: got %T, but *ast.KeyValueExpr is expected: if any element of a struct literal has a key, every element must have a key", fset.Pos(expr), expr)
	}

	ident, ok := kv.Key.(*ast.Ident)
	if !ok {
		return reflect.Value{}, nil, "", fmt.Errorf("adv-pg: %s: parsing key: got %T, but a field identifier is expected", fset.Pos(kv.Key), kv.Key)
	}

	field = structVal.FieldByName(ident.Name)
	if !field.IsValid() {
		return reflect.Value{}, nil, "", fmt.Errorf("adv-pg: %s: %s is missing from %s.%s", fset.Pos(ident), ident.Name, typePkg, typeName)
	}

	return field, kv.Value, ident.Name, nil
}

func parseInt[T any](fset FileSet, expr ast.Expr, val reflect.Value, typeName, identName, intName string, parseFunc func(string, int, int) (T, error), setFunc func(reflect.Value, T)) error {
	strVal, err := getBasicLit(fset, expr, token.INT, typeName, identName)
	if err != nil {
		return err
	}

	intVal, err := parseFunc(strVal, 10, 64) // bitsize check isn't useful here
	if err != nil {
		return fmt.Errorf("adv-pg: %s: %s.%s: error parsing %s: %w", fset.Pos(expr), typeName, identName, intName, err)
	}

	setFunc(val, intVal)

	return nil
}

func parseDirection(fset FileSet, expr ast.Expr, val reflect.Value, typePkg, typeName, identName string) error {
	switch {
	case matchSelector(expr, typePkg, "OrderAsc"):
		val.SetInt(int64(advpg.OrderAsc)) // set the default value for clarity
	case matchSelector(expr, typePkg, "OrderDesc"):
		val.SetInt(int64(advpg.OrderDesc))
	default:
		return fmt.Errorf("adv-pg: %s: %s.%s: unknown OrderDirection: %v", fset.Pos(expr), typeName, identName, expr)
	}

	return nil
}

func parseBool(fset FileSet, expr ast.Expr, val reflect.Value, typeName, identName string) error {
	boolIdent, ok := expr.(*ast.Ident)
	if !ok {
		return fmt.Errorf("adv-pg: %s: %s.%s: parsing bool: got %T, but true or false identifier is expected", fset.Pos(expr), typeName, identName, expr)
	}

	switch boolIdent.Name {
	case "true":
		val.SetBool(true)
	case "false":
		val.SetBool(false) // set the default value for clarity
	default:
		return fmt.Errorf("adv-pg: %s: %s.%s: error parsing bool: got %s, expected true or false", fset.Pos(expr), typeName, identName, boolIdent.Name)
	}

	return nil
}

func parseSlice(fset FileSet, expr ast.Expr, val reflect.Value, typePkg, typeName, identName string) error {
	elKind := val.Type().Elem().Kind()
	if elKind != reflect.String && elKind != reflect.Struct {
		return fmt.Errorf("adv-pg: %s: %s.%s: internal error: only the string and struct slices are supported", fset.Pos(expr), typeName, identName)
	}

	sliceLit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return fmt.Errorf("adv-pg: %s: %s.%s: parsing %v slice: got %T, but slice literal is expected", fset.Pos(expr), typeName, identName, elKind, expr)
	}

	sl := reflect.MakeSlice(val.Type(), len(sliceLit.Elts), len(sliceLit.Elts))

	for i, elt := range sliceLit.Elts {
		slElt := sl.Index(i)

		if elKind == reflect.String {
			s, err := getBasicLit(fset, elt, token.STRING, typeName, identName)
			if err != nil {
				return err
			}

			slElt.SetString(s)
		} else {
			structLit, ok := elt.(*ast.CompositeLit)
			if !ok {
				return fmt.Errorf("adv-pg: %s: %s.%s: parsing %v slice element #%d: got %T, but a struct literal is expected", fset.Pos(expr), typeName, identName, elKind, i, elt)
			}

			if err := parseStructFields(fset, structLit, slElt, typePkg, typeName+"."+identName); err != nil {
				return err
			}
		}
	}

	val.Set(sl)

	return nil
}

func parseTypeName(fset FileSet, expr ast.Expr, val reflect.Value, typeName, identName string) error {
	var (
		lit *ast.CompositeLit
		ok  bool
	)

	switch anyVal := expr.(type) {
	case *ast.UnaryExpr: // the table can be declared using a pointer to its type....
		if anyVal.Op != token.AND {
			return fmt.Errorf("adv-pg: %s: %s.%s: parsing `any` val: got %v, but & is expected", fset.Pos(expr), typeName, identName, anyVal.Op)
		}

		lit, ok = anyVal.X.(*ast.CompositeLit)
		if !ok {
			return fmt.Errorf("adv-pg: %s: %s.%s: parsing `any` val: got %T, but *ast.CompositeLit is expected", fset.Pos(expr), typeName, identName, anyVal.X)
		}
	case *ast.CompositeLit: // ... or by value
		lit = anyVal
	default:
		return fmt.Errorf("adv-pg: %s: %s.%s: parsing `any` val: got %T, but *ast.UnaryExpr(&) or *ast.CompositeLit, e.g. &TableName{} or TableName{} is expected", fset.Pos(expr), typeName, identName, expr)
	}

	goType, ok := lit.Type.(*ast.Ident)
	if !ok {
		return fmt.Errorf("adv-pg: %s: %s.%s: parsing any val: got %T, but ast.Ident (i.e. a simple type name) is expected", fset.Pos(expr), typeName, identName, expr)
	}

	val.Set(reflect.ValueOf(goType.Name))

	return nil
}

func getBasicLit(fset FileSet, expr ast.Expr, kind token.Token, typeName, fieldName string) (string, error) {
	blit, ok := expr.(*ast.BasicLit)
	if !ok {
		return "", fmt.Errorf("adv-pg: %s: %s.%s: got %T, but a %v literal is expected", fset.Pos(expr), typeName, fieldName, expr, kind)
	}

	if blit.Kind != kind {
		return "", fmt.Errorf("adv-pg: %s: %s.%s: got %v, but %v is expected", fset.Pos(expr), typeName, fieldName, blit.Kind, kind)
	}

	if kind != token.STRING {
		return blit.Value, nil
	}

	strVal, err := strconv.Unquote(blit.Value)
	if err != nil {
		return "", fmt.Errorf("adv-pg: %s: %s.%s: unquote: %w", fset.Pos(expr), typeName, fieldName, err)
	}

	return strVal, nil
}
