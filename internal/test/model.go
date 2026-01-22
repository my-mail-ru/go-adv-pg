// Package advpgtest foobar
//
//foo:bar
//foo:test

//go:generate go tool adv-pg

package advpgtest

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"time"

	advpg "github.com/my-mail-ru/go-adv-pg"
)

// User is used to test single-field primary keys, IsMulti, custom Deleter, Order,
// InitByStorage, DisableUpdate, UpdateByStorage, and EnableMutators with InitByStorage.
type User struct {
	ID        int    `db:"id"`
	Name      string `db:"name"`
	Type      int    `db:"type"`
	PostCount int    `db:"post_count"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

var _ = advpg.Table{
	Model: User{},
	Table: "users",
	Indices: []advpg.Index{{
		Keys:         []string{"ID"},
		IsPrimaryKey: true,
	}, {
		Keys:    []string{"ID", "Type"},
		IsMulti: true,
		Order: []advpg.Order{{
			Field: "CreatedAt",
			Order: advpg.OrderDesc,
		}},
	}, {
		Keys: []string{"Name"},
	}},
	Fields: []advpg.Field{{
		Field:         "ID",
		InitByStorage: true,
	}, {
		Field:         "CreatedAt",
		InitByStorage: true,
		DisableUpdate: true,
	}, {
		Field:           "UpdatedAt",
		InitByStorage:   true,
		UpdateByStorage: true,
	}, {
		Field:          "PostCount",
		EnableMutators: true,
		InitByStorage:  true,
	}},
}

// ExtLink is used to test multiple-field primary keys, UpdateOnConflict,
// SQLScan, SQLValue, and EnableMutators with InitByStorage (this time
// along with UpdateOnConflict set, unlike [User]).
type ExtLink struct {
	UserID     int    `db:"user_id"`
	ExternalID int    `db:"ext_id"`
	CreatedAt  MyTime `db:"created_at"`
	Status     int    `db:"status"`
	LinkCount  int    `db:"link_count"`
}

var _ = advpg.Table{
	Model:            ExtLink{},
	Table:            "ext_links",
	UpdateOnConflict: true,
	Indices: []advpg.Index{{
		Keys:         []string{"user_id", "ext_id"},
		IsPrimaryKey: true,
	}, {
		Keys: []string{"status"},
		//	DisableSelector: true,
		IsMulti: true,
	}},
	Fields: []advpg.Field{{
		Field:         "created_at",
		DisableUpdate: true,
		SQLScan:       "EXTRACT(EPOCH FROM %s::TIMESTAMP WITH TIME ZONE)::BIGINT AS %s",
		SQLValue:      "TIMESTAMP WITH TIME ZONE 'epoch' + INTERVAL '1 sec' * %s",
	}, {
		Field:          "link_count",
		EnableMutators: true,
		InitByStorage:  true,
	}},
}

// UserViews is used to test EnableMutators without InitByStorage while
// UpdateOnConflict is on (unlike [ExtLink]).
type UserViews struct {
	UserID int `db:"user_id"`
	Views  int `db:"views"` // doesn't have a default and must be set explicitly
}

var _ = advpg.Table{
	Model:            UserViews{},
	Table:            "user_views",
	UpdateOnConflict: true,
	Indices: []advpg.Index{{
		Keys:         []string{"user_id"},
		IsPrimaryKey: true,
	}},
	Fields: []advpg.Field{{
		Field:          "views",
		EnableMutators: true,
	}},
}

// Seen is used to test OnConflictDoNothing.
type Seen struct {
	UserID int       `db:"user_id"`
	SeenAt time.Time `db:"seen_at"`
}

var _ = advpg.Table{
	Model:               Seen{},
	Table:               "seen",
	OnConflictDoNothing: true,
	Indices: []advpg.Index{{
		Keys:         []string{"UserID"},
		IsPrimaryKey: true,
	}},
	Fields: []advpg.Field{{
		Field:         "SeenAt",
		InitByStorage: true,
	}},
}

// MyTime is an example of SQLScan and SQLValue used with [sql.Scanner] and [driver.Valuer] implementation.
type MyTime struct {
	time.Time
}

var (
	_ sql.Scanner   = &MyTime{}
	_ driver.Valuer = MyTime{}
)

func (t *MyTime) Scan(x any) error {
	var unixtime int64

	switch t := x.(type) {
	case int:
		unixtime = int64(t)
	case int64:
		unixtime = t
	default:
		return fmt.Errorf("MyTime.Scan: unsupported type %T, int or int64 is expected", x)
	}

	t.Time = time.Unix(unixtime, 0)

	return nil
}

func (t MyTime) Value() (driver.Value, error) {
	return t.Unix(), nil
}
