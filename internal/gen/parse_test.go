//go:build !integration

package advpggen_test

import (
	"bytes"
	_ "embed"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	advpg "github.com/my-mail-ru/go-adv-pg"
	advpggen "github.com/my-mail-ru/go-adv-pg/internal/gen"
)

//go:embed parse_schema_test.go
var testSource string

func testFile(models []*advpggen.TableModel, noPkgName ...bool) advpggen.File {
	pkgName := "advpg"
	if len(noPkgName) != 0 && noPkgName[0] {
		pkgName = ""
	}

	return advpggen.File{
		DestFileName: "test_generated.go",
		Package:      "advpggen_test",
		AdvPgPkg:     "advpg",
		AdvPgConnPkg: "advpgconn",
		Directives:   []string{"//foo:bar test"},
		Imports: []advpggen.ImportSpec{
			{PkgPath: `"time"`},
			{PkgName: pkgName, PkgPath: `"github.com/my-mail-ru/go-adv-pg"`},
			{PkgName: "advpgconn", PkgPath: `"github.com/my-mail-ru/go-adv-pg/conn"`},
		},
		Models: models,
	}
}

func copyCol(col *advpggen.Column) *advpggen.Column {
	ret := *col
	return &ret
}

func TestParse(t *testing.T) {
	colID := &advpggen.Column{
		GoName:       "ID",
		GoExpr:       "model.ID",
		GoType:       "int",
		ColumnName:   "id",
		IsPrimaryKey: true,
		Field:        &advpg.Field{},
	}

	colIDNoPK := copyCol(colID)
	colIDNoPK.IsPrimaryKey = false

	colARID := func(f *advpg.Field) *advpggen.Column {
		ret := copyCol(colID)
		ret.GoExpr = "model.data.ID"
		if f == nil {
			ret.Field = &advpg.Field{}
		} else {
			ret.Field = f
		}

		return ret
	}

	colType := &advpggen.Column{
		GoName:     "Type",
		GoExpr:     "model.Type",
		GoType:     "int",
		ColumnName: "type",
		Field:      &advpg.Field{},
	}

	colTypePK := copyCol(colType)
	colTypePK.IsPrimaryKey = true

	colARType := copyCol(colType)
	colARType.GoExpr = "model.data.Type"

	colCreatedAt := func(f *advpg.Field, isAR bool) *advpggen.Column {
		if f == nil {
			f = &advpg.Field{}
		}

		ret := &advpggen.Column{
			GoName:     "CreatedAt",
			GoType:     "time.Time",
			ColumnName: "created_at",
			Field:      f,
		}

		if isAR {
			ret.GoExpr = "model.data.CreatedAt"
		} else {
			ret.GoExpr = "model.CreatedAt"
		}

		return ret
	}

	colUpdatedAt := func(f *advpg.Field, isAR bool) *advpggen.Column {
		if f == nil {
			f = &advpg.Field{}
		}

		ret := &advpggen.Column{
			GoName:     "UpdatedAt",
			GoType:     "time.Time",
			ColumnName: "updated_at",
			Field:      f,
		}

		if isAR {
			ret.GoExpr = "model.data.UpdatedAt"
		} else {
			ret.GoExpr = "model.UpdatedAt"
		}

		return ret
	}

	colDescr := func(f *advpg.Field, isAR bool) *advpggen.Column {
		if f == nil {
			f = &advpg.Field{}
		}

		ret := &advpggen.Column{
			GoName:     "Descr",
			GoType:     "string",
			ColumnName: "descr",
			Field:      f,
		}

		if isAR {
			ret.GoExpr = "model.data.Descr"
		} else {
			ret.GoExpr = "model.Descr"
		}

		return ret
	}

	colCounter := &advpggen.Column{
		GoName:     "Counter",
		GoExpr:     "model.data.Counter",
		GoType:     "int",
		ColumnName: "counter",
		Field:      &ActiveRecordEnabledTable.Fields[3],
	}

	colName := &advpggen.Column{
		GoName:     "Name",
		GoExpr:     "model.data.Name",
		GoType:     "string",
		ColumnName: "name",
		Field:      &ImplicitModel.Fields[1],
	}

	// we can modify Index/Field values used as expected results in runtime,
	// because test values are read from the file and aren't taken from the
	// compiled binary.
	ActiveRecordEnabledTable.Fields[3].UpdateByStorage = true

	NoActiveRecordTable.DAO = "ImplicitDAO"
	NoActiveRecordTable.Indices[0].Selector = "SelectNoActiveRecordByPK"
	NoActiveRecordTable.Indices[0].Deleter = "DeleteNoActiveRecordByPK"
	NoActiveRecordTable.Indices[0].IsUniq = true
	NoActiveRecordTable.Indices[1].Name = "IDType"
	NoActiveRecordTable.Indices[1].Selector = "SelectMultiNoActiveRecordByIDType"
	NoActiveRecordTable.Indices[1].Deleter = "DeleteMultiNoActiveRecordByIDType"

	NoPrimaryKeyTable.DAO = NoActiveRecordTable.DAO
	NoPrimaryKeyTable.Indices[0].Name = "ID"
	NoPrimaryKeyTable.Indices[0].Selector = "SelectNoPrimaryKeyByID"
	NoPrimaryKeyTable.Indices[1].Name = "Type"
	NoPrimaryKeyTable.Indices[1].Selector = "SelectNoPrimaryKeyByType"
	NoPrimaryKeyTable.Indices[1].Deleter = ""
	NoPrimaryKeyTable.Indices[2].Name = NoActiveRecordTable.Indices[1].Name
	NoPrimaryKeyTable.Indices[2].Selector = "SelectMultiNoPrimaryKeyByIDType"

	PrimaryKeyOnlyTable.DAO = NoActiveRecordTable.DAO
	PrimaryKeyOnlyTable.Indices[0].Name = "PrimaryKey"
	PrimaryKeyOnlyTable.Indices[0].Selector = "SelectPrimaryKeyOnlyByPrimaryKey"
	PrimaryKeyOnlyTable.Indices[0].Deleter = "DeletePrimaryKeyOnlyByPrimaryKey"
	PrimaryKeyOnlyTable.Indices[0].IsUniq = true
	PrimaryKeyOnlyTable.Indices[1].Name = NoActiveRecordTable.Indices[1].Name

	UpdateOnConflictWithoutValueColumnsTable.DAO = NoActiveRecordTable.DAO
	UpdateOnConflictWithoutValueColumnsTable.UpdateOnConflict = false
	UpdateOnConflictWithoutValueColumnsTable.OnConflictDoNothing = true
	UpdateOnConflictWithoutValueColumnsTable.Indices[0].Name = NoPrimaryKeyTable.Indices[0].Name
	UpdateOnConflictWithoutValueColumnsTable.Indices[0].Selector = "SelectUpdateOnConflictWithoutValueColumnsByID"
	UpdateOnConflictWithoutValueColumnsTable.Indices[0].Deleter = "DeleteUpdateOnConflictWithoutValueColumnsByID"
	UpdateOnConflictWithoutValueColumnsTable.Indices[0].IsUniq = true

	NoMethodsTable.DAO = "CustomDAO"
	NoMethodsTable.Indices[0].Name = PrimaryKeyOnlyTable.Indices[0].Name
	NoMethodsTable.Indices[0].IsUniq = true

	ActiveRecordWithoutValueColumnsTable.Table = "active_record_without_value_columns"
	ActiveRecordWithoutValueColumnsTable.DAO = "DAOInThisFile"
	ActiveRecordWithoutValueColumnsTable.Indices[0].Name = NoPrimaryKeyTable.Indices[0].Name
	ActiveRecordWithoutValueColumnsTable.Indices[0].Selector = "SelectByID"
	ActiveRecordWithoutValueColumnsTable.Indices[0].Deleter = "DeleteByID"
	ActiveRecordWithoutValueColumnsTable.Indices[0].IsUniq = true

	ActiveRecordEnabledTable.DAO = "DAOInOtherFile"
	ActiveRecordEnabledTable.Indices[0].Name = ActiveRecordWithoutValueColumnsTable.Indices[0].Name
	ActiveRecordEnabledTable.Indices[0].Selector = ActiveRecordWithoutValueColumnsTable.Indices[0].Selector
	ActiveRecordEnabledTable.Indices[0].Deleter = ActiveRecordWithoutValueColumnsTable.Indices[0].Deleter
	ActiveRecordEnabledTable.Indices[0].IsUniq = true
	ActiveRecordEnabledTable.Indices[1].Name = ActiveRecordWithoutValueColumnsTable.Indices[0].Name // TODO include Multi in the index name
	ActiveRecordEnabledTable.Indices[1].Selector = "SelectMultiByID"
	ActiveRecordEnabledTable.Indices[1].Deleter = "DeleteMultiByID"
	ActiveRecordEnabledTable.Indices[2].Name = NoActiveRecordTable.Indices[1].Name
	ActiveRecordEnabledTable.Indices[2].Selector = "SelectMultiByIDType"
	ActiveRecordEnabledTable.Indices[2].Deleter = "DeleteMultiByIDType"

	ImplicitModel.Model = &advpggen.ModelName{Name: "Implicit", IsString: true}
	ImplicitModel.DAO = NoActiveRecordTable.DAO
	ImplicitModel.Indices[0].Name = NoPrimaryKeyTable.Indices[0].Name
	ImplicitModel.Indices[0].Selector = "SelectImplicitByID"
	ImplicitModel.Indices[0].Deleter = "DeleteImplicitByID"
	ImplicitModel.Indices[0].IsUniq = true

	tests := []struct {
		name         string
		transformSrc func([]byte) []byte
		want         advpggen.File
		wantErr      string
		must         []string
		mustNot      []string
	}{{
		name:    "broken go file",
		wantErr: "raw string literal not terminated",
	}, {
		name:    "no models",
		wantErr: "no Table is declared in file",
	}, {
		name:    "incorrect var declaration",
		wantErr: "validation failed",
	}, {
		name:    "type alias",
		wantErr: "TestAlias: type alias cannot be used as a table schema",
	}, {
		name:    "not a struct type.go",
		wantErr: "a struct type declaration is expected",
	}, {
		name:    "empty struct",
		wantErr: "EmptyStruct: the struct has no fields",
	}, {
		name:    "embedded types are not supported",
		wantErr: "Outer: embedded types aren't supported yet",
	}, {
		name:    "multiple fields per line",
		wantErr: "each field must be declared separately", // explicitly check that "One, Two" line is ignored
	}, {
		name:    "incompatible ON CONFLICT",
		wantErr: "UpdateOnConflict and OnConflictDoNothing are mutually exclusive",
	}, {
		name:    "UpdateOnConflict without PrimaryKey",
		wantErr: "UpdateOnConflict and OnConflictDoNothing require a primary key",
	}, {
		name:    "UpdateOnConflict with InitByStorage PrimaryKey",
		wantErr: "UpdateOnConflict may not be used with tables with InitByStorage primary keys",
	}, {
		name:    "useless SQLValue",
		wantErr: "SQLValue is useless",
	}, {
		name:    "column name conflict",
		wantErr: `ColunmNameConflict.Duplicate: column name "disallowed" may be specified multiple times only when SQLValue is used`,
	}, {
		name:    "unknown column name",
		wantErr: "unknown column name",
	}, {
		name:    "unknown index column name",
		wantErr: "unknown column name",
	}, {
		name:    "multiple primary keys",
		wantErr: "multiple primary key indices are specified",
	}, {
		name:    "conflicting selector names",
		wantErr: "method name SelectByID is already used for index",
	}, {
		name:    "conflicting Selector and Deleter",
		wantErr: "method name SelectByID is already used for index",
	}, {
		name:    "mutators are used when the ActiveRecord is disabled",
		wantErr: "mutators require ActiveRecord but it is disabled for the table",
	}, {
		name:    "mutators and DisableUpdate",
		wantErr: "DisableUpdate is incompatible with EnableMutators",
	}, {
		name:    "mutators and IsPrimaryKey",
		wantErr: "IsPrimaryKey is incompatible with EnableMutators",
	}, {
		name:    "mutators without primary key",
		wantErr: "mutators require a primary key to be defined",
	}, {
		name:    "DAO is not a struct.go",
		wantErr: "NotAStruct type isn't a struct",
	}, {
		name:    "implicit model without GoType",
		wantErr: "GoType is mandatory for implicitly declared models",
	}, {
		name:    "GoType with explicitly declared table",
		wantErr: "specifying GoType for explicitly declared table model is forbidden",
	}, {
		name: "no ActiveRecord",
		want: testFile([]*advpggen.TableModel{{
			Table:            NoActiveRecordTable,
			GoName:           "NoActiveRecord",
			NeedGeneratedDAO: true,
			HasPackageDAO:    true,
			Columns: []*advpggen.Column{
				colID,
				colType,
				colCreatedAt(&NoActiveRecordTable.Fields[0], false),
				colUpdatedAt(&NoActiveRecordTable.Fields[1], false),
				colDescr(&NoActiveRecordTable.Fields[2], false),
			},
			PrimaryKeyIndex: &NoActiveRecordTable.Indices[0],
			PrimaryKeyColumns: []*advpggen.Column{
				colID,
			},
			InsertValueColumns: []*advpggen.Column{
				colID,
				colType,
				colDescr(&NoActiveRecordTable.Fields[2], false),
			},
			InsertResultColumns: []*advpggen.Column{
				colCreatedAt(&NoActiveRecordTable.Fields[0], false),
				colUpdatedAt(&NoActiveRecordTable.Fields[1], false),
			},
			UpdateValueColumns: []*advpggen.Column{
				colType,
				colDescr(&NoActiveRecordTable.Fields[2], false),
			},
			UpdateResultColumns: []*advpggen.Column{
				colUpdatedAt(&NoActiveRecordTable.Fields[1], false),
			},
		}}),
		must: []string{
			"//foo:bar test",
			"querySelectNoActiveRecordByPK(inID int)",
			"queryDeleteNoActiveRecordByPK(inID int)",
			"queryInsert()",
			"queryFullUpdate()",
		},
		mustNot: []string{
			"//go:generate",
			"ID()",
			"Record()",
		},
	}, {
		name: "no primary key",
		want: testFile([]*advpggen.TableModel{{
			Table:            NoPrimaryKeyTable,
			GoName:           "NoPrimaryKey",
			NeedGeneratedDAO: true,
			HasPackageDAO:    true,
			Columns: []*advpggen.Column{
				colIDNoPK,
				colType,
				colCreatedAt(&advpg.Field{}, false),
			},
			InsertValueColumns: []*advpggen.Column{
				colIDNoPK,
				colType,
				colCreatedAt(&advpg.Field{}, false),
			},
		}}),
		must: []string{
			"querySelectNoPrimaryKeyByID(inID int)",
			"queryInsert()",
			"type SelectMultiNoPrimaryKeyByIDTypeKey",
		},
		mustNot: []string{
			"ID()",
			"queryDeleteByNoPrimaryKeyID(inID int)",
			"queryFullUpdate()",
		},
	}, {
		name: "primary key only",
		want: testFile([]*advpggen.TableModel{{
			Table:            PrimaryKeyOnlyTable,
			GoName:           "PrimaryKeyOnly",
			NeedGeneratedDAO: true,
			HasPackageDAO:    true,
			Columns: []*advpggen.Column{
				colID,
				colTypePK,
			},
			PrimaryKeyIndex: &PrimaryKeyOnlyTable.Indices[0],
			PrimaryKeyColumns: []*advpggen.Column{
				colID,
				colTypePK,
			},
			InsertValueColumns: []*advpggen.Column{
				colID,
				colTypePK,
			},
		}}),
		must: []string{
			"querySelectPrimaryKeyOnlyByPrimaryKey(inID int, inType int)",
			"queryInsert()",
			"queryDeletePrimaryKeyOnlyByPrimaryKey(inID int, inType int)",
			"type DeleteMultiByIDTypeKey",
		},
		mustNot: []string{
			"ID()",
			"queryFullUpdate()",
		},
	}, {
		name: "UpdateOnConflict without value columns",
		want: testFile([]*advpggen.TableModel{{
			Table:            UpdateOnConflictWithoutValueColumnsTable,
			GoName:           "UpdateOnConflictWithoutValueColumns",
			NeedGeneratedDAO: true,
			HasPackageDAO:    true,
			Columns: []*advpggen.Column{
				colID,
			},
			PrimaryKeyIndex: &UpdateOnConflictWithoutValueColumnsTable.Indices[0],
			PrimaryKeyColumns: []*advpggen.Column{
				colID,
			},
			InsertValueColumns: []*advpggen.Column{
				colID,
			},
		}}),
	}, {
		name: "no methods",
		want: testFile([]*advpggen.TableModel{{
			Table:            NoMethodsTable,
			GoName:           "NoMethods",
			NeedGeneratedDAO: true,
			Columns: []*advpggen.Column{
				colID,
				colTypePK,
			},
			PrimaryKeyIndex: &NoMethodsTable.Indices[0],
			PrimaryKeyColumns: []*advpggen.Column{
				colID,
				colTypePK,
			},
			InsertValueColumns: []*advpggen.Column{
				colID,
				colTypePK,
			},
		}}),
		must: []string{"sqlSelect"},
		mustNot: []string{
			"querySelectBy",
			"queryDeleteBy",
			"querySelectMutators",
		},
	}, {
		name: "ActiveRecord without value columns.go",
		want: testFile([]*advpggen.TableModel{{
			Table:  ActiveRecordWithoutValueColumnsTable,
			GoName: "ActiveRecordWithoutValueColumns",
			Columns: []*advpggen.Column{
				colARID(nil),
			},
			PrimaryKeyIndex: &ActiveRecordWithoutValueColumnsTable.Indices[0],
			PrimaryKeyColumns: []*advpggen.Column{
				colARID(nil),
			},
			InsertValueColumns: []*advpggen.Column{
				colARID(nil),
			},
			SetterColumns: []*advpggen.Column{
				colARID(nil),
			},
		}}),
	}, {
		name: "with ActiveRecord enabled",
		transformSrc: func(src []byte) []byte {
			return bytes.Replace(src, []byte(`advpg "`), []byte(`"`), 1)
		},
		want: testFile([]*advpggen.TableModel{{
			Table:  ActiveRecordEnabledTable,
			GoName: "ActiveRecordEnabled",
			Columns: []*advpggen.Column{
				colARID(nil),
				colARType,
				colCreatedAt(&ActiveRecordEnabledTable.Fields[0], true),
				colUpdatedAt(&ActiveRecordEnabledTable.Fields[1], true),
				colDescr(&ActiveRecordEnabledTable.Fields[2], true),
				colCounter,
			},
			PrimaryKeyIndex: &ActiveRecordEnabledTable.Indices[0],
			PrimaryKeyColumns: []*advpggen.Column{
				colARID(nil),
			},
			InsertValueColumns: []*advpggen.Column{
				colARID(nil),
				colARType,
				colDescr(&ActiveRecordEnabledTable.Fields[2], true),
				colCounter,
			},
			InsertResultColumns: []*advpggen.Column{
				colCreatedAt(&ActiveRecordEnabledTable.Fields[0], true),
				colUpdatedAt(&ActiveRecordEnabledTable.Fields[1], true),
				colCounter,
			},
			UpdateValueColumns: []*advpggen.Column{
				colARType,
				colDescr(&ActiveRecordEnabledTable.Fields[2], true),
			},
			UpdateResultColumns: []*advpggen.Column{
				colUpdatedAt(&ActiveRecordEnabledTable.Fields[1], true),
				colCounter,
			},
			SetterColumns: []*advpggen.Column{
				colARID(nil),
				colARType,
				colDescr(&ActiveRecordEnabledTable.Fields[2], true),
			},
			MutatorColumns: []*advpggen.Column{
				colCounter,
			},
		}}, true),
		must: []string{
			"querySelectByID(inID int)",
			"querySelectMultiByID(inIDS []int)",
			"queryDeleteByID(inID int)",
			"queryInsert()",
			"queryFullUpdate()",
			"ID()",
			"SetID",
			"SetType",
			"CreatedAt()",
			"querySelectMutators",
			"Record()",
		},
		mustNot: []string{
			"SetCreatedAt",
			"SetUpdatedAt",
		},
	}, {
		name: "implicit model",
		want: testFile([]*advpggen.TableModel{{
			Table:            &ImplicitModel,
			GoName:           "Implicit",
			NeedGeneratedDAO: true,
			HasPackageDAO:    true,
			IsImplicit:       true,
			Columns: []*advpggen.Column{
				colARID(&ImplicitModel.Fields[0]),
				colName,
			},
			PrimaryKeyIndex: &ImplicitModel.Indices[0],
			PrimaryKeyColumns: []*advpggen.Column{
				colARID(&ImplicitModel.Fields[0]),
			},
			InsertValueColumns: []*advpggen.Column{
				colName,
			},
			InsertResultColumns: []*advpggen.Column{
				colARID(&ImplicitModel.Fields[0]),
			},
			UpdateValueColumns: []*advpggen.Column{
				colName,
			},
			SetterColumns: []*advpggen.Column{
				colName,
			},
		}}),
		must:    []string{"type Implicit struct", `db:"name"`},
		mustNot: []string{`db:"Name"`},
	}}

	testSources := getTestSources()
	verbose := func(string) string {
		return ""
	}

	if os.Getenv("ADVPG_TEST_VERBOSE") != "" {
		verbose = func(src string) string {
			return "\ngenerated:\n" + src
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.transformSrc != nil {
				testSources[tt.name] = &fstest.MapFile{Data: tt.transformSrc(testSources[tt.name].Data)}
			}

			got, err := advpggen.Parse(testSources, tt.name)
			if err != nil {
				if tt.wantErr == "" {
					t.Fatalf("unexpected error %q", err)
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got error %q, want error %q", err, tt.wantErr)
				}

				return
			}

			if tt.wantErr != "" {
				t.Fatalf("got no error, want error %q", tt.wantErr)
			}

			got.DestFileName = "test_generated.go" // there's a separate test for it

			if diff := cmp.Diff(tt.want, got, cmpOptsTableModel...); diff != "" {
				t.Error("Parse: result mismatch (-want +got):\n" + diff)
			}

			buf := &strings.Builder{}

			err = got.Generate(advpggen.NewWriterWriter(buf), advpggen.WithGoimports)
			if err != nil {
				t.Fatal("Generate:", err)
			}

			out := buf.String()

			for _, must := range tt.must {
				if !strings.Contains(out, must) {
					t.Errorf("output doesn't contain the required substring %q%s", must, verbose(out))
				}
			}

			for _, mustNot := range tt.mustNot {
				if strings.Contains(out, mustNot) {
					t.Errorf("output contains the substring it must not contain: %q%s", mustNot, verbose(out))
				}
			}
		})
	}
}

var cmpOptsTableModel = []cmp.Option{
	cmp.FilterValues(filterTableName, cmp.Comparer(compareTableName)),
	cmp.Comparer(func(_, _ map[string][]*advpg.Index) bool { return true }),
	cmp.Comparer(func(_, _ map[string][]*advpg.Field) bool { return true }),
	cmp.Comparer(func(_, _ map[string][]*advpg.Table) bool { return true }),
	cmp.Comparer(func(_, _ map[string]*advpggen.Column) bool { return true }),
	cmp.Comparer(func(_, _ map[string]*advpggen.TableModel) bool { return true }),
}

func filterTableName(name, table any) bool {
	if name == nil || table == nil {
		return false
	}

	if _, ok := table.(*advpggen.ModelName); ok {
		name, table = table, name
	}

	_, ok := name.(*advpggen.ModelName)
	if !ok {
		return false
	}

	rt := reflect.TypeOf(table)
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}

	return rt.Kind() == reflect.Struct
}

func compareTableName(name, table any) bool {
	if modelName1, ok := table.(*advpggen.ModelName); ok {
		if modelName2, ok := name.(*advpggen.ModelName); ok {
			return *modelName1 == *modelName2
		}

		name, table = table, name
	}

	modelName, ok := name.(*advpggen.ModelName)
	if !ok {
		return false
	}

	rt := reflect.TypeOf(table)
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}

	return modelName.Name == rt.Name()
}

func getTestSources() fstest.MapFS {
	name := ""
	ret := make(fstest.MapFS)
	preamble := strings.Builder{}
	buf := bytes.Buffer{} // is more efficient than strings.Builder since we reuse it multiple times

	for l := range strings.SplitAfterSeq(testSource, "\n") {
		newName, ok := strings.CutPrefix(l, "//adv:pg:test:")
		if ok {
			if name != "" {
				ret[name] = &fstest.MapFile{Data: append([]byte(nil), buf.Bytes()...)}
				buf.Reset()
			}

			_, _ = buf.WriteString(preamble.String())
			name = strings.TrimSpace(newName)
		} else {
			if name == "" {
				_, _ = preamble.WriteString(l)
			} else {
				_, _ = buf.WriteString(l)
			}
		}
	}

	if name != "" {
		ret[name] = &fstest.MapFile{Data: append([]byte(nil), buf.Bytes()...)}
	}

	return ret
}

func TestGeneratedFileName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{{
		in:   ".source",
		want: "_generated.source",
	}, {
		in:   ".source.go",
		want: ".source_generated.go",
	}, {
		in:   "source",
		want: "source_generated",
	}, {
		in:   "source.go",
		want: "source_generated.go",
	}, {
		in:   "source_test.go",
		want: "source_generated_test.go",
	}}

	for _, tt := range tests {
		fsys := fstest.MapFS{
			tt.in: &fstest.MapFile{Data: []byte("//\npackage foobar\ntype M struct{ID int}\nvar _ = advpg.Table{Model:M{}}")},
		}

		got, err := advpggen.Parse(fsys, tt.in)
		if err != nil {
			t.Fatal(err)
		}

		if got.DestFileName != tt.want {
			t.Errorf("%s: got file name %s, want %s", tt.in, got.DestFileName, tt.want)
		}
	}
}
