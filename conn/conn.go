package advpgconn

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Conn struct {
	*pgx.Conn
	replica *pgx.Conn
	config  Config
}

func (c *Conn) Replica() *pgx.Conn {
	if c.replica != nil {
		return c.replica
	}

	return c.Conn
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

type TimeoutTracer struct {
	timeout time.Duration
}

type ctxCancel struct{}

func (tt *TimeoutTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	ctx, cancel := context.WithTimeout(ctx, tt.timeout)
	ctx = context.WithValue(ctx, ctxCancel{}, cancel)

	return ctx
}

func (*TimeoutTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if cancel := ctx.Value(ctxCancel{}); cancel != nil {
		cancel.(context.CancelFunc)()
	}
}
