package advpgconn

import (
	"context"
	"errors"
	"log/slog"
	"path"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/onlineconf/onlineconf-go/v2"
)

type Conn struct {
	*pgx.Conn
	replica *pgx.Conn
	config  Config
}

func (c *Conn) Replica() *pgx.Conn {
	if c.replica == nil {
		return c.Conn
	}

	return c.replica
}

func (c *Conn) ReplicaPerTable(table string) *pgx.Conn {
	if c.replica == nil || !c.config.GetBool(path.Join("/table", table, "force_replica_usage"), false) {
		return c.Conn
	}

	return c.replica
}

func (c *Conn) Config() Config {
	return c.config
}

func NewConn(ctx context.Context, config Config) (*Conn, error) {
	masterConf, replicaConf, err := LoadConnConfig(config)
	if err != nil {
		return nil, err
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

type Pool struct {
	*pgxpool.Pool
	replica *pgxpool.Pool
	config  Config
}

func (p *Pool) Replica() *pgxpool.Pool {
	if p.replica != nil {
		return p.replica
	}

	return p.Pool
}

func (p *Pool) Config() Config {
	return p.config
}

func NewPool(ctx context.Context, config Config) (*Pool, error) {
	masterConf, replicaConf, err := LoadPoolConfig(config)
	if err != nil {
		return nil, err
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
	config      Config
	prevTimeout atomic.Int64
}

func newTimeoutTracer(config Config, origTimeout time.Duration) *timeoutTracer {
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
	if qi := QueryInfoFromContext(ctx); qi != nil {
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
