/*
Package advpgconn - configuration and initialization of pgx connections and connection pools.

# OnlineConf

This package uses OnlineConf to configure connections and connection pools.
The typical configuration layout is:

	confRoot, err := onlineconf.OpenModule("")
	dbSubtree := confRoot.Subtree("/project/db")
	pool, err := advpgconn.NewPool(ctx, dbSubtree) // uses /project/db/base and so on

The [OnlineConf] interface assumes the [github.com/onlineconf/onlineconf-go/v2] llibrary, but you may use any compatible implementation (or test mock).

Connection and pool settings are compatible with the Perl implementation. Durations can be specified as integer seconds
or [time.Duration] syntax (which obviously isn't compatible with Perl).

# Connection settings

The following settings are used by [NewConn] (which in turn calls [LoadConnConfigs]):
  - /host - the database master host with port,
  - /replica - the database replica(s) host with port. Multiple comma or semicolon host names are supported,
  - /base - the database name,
  - /user,
  - /pass,
  - /timeout - query and statement timeout. Default: [DefaultTimeout] (30s),
  - /connect_timeout - connection timeout. Default: timeout setting,
  - /set_statement_timeout - perform an SET statement_timeout query after establishing the new connection.
    Default: don't set a statement timeout (0).

Note that round-robin replica balancing method is not yet supported. The connection to the next host on this list is
performed only when the previous one isn't available.

DefaultQueryExecMode is set to SimpleProtocol.

/timeout and /set_statement_timeout are checked every time a new connection is established or a query is executed.
To change any other connection setting, you have to restart the application (TODO support configuration reloading).

# Pool settings

All of the above, plus:
  - /pool_max_conns - maximum pool size. Fallback: /pool_size (for Perl compatibility). Default: [DefaultPoolMaxConns] (10),
  - /pool_min_conns - minimum pool size. Default: DefaultPoolMinConns (1),
  - /pool_min_idle_conns - minimum number of idle connections in the pool. Default: DefaultPoolMinIdleConns (0),
  - /pool_max_conn_lifetime - duration since creation after which a connection will be automatically closed. Default: not set.
  - /pool_max_conn_lifetime_jitter - maximum random increment for /pool_max_conn_lifetime. Default: not set.
  - /pool_max_conn_idle_time - Default: not set.
  - /pool_health_check_period - duration between connection checks. Default: not set.
  - /pool_ping_timeout - maximum amount of time to wait for a ping reply. Default: not set.

For more details, see [pgxpool.Config] and [pgxpool.ParseConfig].

To change any of these settings, you have to restart the application.

# Table settings

These settings can be set per-table:
  - /table/${TableName}/timeout - query timeout. Overrides /timeout.
  - /table/${TableName}/force_replica_usage - use a replica for Select queries even if it's not enabled
    explicitly with [advpg.WithReplica].

Note that the /force_replica_usage setting isn't compatible with similarly named Perl setting
because the Perl implementation uses language-specific package names instead of table names used by this library.

Per-table settings are checked on-the-fly, so no application restart is required to change these.
*/
package advpgconn
