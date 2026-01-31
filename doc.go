/*
Package advpg - the code-first SQL query generator with [ActiveRecord] support.

# Features

  - Maps a value of a Go struct type to the PostgreSQL database table record.
  - Generates `SELECT`, `INSERT`, `UPDATE`, and `DELETE` queries and corresponding Data Access Object (DAO).
    methods for single- or multiple-valued keys.
  - Multiple keys can be specified for `SELECT` and `DELETE` queries (`IsMulti`).
  - Raw SQL snippets can be specified to customize data storage or retrieval (`SQLScan` and `SQLValue`).
  - Returning `DEFAULT` values from a table schema during `INSERT` (`InitByStorage`).
  - Returning a value set by a `BEFORE UPDATE` trigger during `UPDATE` (`UpdateByStorage`).
  - Update an existing record if a unique constraint (like a primary key) conflict occurs, aka "UPSERT" (`UpdateOnConflict`).
  - Ignore unique constraint conflict (`OnConflictDoNothing`).
  - [ActiveRecord]: generate Getter and Setter methods for all applicable fields, "smart" `UPDATE` that updates only the
    fields that are really changed.
  - [Mutators]: implement concurrency-safe counters in a table.
  - [github.com/my-mail-ru/go-adv-pg/conn]: Configuration and initialization of database connections and connection pools using the [OnlineConf].

# Simplified workflow

1. Add [github.com/my-mail-ru/go-adv-pg/cmd/adv-pg] tool for your project:

	go get -tool github.com/my-mail-ru/go-adv-pg/cmd/adv-pg

2. Declare a table model type, include a //go:generate directive in this file:

	//go:generate go tool adv-pg

	type UserViews struct {
	    UserID int `db:"user_id"`
	    Views  int `db:"views"`
	}

3. Declare [Table], [Index], [Field]:

	var _ = advpg.Table{
		Model:            UserViews{},
		Table:            "user_views",
		UpdateOnConflict: true,
		Indices: []advpg.Index{{
			...
		}},
		Fields: []advpg.Field{{
			...
		}},
	}

4. Generate the code:

	go generate ./...

5. Use generated methods:

	dao := NewUserDAO(db)
	views, err := dao.SelectByUserID(ctx, userID)

# Generated files

For every source file having the //go:generate directive as specified above, the corresponding
output file in the same directory is generated. The generated file name is suffixed with
_generated (e.g. model_generated.go for the source file model.go).

Every [Table] describing the model ${Model} declared in the source file causes the
generation of the following entities:
  - Query constants (sqlSelect..., sqlInsert... and so on) containing SQL query parts,
  - The Record type named ${Model}Record, if the [ActiveRecord] isn't disabled,
  - Accessor methods: Getters for every column, and Setters for every updatable column, and
    [Mutators], if the [ActiveRecord] isn't disabled.
    The receiver type for these methods is the Record type,
  - Query methods (querySelect..., queryInsert and so on) that return the type implementing the [Query] interface. The receiver type for these
    methods is the Record type (or the ${Model} itself if the [ActiveRecord] is disabled),
  - The DAO type, if it isn't specified explicitly by a developer (concerns both the default and explicitly
    declared DAOs),
  - Database access methods ([Select], [Insert], [Update], [Delete]) that accept keys or Record values (or the ${Model}
    if the [ActiveRecord] is disabled). The receiver type for these methods is the DAO type. The first
    argument of all these methods is always the [context.Context].

The Record type, accessors, the DAO type, and database access methods are exported, other entities are not.
Exported entities represent a public API of the model, and non-exported ones can be used to
extend this public API with custom database access methods (like cross-table joins).

# ActiveRecord

In a broader sense, the active record pattern is a software architectural pattern that maps
an object instance in memory to a database table record. This project treats ActiveRecord as:
  - High-level database access for a defined user type. The developer declares tables, indices,
    and properties of some fields, then the queries and database access methods are generated automatically,
  - Direct access to the object fields is restricted (i.e. is possible only from inside the
    model package), and generated Getter and Setter methods of a Record type  have to be used for it,
  - The Update database access method updates only the fields that were changed since
    the previous Select, Insert or Update operation. If no fields were changed, the Update
    operation is omitted,
  - [Mutators] can be used to implement concurrency-safe counters in a table.

When the ActiveRecord is disabled (DisableActiveRecord in a [Table] definition), no Update or accessor
methods are generated, but you can access the object fields directly (no additional Record type is generated).

To simplify a record initialization, the Record() method of the Model is generated.

# Select

For each [Index] declared for a table, a Select DAO method is generated unless the
[Index].DisableSelector is set to true. You can specify the Select method name
explicitly using the [Index].Selector, otherwise the method name is produced in
the following way:
  - The base method name is SelectBy for single-key Select methods or SelectMultiBy
    when IsMulti is on,
  - In the PackageDAO mode (see [Table] for detailed description), the base method
    names are Select${Model}By or Select${Model}MultiBy, respectively,
  - Then the Index.Name is appended.

In complex table schemas with many multi-key indices, default Selector names may become illegible.
In such cases, always specify the method name explicitly, describing the purpose of the operation
instead of simply listing the keys (e.g. SelectBlogPosts vs. SelectMultiPostsByBlogIDPostIDStatus).

IsMulti, IsUniq, and also the count of the index keys determine the possible combinations of
arguments and returned values:

  - IsMulti: false, IsUniq: true: the Select method accepts index key fields as separate arguments.
    Exactly one record is returned by value of the Record type (or the ${Model} type itself when
    the [ActiveRecord] is disabled). The [sql.ErrNoRows] error is returned if no record corresponding to
    the Select method arguments is found.

  - IsMulti: false, IsUniq: false: the Select method accepts index key fields as separate arguments.
    A slice of []Record (or []${Model) type is returned. Empty `SELECT` responses aren't
    considered as errors and simply return an empty slice.

  - IsMulti: true, a single-key index: the Select method accepts a key slice.
    A slice of []Record (or []${Model) type is returned. Empty `SELECT` responses aren't
    considered as errors and simply return an empty slice.

  - IsMulti: true, a multi-key index: the Select method accepts a slice of the generated
    type, which name is the Select method name with "Key" appended.
    A slice of []Record (or []${Model) type is returned. Empty `SELECT` responses aren't
    considered as errors and simply return an empty slice.

Some of the following options can be specified after the key(s) argument(s):
  - advpg.[WithLimit] - override the DefaultLimit specified for an [Index],
  - advpg.[WithOffset] - use an offset,
  - advpg.[WithReplica] - whether a replica should be used to perform a query.

Examples:

Having the configuration:

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
		},
		...

1. Simple Select:

	user, err := userDAO.SelectByID(ctx, userID, advpg.WithReplica(true))

2. SelectMulti by composite key:

	    results, err := userDAO.SelectMultiByIDType(ctx, []SelectMultiByIDTypeKey{
			{ID: 10, Type: 1},
			{ID: 20, Type: 2},
			// ...
	    }, advpg.WithLimit(5))

# Delete

The only difference between Select and Delete method configuration is the names of the Index
properties (Deleter and DisableDeleter).
Only the error value is returned by the Deleter.

# Insert

The insert operation is represented by two methods:
  - Insert, which takes a single record,
  - InsertMulti, which takes a slice of records.

The Insert DAO method takes two arguments: the [context.Context] and a pointer to the Record
(or to ${Model} directly if [ActiveRecord] is disabled for a table).

  - All the fields except for those having DisableInsert and InitByStorage set to true
    are INSERTed into a table,

  - Fields with DisableInsert set to true are completely ignored,

  - Fields with InitByStorage set to true are included in the RETURNING clause of a query.
    You can implement auto-increment IDs using this [Field] flag,

  - If the OnConflictDoNothing is set to true for the [Table], an attempt to insert a record
    with an already existing primary key value will be completely ignored,

  - If the UpdateOnConflict is set to true for the [Table], an attempt to insert a record
    with an already existing primary key value will be converted to UPDATE, which will SET
    all the non-primary key fields to the actual values,

  - If the UpdateOnConflict is set to true for the [Table], and there are fields with
    mutators enabled (EnableMutators: true in the [Field] definition), mutators are updated
    as usual (using an incremental counter rather than setting the value directly), and the actual
    value is returned from the table back to the Record/Model value,

    UpdateOnConflict may not be used with tables with InitByStorage primary keys.

  - You can use InitByStorage with mutators to set an initial value of the counter using
    DEFAULT in a table schema.

Calling the InsertMulti method is equivalent to calling the Insert method for each record in a slice,
but only a single query is performed. Currently, when UpdateOnConflict _and_ [Mutators] are
both used, the InsertMulti method isn't generated. All other features described above are
supported, including UpdateOnConflict or [Mutators] used alone.

A successful Insert (or InsertMulti) will reset the changed field flags for all the record(s) processed.
Flags are not reset when InsertMulti fails, regardless of whether some records were successfully inserted.

TODO support [Mutators] with UpdateOnConflict enabled using `INSERT ... ON CONFLICT DO UPDATE ... FROM VALUES` syntax.

# Update

The update operation is represented by two methods:

  - FullUpdate. Requires that some [Index] of a [Table] be declared as a primary key.
    If no primary key is declared for a table, this method isn't generated.
    Does not require [ActiveRecord] (i.e. is generated for every table that has a primary key
    declaration). This method updates all settable (see below) fields of a table.
    The mutator fields are always updated too, regardless of whether the mutator counter
    is non-zero.
  - Update (aka "smart" Update). Like FullUpdate, it requires a primary key to be declared.
    Requires [ActiveRecord] (i.e. DisableActiveRecord set to false for a [Table]).
    The Record type's Setter methods and mutator (Inc/Dec/Add) methods track data changes.
    Only the changed fields (i.e. on which the Set method is called, or which
    mutator counter is non-zero) are mentioned in the SET clause of the UPDATE query.
    If a record has no changed fields and there are no mutator fields defined, the
    UPDATE query isn't issued at all.
    When there are some mutator fields defined, but no fields are changed (i.e. no Set methods
    are called after the previous operation, and all mutator counters are zero),
    the SELECT operation is issued instead of the UPDATE to retrieve the current mutator values
    from a database. Thus, all mutator fields are guaranteed to hold actual values when
    the Update method returns.

The following [Field] properties control the [Update] method behavior:

  - UpdateByStorage. The value is returned from a database using the UPDATE...RETURNING query.
    Can be used together with the `BEFORE UPDATE` triggers.
  - DisableUpdate. Fully disables the Update of a field.

When any of these properties are set to true, the Setter method for a field isn't generated.

# Mutators

Mutators (EnableMutators: true in the [Field] definition) are additional accessor methods
(Inc, Dec, and Add) that update the corresponding fields atomically, performing
addition (subtraction) using database queries (rather than modifying the field's value in memory)
during the [Update] operation. Note that when no fields are changed, including EnableMutators ones,
the Update operation will be converted to Select to retrieve actual values from the database.

Mutator methods are:

  - IncFieldName - increments the mutator counter inside the Record struct by 1,
  - DecFieldName - decrements the mutator counter inside the Record struct by 1,
  - AddFieldName(x int) - adds x (possibly negative) to the mutator counter inside the Record struct.

[ActiveRecord]: https://pkg.go.dev/github.com/my-mail-ru/go-adv-pg#hdr-ActiveRecord
[Select]: https://pkg.go.dev/github.com/my-mail-ru/go-adv-pg#hdr-Select
[Delete]: https://pkg.go.dev/github.com/my-mail-ru/go-adv-pg#hdr-Delete
[Insert]: https://pkg.go.dev/github.com/my-mail-ru/go-adv-pg#hdr-Insert
[Update]: https://pkg.go.dev/github.com/my-mail-ru/go-adv-pg#hdr-Update
[Mutators]: https://pkg.go.dev/github.com/my-mail-ru/go-adv-pg#hdr-Mutators
[OnlineConf]: https://github.com/onlineconf/onlineconf
*/
package advpg

// TODO remove section link hacks when the go doc will support it natively
