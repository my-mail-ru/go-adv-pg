//go:build !integration

package advpggen_test

import (
	_ "embed"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	advpg "github.com/my-mail-ru/go-adv-pg"
	advpggen "github.com/my-mail-ru/go-adv-pg/internal/gen"
)

var _ = 123 // cover a non-composite var check

var singleIndex = advpg.Index{
	Selector:        "SelectByPrimaryKey",
	Deleter:         "DeleteByPrimaryKey",
	Keys:            []string{"ID", "ExtID"},
	DisableSelector: false,
	DisableDeleter:  false,
	IsMulti:         true,
	IsPrimaryKey:    true,
	IsUniq:          true,
	DefaultLimit:    1,
	Order: []advpg.Order{
		{Field: "created_at", Order: advpg.OrderAsc},
		{"id", advpg.OrderDesc}, //nolint:govet
	},
	Condition: "status != 0",
}

var singleIdxByPointer = &advpg.Index{
	Selector:     "SelectTest",
	Keys:         []string{"ID", "Test"},
	IsPrimaryKey: true,
}

var (
	multiIndex = []advpg.Index{{
		Selector: "SelectByStatus",
		Deleter:  "DeleteByStatus",
		Keys:     []string{"status"},
		IsMulti:  true,
	}, {
		Selector: "SelectByName",
		Keys:     []string{"name"},
		IsMulti:  true,
	}}

	multiIdxByPointer = []*advpg.Index{{
		Selector: "SelectMultiByID",
		Keys:     []string{"id"},
	}, {
		Deleter: "DeleteByStatus",
		Keys:    []string{"status"},
		IsMulti: true,
	}}

	fieldDesc = []advpg.Field{{
		Field:   "nullable",
		SQLScan: "COALESCE(nullable, 'default')",
	}, {
		Field:          "counter",
		EnableMutators: true,
	}}
)

type MyTable struct{}

var tableDecl = advpg.Table{
	Model:            MyTable{},
	Table:            "my_table",
	UpdateOnConflict: true,
	Indices: []advpg.Index{{
		Selector: "SelectMultiByID",
		Keys:     []string{"id"},
	}, {
		Deleter: "DeleteByStatus",
		Keys:    []string{"status"},
		IsMulti: true,
	}},
	Fields: []advpg.Field{{
		Field:   "nullable",
		SQLScan: "COALESCE(nullable, 'default')",
	}, {
		Field:          "counter",
		EnableMutators: true,
	}},
}

//go:embed parse_var_test.go
var structVarParserTestSrc string

func TestStructVarParser(t *testing.T) {
	p := advpggen.NewStructVarParser([]advpggen.VarSpec{{
		TypePkg:  "advpg",
		TypeName: "Table",
		New: func() any {
			return &advpg.Table{}
		},
	}, {
		TypePkg:  "advpg",
		TypeName: "Index",
		New: func() any {
			return &advpg.Index{}
		},
	}, {
		TypePkg:  "advpg",
		TypeName: "Field",
		New: func() any {
			return &advpg.Field{}
		},
	}})

	if err := parseFile(t, structVarParserTestSrc, p); err != nil {
		t.Fatal("StructVarParser.Parse:", err)
	}

	want := map[string][]any{
		"Table": []any{&tableDecl},
		"Index": []any{&singleIndex, singleIdxByPointer, &multiIndex[0], &multiIndex[1], multiIdxByPointer[0], multiIdxByPointer[1]},
		"Field": []any{&fieldDesc[0], &fieldDesc[1]},
	}

	if diff := cmp.Diff(want, p.Vars, cmpOptsTableModel...); diff != "" {
		t.Error("unexpected difference (-want +got)\n" + diff)
	}
}

func parseFile(t *testing.T, src string, p advpggen.StructVarParser) error {
	fset := advpggen.NewFileSet()

	f, err := parser.ParseFile(fset.FileSet, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}

		if err := p.Parse(fset, genDecl.Specs); err != nil {
			return err
		}
	}

	return nil
}

func TestStructVarParserErrors(t *testing.T) {
	preamble := "package test\nvar _ = advpg.Index{\n"
	preambleTable := "package test\n var _ = advpg.Table{\n"

	tests := []struct {
		name    string
		src     string
		wantErr string
	}{{
		name:    "field count mismatch",
		src:     preamble + "1}",
		wantErr: "non-keyed struct literal values count must match struct field count",
	}, {
		name:    "if any element has a key, every element must have a key",
		src:     preamble + "DisableSelector: true, 1}",
		wantErr: "every element must have a key",
	}, {
		name:    "key is not an identifier",
		src:     preamble + "123: 123}",
		wantErr: "field identifier is expected",
	}, {
		name:    "missing field",
		src:     preamble + "Foobar: 123}",
		wantErr: "Foobar is missing",
	}, {
		name:    "not a string",
		src:     preamble + "Selector: 123}",
		wantErr: "got INT, but STRING is expected",
	}, {
		name:    "not an int",
		src:     preamble + "DefaultLimit: 1.23}",
		wantErr: "got FLOAT, but INT is expected",
	}, {
		name:    "int overflow",
		src:     preamble + "DefaultLimit: 11111111111111111111111}",
		wantErr: "value out of range",
	}, {
		name:    "unknown direction",
		src:     preamble + "Order: []advpg.Order{{Order: Foobar}}}",
		wantErr: "unknown Order",
	}, {
		name:    "bad struct slice literal",
		src:     preamble + "Order: []advpg.Order{123}}",
		wantErr: "struct literal is expected",
	}, {
		name:    "bad bool type",
		src:     preamble + "IsMulti: 1}",
		wantErr: "parsing bool",
	}, {
		name:    "bad bool value",
		src:     preamble + "IsMulti: foobar}",
		wantErr: "parsing bool",
	}, {
		name:    "bad string slice type",
		src:     preamble + "Keys: 123}",
		wantErr: "parsing string slice",
	}, {
		name:    "bad string slice element",
		src:     preamble + "Keys: []string{1,2,3}}",
		wantErr: "got INT, but STRING is expected",
	}, {
		name:    "bad basic literal",
		src:     preamble + "Selector: Test}",
		wantErr: "STRING literal is expected",
	}, {
		name:    "slice is not a composite literal",
		src:     "package test\nvar _ = []advpg.Index{123}",
		wantErr: "struct literal is expected",
	}, {
		name:    "error inside a composite literal",
		src:     "package test\nvar _ = []advpg.Index{{Keys: 123}}",
		wantErr: "slice literal is expected",
	}, {
		name:    "not a string slice",
		src:     "package test\nvar _ = test.IntSlice{Ints: []int{1,2,3}}",
		wantErr: "internal error",
	}, {
		name:    "unknown kind",
		src:     "package test\nvar _ = test.UnknownKind{F: 3.14}",
		wantErr: "internal error",
	}, {
		name:    "unknown table literal type",
		src:     preambleTable + "Model: 123}",
		wantErr: "TableName{} is expected",
	}, {
		name:    "unknown table literal pointer type",
		src:     preambleTable + "Model: &1}",
		wantErr: "*ast.CompositeLit is expected",
	}, {
		name:    "unknown unary operation",
		src:     preambleTable + "Model: +1}",
		wantErr: "& is expected",
	}, {
		name:    "not a simple type name",
		src:     preambleTable + "Model: pkg.Type{}}",
		wantErr: "ast.Ident (i.e. a simple type name) is expected",
	}, {
		name:    "Table validation failed",
		src:     preambleTable + "}",
		wantErr: "validation failed: missing Model",
	}, {
		name:    "Index validation failed",
		src:     "package test\nvar _ = []advpg.Index{{}}",
		wantErr: "validation failed: missing Keys",
	}, {
		name:    "field validation failed",
		src:     "package test\nvar _ = []advpg.Field{{}}",
		wantErr: "validation failed: missing Field",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := advpggen.NewStructVarParser([]advpggen.VarSpec{{
				TypePkg:  "advpg",
				TypeName: "Table",
				New: func() any {
					return &advpg.Table{}
				},
			}, {
				TypePkg:  "advpg",
				TypeName: "Index",
				New: func() any {
					return &advpg.Index{}
				},
			}, {
				TypePkg:  "advpg",
				TypeName: "Field",
				New: func() any {
					return &advpg.Field{}
				},
			}, {
				TypePkg:  "test",
				TypeName: "IntSlice",
				New: func() any {
					return &struct {
						Ints []int
					}{}
				},
			}, {
				TypePkg:  "test",
				TypeName: "UnknownKind",
				New: func() any {
					return &struct {
						F float32
					}{}
				},
			}})

			err := parseFile(t, tt.src, p)
			if err == nil {
				t.Fatal("got no error, but an error is expected:", tt.wantErr)
			}

			gotErr := err.Error()
			if !strings.Contains(gotErr, tt.wantErr) {
				t.Fatalf("got an unexpected error %v.\n%q is expected", err, tt.wantErr)
			}
		})
	}
}
