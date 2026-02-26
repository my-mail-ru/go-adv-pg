//go:generate true
//foo:bar test

package advpggen_test

import (
	"time"

	advpg "github.com/my-mail-ru/go-adv-pg"
)

//adv:pg:test: broken go file

var _ = `
//adv:pg:test: file separator
`

//adv:pg:test: no models

//adv:pg:test: incorrect var declaration

var _ = advpg.Table{}

//adv:pg:test: type alias

var _ = advpg.Table{Model: TestAlias{}}

type TestAlias = struct{}

//adv:pg:test: not a struct type.go

var _ = advpg.Table{Model: NotAStruct{}}

type NotAStruct []int

//adv:pg:test: empty struct

var _ = advpg.Table{Model: EmptyStruct{}}

type EmptyStruct struct{}

//adv:pg:test: embedded types are not supported

type Embedded1 struct{}

var _ = advpg.Table{Model: Outer{}}

type Outer struct {
	Embedded1
}

//adv:pg:test: multiple fields per line

type Embedded2 struct{}

var _ = advpg.Table{Model: MultipleFields{}}

type MultipleFields struct {
	Embedded2   `db:"-"`
	One, Two    int `db:"-"`
	Three, Four int
}

//adv:pg:test: incompatible ON CONFLICT

type IncompatOnConflict struct {
	ID int
}

var _ = advpg.Table{
	Model:               IncompatOnConflict{},
	UpdateOnConflict:    true,
	OnConflictDoNothing: true,
}

//adv:pg:test: UpdateOnConflict without PrimaryKey

type UpdateOnConflictWithoutPrimaryKey struct {
	ID int
}

var _ = advpg.Table{
	Model:            UpdateOnConflictWithoutPrimaryKey{},
	UpdateOnConflict: true,
}

//adv:pg:test: UpdateOnConflict with InitByStorage PrimaryKey

type UpdateOnConflictInitByStorage struct {
	ID int
}

var _ = advpg.Table{
	Model:            UpdateOnConflictInitByStorage{},
	UpdateOnConflict: true,
	Indices: []advpg.Index{{
		Keys:         []string{"ID"},
		IsPrimaryKey: true,
	}},
	Fields: []advpg.Field{{
		Field:         "ID",
		InitByStorage: true,
	}},
}

//adv:pg:test: useless SQLValue

var _ = advpg.Table{
	Model: "UselessSQLValue",
	Fields: []advpg.Field{{
		Field:           "Test",
		GoType:          "int",
		InitByStorage:   true,
		UpdateByStorage: true,
		SQLValue:        "useless",
	}},
}

//adv:pg:test: column name conflict

type ColunmNameConflict struct {
	Name         string `db:"name"`
	NullableName string `db:"name"`
	Disallowed   int    `db:"disallowed"`
	Duplicate    int    `db:"disallowed"`
}

var _ = advpg.Table{
	Model: ColunmNameConflict{},
	Fields: []advpg.Field{{
		Field:    "NullableName",
		SQLValue: "NULLIF(%s, 'default')",
	}},
}

//adv:pg:test: unknown column name

type UnknownField struct {
	ID int `db:"id"`
}

var _ = advpg.Table{
	Model: UnknownField{},
	Fields: []advpg.Field{{
		Field: "unknown",
	}},
}

//adv:pg:test: unknown index column name

type UnknownIndexColumn struct {
	ID int `db:"id"`
}

var _ = advpg.Table{
	Model: UnknownIndexColumn{},
	Indices: []advpg.Index{{
		Keys: []string{"unknown"},
	}},
}

//adv:pg:test: multiple primary keys

type MultiplePrimaryKeys struct {
	K1 int
	K2 int
}

var _ = advpg.Table{
	Model: MultiplePrimaryKeys{},
	Indices: []advpg.Index{{
		Keys:         []string{"K1"},
		IsPrimaryKey: true,
	}, {
		Keys:         []string{"K2"},
		IsPrimaryKey: true,
	}},
}

//adv:pg:test: conflicting selector names

type SelectorConflict struct {
	ID int `db:"id"`
}

var _ = advpg.Table{
	Model: SelectorConflict{},
	Indices: []advpg.Index{{
		Selector: "SelectByID",
		Keys:     []string{"ID"},
		IsUniq:   true,
	}, {
		Selector: "SelectByID",
		Keys:     []string{"ID"},
		IsUniq:   true,
		IsMulti:  true,
	}},
}

//adv:pg:test: conflicting Selector and Deleter

type DeleterConflict struct {
	ID int `db:"id"`
}

var _ = advpg.Table{
	Model: DeleterConflict{},
	Indices: []advpg.Index{{
		Selector: "SelectByID",
		Deleter:  "SelectByID",
		Keys:     []string{"ID"},
	}},
}

//adv:pg:test: mutators are used when the ActiveRecord is disabled

type MutatorsWithoutActiveRecord struct {
	ID      int
	Counter int
}

var _ = advpg.Table{
	Model:               MutatorsWithoutActiveRecord{},
	DisableActiveRecord: true,
	Indices: []advpg.Index{{
		Keys:         []string{"ID"},
		IsPrimaryKey: true,
	}},
	Fields: []advpg.Field{{
		Field:          "counter",
		EnableMutators: true,
	}},
}

//adv:pg:test: mutators and DisableUpdate

type MutatorsAndDisableUpdate struct {
	ID      int
	Counter int
}

var _ = advpg.Table{
	Model: MutatorsAndDisableUpdate{},
	Indices: []advpg.Index{{
		Keys:         []string{"ID"},
		IsPrimaryKey: true,
	}},
	Fields: []advpg.Field{{
		Field:          "counter",
		EnableMutators: true,
		DisableUpdate:  true,
	}},
}

//adv:pg:test: mutators and IsPrimaryKey

type MutatorsAndPrimaryKey struct {
	ID      int
	Counter int
}

var _ = advpg.Table{
	Model: MutatorsAndPrimaryKey{},
	Indices: []advpg.Index{{
		Keys:         []string{"id", "counter"},
		IsPrimaryKey: true,
	}},
	Fields: []advpg.Field{{
		Field:          "counter",
		EnableMutators: true,
	}},
}

//adv:pg:test: mutators without primary key

type MutatorsWithoutPrimaryKey struct {
	Counter int
}

var _ = advpg.Table{
	Model: MutatorsWithoutPrimaryKey{},
	Fields: []advpg.Field{{
		Field:          "counter",
		EnableMutators: true,
	}},
}

//adv:pg:test: DAO is not a struct.go

type DAOIsNotAStruct struct {
	ID int
}

var _ = advpg.Table{
	Model: DAOIsNotAStruct{},
	DAO:   "NotAStruct",
}

//adv:pg:test: implicit model without GoType

var _ = advpg.Table{
	Model: "Implicit",
	Fields: []advpg.Field{{
		Field: "ID",
	}},
}

//adv:pg:test: GoType with explicitly declared table

type GoTypeWithExplicitTable struct {
	ID int `db:"id"`
}

var _ = advpg.Table{
	Model: GoTypeWithExplicitTable{},
	Fields: []advpg.Field{{
		Field:  "ID",
		GoType: "int",
	}},
}

//adv:pg:test: no ActiveRecord

type NoActiveRecord struct {
	ID        int       `db:"id"`
	Type      int       `db:"type"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Descr     string    `db:"descr"`
}

var NoActiveRecordTable = &advpg.Table{
	Model:               NoActiveRecord{},
	Table:               "test_table",
	DisableActiveRecord: true,
	Indices: []advpg.Index{{
		Name:         "PK",
		Keys:         []string{"id"},
		IsPrimaryKey: true,
	}, {
		Keys:    []string{"id", "type"},
		IsMulti: true,
		Order:   []advpg.Order{{Field: "created_at", Order: advpg.OrderAsc}},
	}},
	Fields: []advpg.Field{{
		Field:         "created_at",
		InitByStorage: true,
		DisableUpdate: true,
	}, {
		Field:           "updated_at",
		InitByStorage:   true,
		UpdateByStorage: true,
		SQLScan:         "EXTRACT(EPOCH FROM %s)::bigint",
	}, {
		Field:    "descr",
		SQLScan:  "COALESCE(descr, 'default')",
		SQLValue: "NULLIF(%s, 'default')",
	}},
}

//adv:pg:test: no primary key

type NoPrimaryKey struct {
	ID        int       `db:"id"`
	Type      int       `db:"type"`
	CreatedAt time.Time `db:"created_at"`
}

var NoPrimaryKeyTable = &advpg.Table{
	Model:               NoPrimaryKey{},
	Table:               "test_table",
	DisableActiveRecord: true,
	Indices: []advpg.Index{{
		Keys:           []string{"id"},
		IsUniq:         true,
		DisableDeleter: true,
	}, {
		Keys:           []string{"type"},
		DisableDeleter: true,
		Deleter:        "ShouldBeEmpty",
	}, {
		Keys:           []string{"id", "type"},
		IsMulti:        true,
		DisableDeleter: true,
		Order: []advpg.Order{
			{Field: "created_at", Order: advpg.OrderDesc},
			{"id", advpg.OrderDesc}, //nolint:govet
		},
	}},
}

//adv:pg:test: primary key only

type PrimaryKeyOnly struct {
	ID   int `db:"id"`
	Type int `db:"type"`
}

var PrimaryKeyOnlyTable = &advpg.Table{
	Model:               PrimaryKeyOnly{},
	Table:               "test_table",
	DisableActiveRecord: true,
	Indices: []advpg.Index{{
		Keys:         []string{"id", "type"},
		IsPrimaryKey: true,
	}, {
		Keys:            []string{"id", "type"},
		IsMulti:         true,
		Deleter:         "DeleteMultiByIDType",
		DisableSelector: true,
	}},
}

//adv:pg:test: UpdateOnConflict without value columns

type UpdateOnConflictWithoutValueColumns struct {
	ID int
}

var UpdateOnConflictWithoutValueColumnsTable = &advpg.Table{
	Model:               UpdateOnConflictWithoutValueColumns{},
	Table:               "test_table",
	UpdateOnConflict:    true,
	DisableActiveRecord: true,
	Indices: []advpg.Index{{
		Keys:         []string{"ID"},
		IsPrimaryKey: true,
	}},
}

//adv:pg:test: no methods

type NoMethods struct {
	ID   int `db:"id"`
	Type int `db:"type"`
}

var NoMethodsTable = &advpg.Table{
	Model:               NoMethods{},
	Table:               "test_table",
	DAO:                 "CustomDAO",
	DisableActiveRecord: true,
	Indices: []advpg.Index{{
		Keys:            []string{"id", "type"},
		IsPrimaryKey:    true,
		DisableSelector: true,
		DisableDeleter:  true,
	}},
}

//adv:pg:test: ActiveRecord without value columns.go

const PackageDAO = "ImplicitDAO"

type ActiveRecordWithoutValueColumns struct {
	ID int
}

var ActiveRecordWithoutValueColumnsTable = &advpg.Table{
	Model: ActiveRecordWithoutValueColumns{},
	DAO:   "DAOInThisFile",
	Indices: []advpg.Index{{
		Keys:         []string{"id"},
		IsPrimaryKey: true,
	}},
}

type DAOInThisFile struct{}

type DAOInOtherFile struct{}

//adv:pg:test: with ActiveRecord enabled

type ActiveRecordEnabled struct {
	ID        int       `db:"id"`
	Type      int       `db:"type"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Descr     string    `db:"descr"`
	Counter   int       `db:"counter"`
}

var ActiveRecordEnabledTable = &advpg.Table{
	Model:            &ActiveRecordEnabled{},
	Table:            "test_table",
	DAO:              "DAOInOtherFile",
	UpdateOnConflict: true,
	Indices: []advpg.Index{{
		Keys:         []string{"ID"},
		IsPrimaryKey: true,
	}, {
		Keys:    []string{"ID"},
		IsMulti: true,
	}, {
		Keys:    []string{"ID", "Type"},
		IsMulti: true,
		Order:   []advpg.Order{{Field: "created_at"}},
	}},
	Fields: []advpg.Field{{
		Field:         "CreatedAt",
		InitByStorage: true,
		DisableUpdate: true,
	}, {
		Field:           "UpdatedAt",
		InitByStorage:   true,
		UpdateByStorage: true,
	}, {
		Field:    "Descr",
		SQLScan:  "COALESCE(descr, 'default')",
		SQLValue: "NULLIF(%s, 'default')",
	}, {
		Field:          "Counter",
		EnableMutators: true,
	}},
}

//adv:pg:test: implicit model

var ImplicitModel = advpg.Table{
	Model: "Implicit",
	Table: "implicit",
	Fields: []advpg.Field{{
		Field:         "ID",
		Column:        "id",
		GoType:        "int",
		InitByStorage: true,
	}, {
		Field:  "Name", // implicit column name
		GoType: "string",
	}},
	Indices: []advpg.Index{{
		Keys:         []string{"ID"},
		IsPrimaryKey: true,
	}},
}
