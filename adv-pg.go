package advpg

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ pgxpool.Pool // hey goimports

// Table describes a database table. [github.com/my-mail-ru/go-adv-pg/cmd/adv-pg] code generator scans the source file
// for global variable declarations of this type. The variable can have a name or be anonymous (`_`),
// and this doesn't affect the code generation result.
//
// Example:
//
//	type UserViews struct {
//	  UserID int `db:"user_id"`
//	  Views  int `db:"views"`
//	}
//	var _ = advpg.Table{
//	  Model:            UserViews{},
//	  Table:            "user_views",
//	  UpdateOnConflict: true,
//	  Indices: []advpg.Index{{
//	    ...
//	  }},
//	  Fields: []advpg.Field{{
//	    ...
//	  }},
//	}
type Table struct {
	// Model specifies a go struct type describing a database table record.
	// Model can be specified by value (`ModelTable{}`) or by pointer (`&ModelTable{}`).
	// The value is completely ignored.
	//
	// The `db:"column_name"` struct tags can be used to specify a field name in a table.
	// This way is compatible with [pgx], although pgx's reflection-based mapping mechanism
	// is not used by this library.
	//
	// Model is mandatory.
	Model any

	// Table specifies a name of a database table corresponding to Model.
	//
	// Default: Model type go name in snake_case.
	// Using the default is not recommended, always specify a table name explicitly.
	Table string

	// DAO specifies a Database Access Object type name - the receiver type for query
	// methods (SelectBy.../Insert/Update/etc). You can use a single DAO shared between
	// multiple table models related to the same entity/task to emphasize their relation,
	// and to simplify (or complicate!) reading your code.
	//
	// You can also make all the models of your package use the single DAO type using
	// the PackageDAO constant declared somewhere in your package (i.e. not only in the
	// same file where the model declaration resides):
	//
	//    const PackageDAO = "DAO"
	//
	// Default: model go type name + "DAO" (a DAO per table - i.e. no shared DAOs).
	//
	// You can declare the DAO type manually. It should be a struct type with the `db` field
	// which type implements the [DB] interface (i.e. having the same method set). You can use
	// any type (not only an interface type), including [pgx.Conn] or [pgxpool.Pool].
	//
	// Otherwise, if the struct type specified by DAO is missing from your package (all source files
	// of the package are checked, including the generated ones, but excluding the
	// file is being generated), it is generated. A generated DAO struct will have the single field
	// `db` of the [DB] type.
	//
	// In most cases, the generated DAO is sufficient.
	DAO string

	// DisableActiveRecord set to `true` disables:
	//  - generation of Record type. The Model type itself will be accepted/returned by the DAO methods.
	//  - generation of accessor methods (getters/setters)
	//  - mutators
	//  - generation of "smart" Update methods (only the FullUpdate method will be generated)
	//
	// Use it for tables with simple schemas or for "SELECT-only" data.
	DisableActiveRecord bool

	// EnableLock isn't implemented yet. Avoid using the Model (or Record) types from
	// multiple goroutines!
	EnableLock bool // TODO

	// UpdateOnConflict issues `ON CONFLICT DO UPDATE` for `INSERT` queries.
	// UpdateByStorage or Mutator fields are retrieved (like `UPDATE` queries do).
	// Also known as "UPSERT".
	//
	// A primary key is required for tables with UpdateOnConflict enabled.
	UpdateOnConflict bool

	// OnConflictDoNothing issues `ON CONFLICT DO NOTHING` for `INSERT` queries.
	// A primary key is required for tables with UpdateOnConflict enabled.
	OnConflictDoNothing bool

	// Indices describe column sets that are used as keys in Select/Delete/Update
	// queries. Strictly speaking, these sets don't have to correspond to the real
	// database indices declared using the `CREATE INDEX` query.
	Indices []Index

	// Fields describe special field properties such as InitByStorage or custom
	// raw SQL snippets. Most table fields don't have to be listed here.
	Fields []Field
}

// Index describes a set of columns used as keys in Select/Delete/Update queries.
// Strictly speaking, the Index value doesn't have to correspond to the real
// database indices declared using the `CREATE INDEX` query.
type Index struct {
	// Name is used as the default suffix for accessor method names and for query metric labels.
	//
	// By default, the Name is a concatenation of Keys (e.g. `IDType` for `Keys: {"ID", "Type"}`)
	// for non-primary key index.
	// For the primary key index with the single key, the default Name is this key name
	// (e.g. `ID` for `Keys: {"ID"}`) or `Primary Key` for a primary index with multiple keys.
	Name string

	// Selector is a base name for a method performing the `SELECT` query for this index.
	//
	// To disable selectors, set `DisableSelector: true`.
	//
	// Default: `"SelectBy" + Name`, or `"SelectMultiBy" + Name` if `IsMulti` is on.
	// When `PackageDAO` is used, the default names are `"Select" + model.GoName + "By" + Name"`
	// and `"SelectMulti" + model.GoName + "By" + Name"`.
	Selector string

	// Deleter is a base name for a method performing the `DELETE` query for this index.
	//
	// To disable deleters, set `DisableDeleter: true`.
	//
	// Default: `"DeleteBy" + Name`, or `"DeleteMultiBy" + Name` if `IsMulti` is on.
	// When `PackageDAO` is used, the default names are `"Delete" + model.GoName + "By" + Name"`
	// and `"DeleteMulti" + model.GoName + "By" + Name"`.
	Deleter string

	// Keys can be specified using a go struct field names (e.g. `CreatedAt`) or a database
	// table column names (e.g. `created_at`). Both are valid and produce the same result.
	Keys []string

	// DisableSelector set to `true` disables the selector method generation.
	DisableSelector bool

	// DisableDeleter set to `true` disables the deleter method generation.
	DisableDeleter bool

	// IsMulti indicates that Selector and Deleter methods should accept several possible keys.
	// For a single-field index, these methods will accept a slice of a field type
	// (e.g. `SelectMultiByIDs(..., ids []int)`). For a multiple-field index, the key struct type
	// will be generated (e.g. `SelectMultiByIDType(..., keys []SelectMultiByIDTypeKey)`).
	//
	// Empty `SELECT` responses aren't considered as errors and simply return an empty slice.
	IsMulti bool

	// IsPrimaryKey declares the primary key index. IsUniq is implied. A table must have at most
	// one primary key declared. Tables with primary keys support UpdateOnConflict,
	// OnConflictDoNothing and smart Update queries.
	IsPrimaryKey bool

	// IsUniq specifies that Select queries return exactly one value.
	// IsUniq is set to true automatically for primary key indices.
	// The [sql.ErrNoRows] error is returned if no record corresponding to
	// the Select method arguments is found.
	//
	// May not correspond to the "real" database unique index or constraint.
	IsUniq bool

	// DefaultLimit is the default limit for `SELECT` queries.
	DefaultLimit uint

	// Order specifies the `ORDER BY` clause for `SELECT queries`.
	// Multiple order specifications are accepted.
	Order []Order

	// Condition specifies an additional `WHERE` clause condition for `SELECT` queries.
	Condition string // TODO what about `DELETE` queries?
}

type OrderDirection int

// OrderAsc is the default order.
const (
	OrderAsc OrderDirection = iota
	OrderDesc
)

// Order specifies the `ORDER BY` clause for `SELECT` queries.
type Order struct {
	Field string         // like Field and Keys, you can use a go struct field name or a database table column name
	Order OrderDirection // default: OrderAsc
}

// Field describes special field properties such as InitByStorage or custom
// raw SQL snippets. Most table fields don't have to be listed here.
type Field struct {
	// Field name. You can use a go struct field name (e.g. `CreatedAt`) or a database
	// table column name (e.g. `created_at`). Both are valid and produce the same result.
	Field string

	// SQLScan is a raw SQL snippet for query results (i.e. `SELECT` or `INSERT/UPDATE...RETURNING`).
	//
	// Is the [sql.Scanner]'s SQL counterpart.
	//
	// Note that you don't have to implement [sql.Scanner] for all the field types with SQLScan enabled
	// (e.g. for integer timestamps).
	SQLScan string

	// SQLValue is the raw SQL snippet for query arguments (`INSERT` or `UPDATE`).
	//
	// Is the [driver.Valuer]'s SQL counterpart.
	//
	// Like with SQLScan, you don't have to implement [driver.Valuer] for all the field types
	// with SQLValue enabled.
	SQLValue string

	// InitByStorage specifies that the field value is to be returned by the `INSERT ... RETURNING`
	// query. Can be used together with `DEFAULT` in the database schema.
	InitByStorage bool

	// UpdateByStorage specifies that the field value is to be returned by the `UPDATE ... RETURNING`
	// query. Can be used together with the `BEFORE UPDATE` triggers.
	UpdateByStorage bool

	// DisableInsert disables passing this field to the `INSERT` query. You can still use InitByStorage
	// to retrieve the default field value from a database.
	DisableInsert bool

	// DisableUpdate disables passing this field to the `UPDATE` query. The Setter method will not be generated.
	// You can still use UpdateByStorage to retrieve the current value from a database (including
	// value set by a trigger).
	DisableUpdate bool

	// EnableMutators generates Inc, Dec, and Add methods for a field. These methods can be used to modify
	// the field value atomically. The value after the modification will be set by Update (or Insert if
	// UpdateOnConflict is enabled). You can use mutators, e.g. to implement counters when multiple
	// instances are modifying a table in parallel.
	//
	// Mutators require that the primary key be defined for a table. DisableActiveRecord should be false.
	EnableMutators bool
}

// Validate ensures that the Model is always specified for a [Table].
func (t *Table) Validate() error {
	if t.Model == nil {
		return errors.New("missing Model")
	}

	return nil
}

// Validate ensures that Keys are always specified for an [Index].
func (idx *Index) Validate() error {
	if len(idx.Keys) == 0 {
		return errors.New("missing Keys")
	}

	return nil
}

// KeyStructName returns a key type name if idx is a multi index
// with multiple keys, and Selector and Deleter aren't both disabled.
// In other cases, it returns an empty string.
func (idx *Index) KeyStructName() string {
	if !idx.IsMulti || len(idx.Keys) <= 1 || idx.Selector == "" && idx.Deleter == "" {
		return ""
	}

	if idx.Selector != "" {
		return idx.Selector + "Key"
	}

	return idx.Deleter + "Key"
}

// Validate ensures that the Field name is always set.
func (f *Field) Validate() error {
	if f.Field == "" {
		return errors.New("missing Field")
	}

	return nil
}

// DB describes the database interface required by the code generated by this library.
// [pgx.Conn], [pgxpool.Pool], and [pgx.Tx] implement this interface.
//
// Note that you can use any other type with the same method set if you wish
// to use handwritten DAO type.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type Query interface {
	SQL() string
	Args() []any
	Results() []any
}

// StringWriter is implemented by [strings.Builder] and [bytes.Buffer], among others.
type StringWriter interface {
	WriteString(string) (int, error)
	String() string
}

// SimpleQuery is a "static" query that isn't indended for modification.
// Implements the [Query] interface.
type SimpleQuery struct {
	sql     string
	args    []any
	results []any
}

var _ Query = &SimpleQuery{}

func NewSimpleQuery(sql string, args, results []any) *SimpleQuery {
	return &SimpleQuery{
		sql:     sql,
		args:    args,
		results: results,
	}
}

func (q *SimpleQuery) SQL() string {
	return q.sql
}

func (q *SimpleQuery) Args() []any {
	return q.args
}

func (q *SimpleQuery) Results() []any {
	return q.results
}

// QueryBuilder is a dynamic query builder. Implements the [Query] interface.
type QueryBuilder struct {
	sql     StringWriter
	args    []any
	results []any
}

var _ Query = &QueryBuilder{}

func NewQueryBuilder(sql string) *QueryBuilder {
	// TODO prealloc
	sb := &strings.Builder{}
	sb.WriteString(sql)

	return &QueryBuilder{
		sql:     sb,
		args:    nil,
		results: nil,
	}
}

func (qb *QueryBuilder) AppendSQL(sql string) {
	_, _ = qb.sql.WriteString(sql)
}

func (qb *QueryBuilder) AppendPlaceholder() {
	qb.AppendSQL("$")
	qb.AppendPlaceholderNum()
}

func (qb *QueryBuilder) AppendPlaceholderNum() {
	qb.AppendSQL(strconv.Itoa(len(qb.args) + 1))
}

func (qb *QueryBuilder) AppendArgs(arg any) {
	qb.args = append(qb.args, arg)
}

func (qb *QueryBuilder) AppendResults(res any) {
	qb.results = append(qb.results, res)
}

func (qb *QueryBuilder) SetResults(res []any) {
	qb.results = res
}

func (qb *QueryBuilder) SQL() string {
	if qb.sql == nil {
		return ""
	}

	return qb.sql.String()
}

func (qb *QueryBuilder) Args() []any {
	return qb.args
}

func (qb *QueryBuilder) Results() []any {
	return qb.results
}
