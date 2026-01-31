package advpgconn

import (
	"context"
	"errors"
	"log/slog"
	"path"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/multitracer"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/onlineconf/onlineconf-go/v2"

	"github.com/my-mail-ru/go-adv-metrics/pgxmetrics"
	advmetricsset "github.com/my-mail-ru/go-adv-metrics/set"
	advpg "github.com/my-mail-ru/go-adv-pg"
)

// Conn represents the master and the replica(s) connections.
// The replica connection is optional, while the master is not.
type Conn struct {
	*pgx.Conn
	replica *pgx.Conn
	config  OnlineConf
}

// Replica returns the replica connection if it's configured.
// The master connection is returned otherwise.
func (c *Conn) Replica() advpg.DB {
	if c.replica == nil {
		return c.Conn
	}

	return c.replica
}

// ReplicaPerTable checks the /table/TableName/force_replica_usage setting in the OnlineConf
// to determine whether the replica should be used.
// The master connection is returned otherwise.
func (c *Conn) ReplicaPerTable(table string) advpg.DB {
	if c.replica == nil || !c.config.GetBool(path.Join("/table", table, "force_replica_usage"), false) {
		return c.Conn
	}

	return c.replica
}

// OnlineConf returns an [OnlineConf] instance passed to [NewConn].
func (c *Conn) OnlineConf() OnlineConf {
	return c.config
}

// ConnOptionFunc represents options for the [NewConn].
// Any user-defined function accepting a pointer to the [pgx.ConnConfig] can be used as an option.
type ConnOptionFunc func(*pgx.ConnConfig)

// WithConnTracers attaches custom tracers to the connection config.
func WithConnTracers(tracers ...pgx.QueryTracer) ConnOptionFunc {
	return func(conf *pgx.ConnConfig) {
		conf.Tracer = multitracer.New(append(tracers, conf.Tracer)...)
	}
}

// WithConnMetrics enables collection of the query and connection metrics.
//
// For detailed description of these metrics, see [pgxmetrics.Tracer].
func WithConnMetrics(ms advmetricsset.Set) ConnOptionFunc {
	return WithConnTracers(pgxmetrics.New(ms))
}

// NewConn creates the master and optionally the replica(s) connection(s).
//
// Use single connections for one-shot cron jobs, CLI tools, etc.
// For high-availability workload like web servers, use [NewPool].
func NewConn(ctx context.Context, config OnlineConf, optFuncs ...ConnOptionFunc) (*Conn, error) {
	masterConf, replicaConf, err := LoadConnConfigs(config)
	if err != nil {
		return nil, err
	}

	for _, optFunc := range optFuncs {
		optFunc(masterConf)

		if replicaConf != nil {
			optFunc(replicaConf)
		}
	}

	ret := &Conn{
		config: config,
	}

	ret.Conn, err = pgx.ConnectConfig(ctx, masterConf)
	if err != nil {
		return nil, err
	}

	if replicaConf != nil {
		ret.replica, err = pgx.ConnectConfig(ctx, replicaConf)
	}

	return ret, err
}

// Pool represents the master and the replica(s) connections pools.
// The replica pool is optional, while the master is not.
type Pool struct {
	*pgxpool.Pool
	replica *pgxpool.Pool
	config  OnlineConf
}

// Replica returns the replica pool if it's configured.
// The master pool is returned otherwise.
func (p *Pool) Replica() advpg.DB {
	if p.replica != nil {
		return p.replica
	}

	return p.Pool
}

// ReplicaPerTable checks the /table/TableName/force_replica_usage setting in the OnlineConf
// to determine whether the replica should be used.
// The master pool is returned otherwise.
func (p *Pool) ReplicaPerTable(table string) advpg.DB {
	if p.replica == nil || !p.config.GetBool(path.Join("/table", table, "force_replica_usage"), false) {
		return p.Pool
	}

	return p.replica
}

// OnlineConf returns an [OnlineConf] instance passed to [NewPool].
func (p *Pool) OnlineConf() OnlineConf {
	return p.config
}

// PoolOptionFunc represents options for the [NewPool].
// Any user-defined function accepting a pointer to the [pgxpool.Config] can be used as an option.
type PoolOptionFunc func(*pgxpool.Config)

// WithPoolTracers attaches custom tracers to the connection pool config.
func WithPoolTracers(tracers ...pgx.QueryTracer) PoolOptionFunc {
	return func(conf *pgxpool.Config) {
		conf.ConnConfig.Tracer = multitracer.New(append(tracers, conf.ConnConfig.Tracer)...)
	}
}

// WithPoolMetrics enables collection of the query, connection, and pool metrics.
//
// For detailed description of these metrics, see [pgxmetrics.Tracer].
func WithPoolMetrics(ms advmetricsset.Set) PoolOptionFunc {
	return WithPoolTracers(pgxmetrics.New(ms))
}

// NewPool creates the master and optionally the replica(s) connection pool(s).
//
// Use pooled connections for high-availability workloads like web servers.
// For short-living tasks like one-shot cron jobs and CLI tools, use [NewConn].
func NewPool(ctx context.Context, config OnlineConf, optFuncs ...PoolOptionFunc) (*Pool, error) {
	masterConf, replicaConf, err := LoadPoolConfigs(config)
	if err != nil {
		return nil, err
	}

	for _, optFunc := range optFuncs {
		optFunc(masterConf)

		if replicaConf != nil {
			optFunc(replicaConf)
		}
	}

	ret := &Pool{
		config: config,
	}

	ret.Pool, err = pgxpool.NewWithConfig(ctx, masterConf)
	if err != nil {
		return nil, err
	}

	if replicaConf != nil {
		ret.replica, err = pgxpool.NewWithConfig(ctx, replicaConf)
	}

	return ret, err
}

type timeoutTracer struct {
	config      OnlineConf
	prevTimeout atomic.Int64
}

func newTimeoutTracer(config OnlineConf, origTimeout time.Duration) *timeoutTracer {
	ret := &timeoutTracer{config: config}
	ret.prevTimeout.Store(int64(origTimeout))

	return ret
}

func (tt *timeoutTracer) loadTimeout() time.Duration {
	timeout, err := getDur(tt.config, "/timeout", DefaultTimeout)
	if err != nil {
		slog.Warn("advpgconn: timeoutTracer", "err", err) // TODO contextual logging
		return time.Duration(tt.prevTimeout.Load())
	}

	tt.prevTimeout.Store(int64(timeout))

	return timeout
}

func (tt *timeoutTracer) loadTableTimeout(ctx context.Context) time.Duration {
	if qi := pgxmetrics.QueryInfoFromContext(ctx); qi != nil {
		timeout, err := tt.config.GetDurationErr(path.Join("/table", qi.Table, "timeout"))
		if err == nil {
			return timeout
		}

		// there're no per-table last known good timeouts
		if !errors.Is(err, onlineconf.ErrNotFound) {
			slog.Warn("advpgconn: timeoutTracer", "err", err)
		}
	}

	return tt.loadTimeout()
}

type ctxCancel struct{}

func (tt *timeoutTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	ctx, cancel := context.WithTimeout(ctx, tt.loadTableTimeout(ctx))
	ctx = context.WithValue(ctx, ctxCancel{}, cancel)

	return ctx
}

func (*timeoutTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if cancel := ctx.Value(ctxCancel{}); cancel != nil {
		cancel.(context.CancelFunc)()
	}
}

type replicaDB interface {
	Replica() advpg.DB
	ReplicaPerTable(table string) advpg.DB
}

var (
	_ replicaDB = &Conn{}
	_ replicaDB = &Pool{}
)

// ReplicaByOpt returns the master or the replica connection (or pool) based on the [advpg.WithReplica] option
// and the /table/TableName/force_replica_usage setting.
func ReplicaByOpt(db advpg.DB, opt *advpg.SelectOptions, table string) advpg.DB {
	if opt.UseMaster() {
		return db
	}

	repl, ok := db.(replicaDB)
	if !ok {
		return db
	}

	if opt.UseReplica() {
		return repl.Replica()
	}

	return repl.ReplicaPerTable(table)
}

// QueryInfoCtx is just a wrapper for [pgxmetrics.QueryInfo.WithContext].
// QueryInfoCtx is intended to be used from generated code to reduce the import count.
func QueryInfoCtx(ctx context.Context, table, index string) context.Context {
	return (&pgxmetrics.QueryInfo{
		Table: table,
		Index: index,
	}).WithContext(ctx)
}
