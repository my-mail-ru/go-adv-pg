package advpgconn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/onlineconf/onlineconf-go/v2"
)

const (
	DefaultTimeout                   = 30 * time.Second
	DefaultPoolMaxConnLifetime       = 0
	DefaultPoolMaxConnLifetimeJitter = 0
	DefaultMaxConnIdleTime           = 0
	DefaultPingTimeout               = 0
	DefaultHealthcheckPeriod         = 0
	DefaultPoolSize                  = 10
	DefaultPoolMinSize               = 1
	DefaultPoolIdleConns             = 0
)

type Config interface {
	Path(string) string
	GetBool(string, bool) bool
	GetStringErr(string) (string, error)
	GetIntErr(string) (int, error)
	GetDurationErr(string) (time.Duration, error)
}

var replicasRegexp = regexp.MustCompile(`\s*[,;]\s*`)

func LoadConnConfig(config Config) (masterConf, replicaConf *pgx.ConnConfig, err error) {
	masterConf = &pgx.ConnConfig{}

	hostPort, err := config.GetStringErr("/host")
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", config.Path("/host"), err)
	}

	masterConf.Host, masterConf.Port, err = parseHostPort(hostPort, "/host", config)
	if err != nil {
		return nil, nil, err
	}

	base, err := config.GetStringErr("/base")
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", config.Path("/base"), err)
	}

	masterConf.Database = base

	masterConf.User, err = config.GetStringErr("/user")
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", config.Path("/user"), err)
	}

	masterConf.Password, err = config.GetStringErr("/password")
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", config.Path("/password"), err)
	}

	timeout, err := getDur(config, "/timeout", DefaultTimeout)
	if err != nil {
		return nil, nil, err
	}

	masterConf.ConnectTimeout, err = getDur(config, "/connect_timeout", timeout)
	if err != nil {
		return nil, nil, err
	}

	tracer := newTimeoutTracer(config, timeout)
	masterConf.Tracer = tracer

	masterConf.AfterConnect = func(ctx context.Context, conn *pgconn.PgConn) error {
		if !config.GetBool("/set_statement_timeout", false) {
			return nil
		}

		timeout := tracer.loadTimeout()

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		query := "SET statement_timeout = " + strconv.FormatInt(timeout.Milliseconds(), 10)
		result := conn.Exec(ctx, query)

		if err := result.Close(); err != nil {
			return fmt.Errorf("%q: %w", query, err)
		}

		return nil
	}

	masterConf.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	replicasStr, err := config.GetStringErr("/replica")
	if err != nil {
		if errors.Is(err, onlineconf.ErrNotFound) {
			return masterConf, nil, nil
		}

		return nil, nil, fmt.Errorf("%s: %w", config.Path("/replica"), err)
	}

	replicas := replicasRegexp.Split(replicasStr, -1)
	if len(replicas) == 0 {
		return nil, nil, fmt.Errorf("%s: replica list is empty", config.Path("/replica"))
	}

	replicaConf = masterConf.Copy()

	replicaConf.Host, replicaConf.Port, err = parseHostPort(replicas[0], "/replica", config)
	if err != nil {
		return nil, nil, err
	}

	for _, replica := range replicas[1:] {
		fbc := &pgconn.FallbackConfig{}

		fbc.Host, fbc.Port, err = parseHostPort(replica, "/replica", config)
		if err != nil {
			return nil, nil, err
		}

		replicaConf.Fallbacks = append(replicaConf.Fallbacks, fbc)
	}

	return masterConf, replicaConf, nil
}

func parseHostPort(hostPort, path string, config Config) (string, uint16, error) {
	host, portStr, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "", 0, fmt.Errorf("%s: %s: %w", config.Path(path), hostPort, err)
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("%s: %s: %w", config.Path(path), portStr, err)
	}

	return host, uint16(port), nil
}

func LoadPoolConfig(config Config) (masterConf, replicaConf *pgxpool.Config, err error) {
	masterConnConf, replicaConnConf, err := LoadConnConfig(config)
	if err != nil {
		return nil, nil, err
	}

	masterConf = &pgxpool.Config{
		ConnConfig: masterConnConf,
	}

	masterConf.MaxConnLifetime, err = getDur(config, "/pool_max_conn_lifetime", DefaultPoolMaxConnLifetime)
	if err != nil {
		return nil, nil, err
	}

	masterConf.MaxConnLifetimeJitter, err = getDur(config, "/pool_max_conn_lifetime_jitter", DefaultPoolMaxConnLifetimeJitter)
	if err != nil {
		return nil, nil, err
	}

	masterConf.MaxConnIdleTime, err = getDur(config, "/pool_max_conn_idle_time", DefaultMaxConnIdleTime)
	if err != nil {
		return nil, nil, err
	}

	masterConf.PingTimeout, err = getDur(config, "/pool_ping_timeout", DefaultPingTimeout)
	if err != nil {
		return nil, nil, err
	}

	masterConf.HealthCheckPeriod, err = getDur(config, "/pool_healthcheck_period", DefaultHealthcheckPeriod)
	if err != nil {
		return nil, nil, err
	}

	masterConf.MaxConns, err = getInt32(config, "/pool_size", DefaultPoolSize)
	if err != nil {
		return nil, nil, err
	}

	masterConf.MinConns, err = getInt32(config, "/pool_min_size", DefaultPoolMinSize)
	if err != nil {
		return nil, nil, err
	}

	masterConf.MinIdleConns, err = getInt32(config, "/pool_min_idle_conns", DefaultPoolIdleConns)
	if err != nil {
		return nil, nil, err
	}

	if replicaConnConf == nil {
		return masterConf, nil, nil
	}

	replicaConf = masterConf.Copy()
	replicaConf.ConnConfig = replicaConnConf

	return masterConf, replicaConf, nil
}

func getDur(config Config, path string, dfl time.Duration) (time.Duration, error) {
	dur, err := config.GetDurationErr(path)
	if err == nil {
		return dur, nil
	}

	if !errors.Is(err, onlineconf.ErrNotFound) {
		return 0, err
	}

	return dfl, nil
}

func getInt32(config Config, path string, dfl int32) (int32, error) {
	i, err := config.GetDurationErr(path)
	if err == nil {
		return int32(i), nil
	}

	if !errors.Is(err, onlineconf.ErrNotFound) {
		return 0, err
	}

	return dfl, nil
}
