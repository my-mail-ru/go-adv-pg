package advpggen

import advpg "github.com/my-mail-ru/go-adv-pg"

// File represents all table model definitions from a single source file.
// A single source file corresponds to a single destination file.
type File struct {
	DestFileName string
	Directives   []string
	Package      string
	AdvPgPkg     string
	AdvPgConnPkg string
	Imports      []ImportSpec
	Models       []*TableModel
	ModelsByName map[string]*TableModel
}

type ImportSpec struct {
	PkgName string
	PkgPath string
}

type TableModel struct {
	*advpg.Table
	GoName              string
	NeedGeneratedDAO    bool // DAO isn't defined in other files (handwritten or generated) of the package
	HasPackageDAO       bool // default index name is based on GoName
	Columns             []*Column
	ColumnsByGoName     map[string]*Column
	ColumnsByName       map[string]*Column
	PrimaryKeyIndex     *advpg.Index
	PrimaryKeyColumns   []*Column
	InsertValueColumns  []*Column
	InsertResultColumns []*Column
	UpdateValueColumns  []*Column
	UpdateResultColumns []*Column
	SetterColumns       []*Column // mutators aren't specified here
	MutatorColumns      []*Column //
}

type Column struct {
	*advpg.Field
	GoName       string // struct field name
	GoExpr       string // model.GoName or model.data.GoName depending on whether ActiveRecord is enabled
	GoType       string // struct field type
	ColumnName   string // db: tag or a GoName converted to the snake_case
	IsPrimaryKey bool   // is set internally in linkIndicesToColumns
}

type IndexKey struct {
	*Column
	IsMulti bool // Index.IsMulti
}

type OrderColumn struct {
	*Column
	Order advpg.OrderDirection
}

type InsertArgColumn struct {
	*Column
	IsUpsertMutator bool
}
