package advpgconn

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/onlineconf/onlineconf-go/v2"
)

// Defaults for connection settings
const (
	DefaultTimeout                   = 30 * time.Second
	DefaultPoolMaxConns              = 10
	DefaultPoolMinConns              = 1
	DefaultPoolMinIdleConns          = 0
	DefaultPoolMaxConnLifetime       = 0
	DefaultPoolMaxConnLifetimeJitter = 0
	DefaultHealthcheckPeriod         = 0
	DefaultMaxConnIdleTime           = 0
	DefaultPingTimeout               = 0
)

// OnlineConf dependency. You can use Module, Subtree or any other compatible type.
type OnlineConf interface {
	Path(string) string
	GetBool(string, bool) bool
	GetStringErr(string) (string, error)
	GetIntErr(string) (int, error)
	GetDurationErr(string) (time.Duration, error)
}

var replicasRegexp = regexp.MustCompile(`\s*[,;]\s*`)

func loadConnStrings(config OnlineConf) (masterConnString, replicaConnString *strings.Builder, timeout time.Duration, err error) {
	hostPort, err := config.GetStringErr("/host")
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%s: %w", config.Path("/host"), err)
	}

	base, err := config.GetStringErr("/base")
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%s: %w", config.Path("/base"), err)
	}

	user, err := config.GetStringErr("/user")
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%s: %w", config.Path("/user"), err)
	}

	pass, err := config.GetStringErr("/pass")
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%s: %w", config.Path("/pass"), err)
	}

	timeout, err = getDur(config, "/timeout", DefaultTimeout)
	if err != nil {
		return nil, nil, 0, err
	}

	connectTimeout, err := getDur(config, "/connect_timeout", timeout)
	if err != nil {
		return nil, nil, 0, err
	}

	masterConnString = buildConnString(user, pass, []string{hostPort}, base, connectTimeout)

	replicasStr, err := config.GetStringErr("/replica")
	if err != nil {
		if errors.Is(err, onlineconf.ErrNotFound) {
			return masterConnString, nil, 0, nil
		}

		return nil, nil, 0, fmt.Errorf("%s: %w", config.Path("/replica"), err)
	}

	replicas := replicasRegexp.Split(replicasStr, -1)
	if len(replicas) == 0 {
		return nil, nil, 0, fmt.Errorf("%s: replica list is specified but is empty", config.Path("/replica"))
	}

	replicas = append(replicas, hostPort) // master fallback

	return masterConnString, buildConnString(user, pass, replicas, base, connectTimeout), timeout, nil
}

func buildConnString(user, pass string, hostPorts []string, base string, connectTimeout time.Duration) *strings.Builder {
	ret := &strings.Builder{}

	ret.WriteString("postgresql://")
	ret.WriteString(user)
	ret.WriteByte(':')
	ret.WriteString(pass)
	ret.WriteByte('@')

	for i, hostPort := range hostPorts {
		if i != 0 {
			ret.WriteByte(',')
		}

		ret.WriteString(hostPort)
	}

	ret.WriteByte('/')
	ret.WriteString(base)
	ret.WriteString("?default_query_exec_mode=simple_protocol")

	if connectTimeout != 0 {
		// XXX parseConnectTimeoutSetting is ill
		appendParam("&connect_timeout=", strconv.FormatInt(int64(connectTimeout/time.Second), 10), ret)
	}

	return ret
}

func attachTimeoutTracer(config OnlineConf, connConfig *pgx.ConnConfig, timeout time.Duration) {
	tracer := newTimeoutTracer(config, timeout)
	connConfig.Tracer = tracer

	connConfig.AfterConnect = func(ctx context.Context, conn *pgconn.PgConn) error {
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
}

// LoadConnConfigs loads the master and the replica(s) configs from the OnlineConf.
func LoadConnConfigs(config OnlineConf) (masterConf, replicaConf *pgx.ConnConfig, err error) {
	masterConnString, replicaConnString, timeout, err := loadConnStrings(config)
	if err != nil {
		return nil, nil, err
	}

	masterConf, err = pgx.ParseConfig(masterConnString.String())
	if err != nil {
		return nil, nil, err
	}

	attachTimeoutTracer(config, masterConf, timeout)

	if replicaConnString != nil {
		replicaConf, err = pgx.ParseConfig(masterConnString.String())
		if err != nil {
			return nil, nil, err
		}

		replicaConf.Tracer = masterConf.Tracer
		replicaConf.AfterConnect = masterConf.AfterConnect
	}

	return masterConf, replicaConf, nil
}

func loadPoolConnStrings(config OnlineConf) (masterConnString, replicaConnString *strings.Builder, timeout time.Duration, err error) {
	masterConnString, replicaConnString, timeout, err = loadConnStrings(config)
	if err != nil {
		return nil, nil, 0, err
	}

	maxConns, err := getInt(config, "/pool_max_conns", -1)
	if err != nil {
		return nil, nil, 0, err
	}

	if maxConns == -1 {
		maxConns, err = getInt(config, "/pool_size", DefaultPoolMaxConns) // perl-compatible
		if err != nil {
			return nil, nil, 0, err
		}
	}

	appendParam("&pool_max_conns=", strconv.Itoa(maxConns), masterConnString, replicaConnString)

	minConns, err := getInt(config, "/pool_min_conns", DefaultPoolMinConns)
	if err != nil {
		return nil, nil, 0, err
	}

	if minConns != 0 {
		appendParam("&pool_min_conns=", strconv.Itoa(minConns), masterConnString, replicaConnString)
	}

	minIdleConns, err := getInt(config, "/pool_min_idle_conns", DefaultPoolMinIdleConns)
	if err != nil {
		return nil, nil, 0, err
	}

	if minIdleConns != 0 {
		appendParam("&pool_min_idle_conns=", strconv.Itoa(minIdleConns), masterConnString, replicaConnString)
	}

	maxConnLifetime, err := getDur(config, "/pool_max_conn_lifetime", DefaultPoolMaxConnLifetime)
	if err != nil {
		return nil, nil, 0, err
	}

	if maxConnLifetime != 0 {
		appendParam("&pool_max_conn_lifetime=", maxConnLifetime.String(), masterConnString, replicaConnString)
	}

	maxConnLifetimeJitter, err := getDur(config, "/pool_max_conn_lifetime_jitter", DefaultPoolMaxConnLifetimeJitter)
	if err != nil {
		return nil, nil, 0, err
	}

	if maxConnLifetimeJitter != 0 {
		appendParam("&pool_max_conn_lifetime_jitter=", maxConnLifetimeJitter.String(), masterConnString, replicaConnString)
	}

	healthCheckPeriod, err := getDur(config, "/pool_health_check_period", DefaultHealthcheckPeriod)
	if err != nil {
		return nil, nil, 0, err
	}

	if healthCheckPeriod != 0 {
		appendParam("&pool_health_check_period=", healthCheckPeriod.String(), masterConnString, replicaConnString)
	}

	return masterConnString, replicaConnString, timeout, nil
}

func appendParam(key, val string, builders ...*strings.Builder) {
	for _, builder := range builders {
		if builder != nil {
			builder.WriteString(key)
			builder.WriteString(val)
		}
	}
}

// LoadPoolConfigs loads the master and the replica(s) pool configs from the OnlineConf.
func LoadPoolConfigs(config OnlineConf) (masterConf, replicaConf *pgxpool.Config, err error) {
	masterConnString, replicaConnString, timeout, err := loadPoolConnStrings(config)
	if err != nil {
		return nil, nil, err
	}

	masterConf, err = pgxpool.ParseConfig(masterConnString.String())
	if err != nil {
		return nil, nil, err
	}

	attachTimeoutTracer(config, masterConf.ConnConfig, timeout)

	masterConf.MaxConnIdleTime, err = getDur(config, "/pool_max_conn_idle_time", DefaultMaxConnIdleTime)
	if err != nil {
		return nil, nil, err
	}

	masterConf.PingTimeout, err = getDur(config, "/pool_ping_timeout", DefaultPingTimeout)
	if err != nil {
		return nil, nil, err
	}

	if replicaConnString != nil {
		replicaConf, err = pgxpool.ParseConfig(replicaConnString.String())
		if err != nil {
			return nil, nil, err
		}

		replicaConf.ConnConfig.Tracer = masterConf.ConnConfig.Tracer
		replicaConf.ConnConfig.AfterConnect = masterConf.ConnConfig.AfterConnect
		replicaConf.MaxConnIdleTime = masterConf.MaxConnIdleTime
		replicaConf.PingTimeout = masterConf.PingTimeout
	}

	return masterConf, replicaConf, nil
}

func getDur(config OnlineConf, path string, dfl time.Duration) (time.Duration, error) {
	dur, err := config.GetDurationErr(path)
	if err == nil {
		return dur, nil
	}

	if !errors.Is(err, onlineconf.ErrNotFound) {
		return 0, err
	}

	return dfl, nil
}

func getInt(config OnlineConf, path string, dfl int) (int, error) {
	i, err := config.GetIntErr(path)
	if err == nil {
		return i, nil
	}

	if !errors.Is(err, onlineconf.ErrNotFound) {
		return 0, err
	}

	return dfl, nil
}
