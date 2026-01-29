[![Go Reference](https://pkg.go.dev/badge/github.com/my-mail-ru/go-adv-pg.svg)](https://pkg.go.dev/github.com/my-mail-ru/go-adv-pg)

# adv-pg: the code-first SQL query generator with ActiveRecord support

## Features

* Maps a value of a Go struct type to the PostgreSQL database table record.
* Generates `SELECT`, `INSERT`, `UPDATE`, and `DELETE` queries and corresponding Data Access Object (DAO).
  methods for single- or multiple-valued keys.
* Multiple keys can be specified for `SELECT` and `DELETE` queries (`IsMulti`).
* Raw SQL snippets can be specified to customize data storage or retrieval (`SQLScan` and `SQLValue`).
* Returning `DEFAULT` values from a table schema during `INSERT` (`InitByStorage`).
* Returning a value set by a `BEFORE UPDATE` trigger during `UPDATE` (`UpdateByStorage`).
* Update an existing record if a unique constraint (like a primary key) conflict occurs, aka "UPSERT" (`UpdateOnConflict`).
* Ignore unique constraint conflict (`OnConflictDoNothing`).
* ActiveRecord: generate Getter and Setter methods for all applicable fields, "smart" `UPDATE` that updates only the
  fields that are really changed.
* Mutators: implement concurrency-safe counters in a table.
* Configuration and initialization of database connections and connection pools using the [OnlineConf](https://github.com/onlineconf/onlineconf).

## Example

```go
//go:generate go tool adv-pg

type UserViews struct {
    UserID int `db:"user_id"`
    Views  int `db:"views"`
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

func main() {
    db, err := advpgconn.NewPool(oconf.Subtree("/project/db")) // or NewConn
    // ...
    // db can also be [pgx.Conn], [pgxpool.Pool], or any type implementing [advpg.DB]

    dao := NewUserViewsDAO(db)

    views := UserViews{
        UserID: userID,
        Views:  initialViews,
    }.Record()

    err := dao.Insert(ctx, views)
    // ...

    gotViews, err := dao.SelectByUserID(ctx, userID)
    // ...

    views.IncViews()

    err := dao.Update(ctx, views)
    // ...
}

```

## Running the tests

This project includes a high-coverage test suite:
* Go unit tests for testing the code generator internals. Use `make test` command to run these tests.
* Integration tests that interact with the real PostgreSQL instance. `docker compose` is required.
  Use `make test-integration` command to run these tests.
* Integration tests reside in the [internal/test](../internal/test) subdirectory (see
  the [schema](../internal/test/testdata/test-schema.sql) and the [model](../internal/test/model.go)).
  If you modify the model, run `make generate` command to re-generate the database access code.

## Go documentation

[https://pkg.go.dev/github.com/my-mail-ru/go-adv-pg](https://pkg.go.dev/github.com/my-mail-ru/go-adv-pg)
