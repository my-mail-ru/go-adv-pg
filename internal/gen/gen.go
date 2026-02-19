package advpggen

import (
	"bytes"
	_ "embed"
	"io"
	"iter"
	"strconv"
	"strings"
	"text/template"

	"github.com/ettle/strcase"
	"github.com/gertd/go-pluralize"
	goimports "golang.org/x/tools/imports"

	advpg "github.com/my-mail-ru/go-adv-pg"
)

type FileWriter interface {
	WriteFile(fname string, data []byte) error
}

type queryMethod struct {
	Name        string
	Query       string
	NeedResults bool
}

//go:embed advpg.go.tmpl
var advpgTmplSrc string

var pluralizer = pluralize.NewClient()

var advpgTmpl = template.Must(template.New("").Funcs(template.FuncMap{
	"inc": func(x int) int {
		return x + 1
	},
	"add": func(x, y int) int {
		return x + y
	},
	"shl": func(x int) uint64 {
		return 1 << x
	},
	"sql_value_args": func(col *Column, placeholder int) any {
		return struct {
			Column      *Column
			Placeholder string
		}{
			Column:      col,
			Placeholder: "$" + strconv.Itoa(placeholder+1),
		}
	},
	"simple_printf": func(s, repl string) string {
		// unlike fmt.Sprintf does not corrupt a string if the placeholder isn't used.
		// using placeholders for SQLScan is fully optional
		// and is implemented only for perl ActiveRecord compatibility.
		return strings.ReplaceAll(s, "%s", repl)
	},
	"plural": func(enabled bool, s string) string {
		if !enabled {
			return s
		}

		return pluralizer.Plural(s)
	},
	"quote":          strconv.Quote,
	"to_upper_camel": strcase.ToGoPascal,
}).Parse(advpgTmplSrc))

type generateOpts struct {
	enableGoimports bool
}

type GenerateOptions func(opt *generateOpts)

func WithGoimports(opt *generateOpts) {
	opt.enableGoimports = true
}

func (f *File) Generate(fw FileWriter, opts ...GenerateOptions) error {
	var opt generateOpts

	for _, f := range opts {
		f(&opt)
	}

	buf := &bytes.Buffer{}

	err := advpgTmpl.Execute(buf, f)
	if err != nil {
		return err
	}

	src := buf.Bytes()

	if opt.enableGoimports {
		src, err = goimports.Process(f.DestFileName, src, nil)
		if err != nil {
			return err
		}
	}

	return fw.WriteFile(f.DestFileName, src)
}

type WriterWriter struct {
	w io.Writer
}

func NewWriterWriter(w io.Writer) WriterWriter {
	return WriterWriter{w: w}
}

func (ww WriterWriter) WriteFile(fname string, data []byte) error {
	_, err := ww.w.Write([]byte("///////////// " + fname + " /////////////\n"))
	if err != nil {
		return err
	}

	_, err = ww.w.Write(data)

	return err
}

// IterateSetterColumns iterates over tm.SetterColumns.
// The key is a column's index in tm.UpdateValueColumns, or -1 if
// a column is missing from tm.UpdateValueColumns.
func (tm *TableModel) IterateSetterColumns() iter.Seq2[int, *Column] {
	// using the fact that all the column slices have the same order
	// and that columns can be compared using the pointer comparison only
	return func(yield func(int, *Column) bool) {
		updIdx := 0
		if len(tm.UpdateValueColumns) == 0 {
			updIdx = -1
		}

		for _, col := range tm.SetterColumns {
			if updIdx < 0 || tm.UpdateValueColumns[updIdx] != col {
				if !yield(-1, col) {
					return
				}
			} else {
				if !yield(updIdx, col) {
					return
				}

				if updIdx++; updIdx >= len(tm.UpdateValueColumns) {
					updIdx = -1
				}
			}
		}
	}
}

func (tm *TableModel) IterateIndexKeys(idx *advpg.Index) iter.Seq2[int, IndexKey] {
	return func(yield func(int, IndexKey) bool) {
		for i, key := range idx.Keys {
			col, err := tm.colByName(key)
			if err != nil {
				// should not happen since we've validated all the index keys in linkIndicesToColumns.
				// nevertheless, go iterators' incapability to return an error is disappointing.
				panic("internal error: unknown index key " + key)
			}

			if !yield(i, IndexKey{Column: col, IsMulti: idx.IsMulti}) {
				return
			}
		}
	}
}

func (tm *TableModel) IterateIndexOrder(idx *advpg.Index) iter.Seq2[int, OrderColumn] {
	if len(idx.Order) == 0 {
		return nil
	}

	return func(yield func(int, OrderColumn) bool) {
		for i, order := range idx.Order {
			col, err := tm.colByName(order.Field)
			if err != nil {
				panic(err)
			}

			if !yield(i, OrderColumn{Column: col, Order: order.Order}) {
				return
			}
		}
	}
}

func (tm *TableModel) IterateQueryMethods(idx *advpg.Index) iter.Seq[queryMethod] {
	return func(yield func(queryMethod) bool) {
		if idx.Selector != "" && !yield(queryMethod{
			Name:        "query" + idx.Selector,
			Query:       "sqlSelect" + tm.GoName + " + `",
			NeedResults: true,
		}) {
			return
		}

		if idx.Deleter != "" {
			yield(queryMethod{
				Name:        "query" + idx.Deleter,
				Query:       "`DELETE FROM " + tm.Table.Table,
				NeedResults: false,
			})
		}
	}
}

func (tm *TableModel) IterateInsertArgs() iter.Seq2[int, InsertArgColumn] {
	return tm.iterInsertArgs(true)
}

func (tm *TableModel) IterateInsertArgsVals() iter.Seq2[int, InsertArgColumn] {
	return tm.iterInsertArgs(false)
}

func (tm *TableModel) iterInsertArgs(needMutators bool) iter.Seq2[int, InsertArgColumn] {
	return func(yield func(int, InsertArgColumn) bool) {
		for i, col := range tm.InsertValueColumns {
			if !yield(i, InsertArgColumn{
				Column:          col,
				IsUpsertMutator: tm.UpdateOnConflict && col.EnableMutators,
			}) {
				return
			}
		}

		if !needMutators || !tm.UpdateOnConflict {
			return
		}

		i := len(tm.InsertValueColumns)

		for _, col := range tm.MutatorColumns {
			if !yield(i, InsertArgColumn{Column: col}) {
				return
			}

			i++
		}
	}
}

func (tm *TableModel) resultColumns(cols []*Column) iter.Seq2[int, Column] {
	suffix := ""
	if !tm.DisableActiveRecord {
		suffix = "data."
	}

	return func(yield func(int, Column) bool) {
		for i, col := range cols {
			multiCol := *col
			multiCol.GoExpr = "models[i]." + suffix + col.GoName

			if !yield(i, multiCol) {
				return
			}
		}
	}
}

func (tm *TableModel) InsertMultiResultColumns() iter.Seq2[int, Column] {
	return tm.resultColumns(tm.InsertResultColumns)
}

func (tm *TableModel) UpdateMultiResultColumns() iter.Seq2[int, Column] {
	return tm.resultColumns(tm.UpdateResultColumns)
}

// UpdateMultiAllColumns iterates over all table columns in order, assigning each a kind
// for the UpdateMulti VALUES clause. Columns not involved in the update (e.g. InitByStorage,
// UpdateByStorage) are emitted as NULLs. This is required because UpdateMulti uses a composite
// type cast (t::tablename) to infer column types, and the cast expects the VALUES row to have
// the same number of columns as the table, in the same order.
//
// NULLs are safe even for NOT NULL columns: the composite type cast only performs type conversion
// and does not enforce table-level constraints (NOT NULL is a property of the table, not of the
// composite type that PostgreSQL auto-creates for it).
func (tm *TableModel) UpdateMultiAllColumns() iter.Seq2[int, UpdateMultiCol] {
	updateSet := make(map[*Column]struct{}, len(tm.UpdateValueColumns))
	for _, col := range tm.UpdateValueColumns {
		updateSet[col] = struct{}{}
	}

	mutatorSet := make(map[*Column]struct{}, len(tm.MutatorColumns))
	for _, col := range tm.MutatorColumns {
		mutatorSet[col] = struct{}{}
	}

	return func(yield func(int, UpdateMultiCol) bool) {
		for i, col := range tm.Columns {
			kind := "null"

			_, isUpdate := updateSet[col]
			_, isMutator := mutatorSet[col]

			switch {
			case col.IsPrimaryKey || isUpdate:
				kind = "value"
			case isMutator:
				kind = "mutator"
			}

			if !yield(i, UpdateMultiCol{Column: col, Kind: kind}) {
				return
			}
		}
	}
}

func (tm *TableModel) UpdateMultiAlias() string {
	if tm.Table.Table == "t" {
		return "tt"
	}

	return "t"
}

func (tm *TableModel) RecordType() string {
	if tm.DisableActiveRecord {
		return tm.GoName // the struct type name as declared by a developer
	}

	return tm.GoName + "Record" // generated struct
}
