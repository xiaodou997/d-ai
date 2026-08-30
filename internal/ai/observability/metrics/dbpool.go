package metrics

import (
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// dbPoolCollector exposes the pgx pool gauges/counters that are useful when a
// request or worker SLO starts to degrade. The collector keeps a stable
// registration while the composition root supplies the live pools once they
// have been opened.
type dbPoolCollector struct {
	mu      sync.RWMutex
	entries map[string]*pgxpool.Pool
}

var (
	dbPoolCollectorOnce sync.Once
	dbPools             = &dbPoolCollector{entries: make(map[string]*pgxpool.Pool)}
)

var dbPoolDescs = map[string]*prometheus.Desc{
	"acquired": prometheus.NewDesc(
		"dai_db_pool_acquired_connections",
		"Number of connections currently acquired from the PostgreSQL pool.",
		[]string{"pool"}, nil,
	),
	"idle": prometheus.NewDesc(
		"dai_db_pool_idle_connections",
		"Number of idle connections currently held by the PostgreSQL pool.",
		[]string{"pool"}, nil,
	),
	"total": prometheus.NewDesc(
		"dai_db_pool_total_connections",
		"Number of connections currently held by the PostgreSQL pool.",
		[]string{"pool"}, nil,
	),
	"max": prometheus.NewDesc(
		"dai_db_pool_max_connections",
		"Configured maximum number of connections for the PostgreSQL pool.",
		[]string{"pool"}, nil,
	),
	"constructing": prometheus.NewDesc(
		"dai_db_pool_constructing_connections",
		"Number of connections currently being constructed.",
		[]string{"pool"}, nil,
	),
	"acquire_count": prometheus.NewDesc(
		"dai_db_pool_acquires_total",
		"Total number of successful PostgreSQL pool acquires.",
		[]string{"pool"}, nil,
	),
	"canceled_acquire": prometheus.NewDesc(
		"dai_db_pool_canceled_acquires_total",
		"Total number of canceled PostgreSQL pool acquires.",
		[]string{"pool"}, nil,
	),
	"empty_acquire": prometheus.NewDesc(
		"dai_db_pool_empty_acquires_total",
		"Total number of PostgreSQL pool acquires that waited for an empty pool.",
		[]string{"pool"}, nil,
	),
	"acquire_duration": prometheus.NewDesc(
		"dai_db_pool_acquire_duration_seconds_total",
		"Total time spent acquiring PostgreSQL pool connections.",
		[]string{"pool"}, nil,
	),
}

// RegisterDBPools attaches the runtime and billing pools to the shared
// Prometheus registry. Calling it more than once replaces the live references
// without attempting to register duplicate collectors.
func RegisterDBPools(runtime, billing *pgxpool.Pool) {
	dbPoolCollectorOnce.Do(func() {
		_ = prometheus.Register(dbPools)
	})
	dbPools.mu.Lock()
	defer dbPools.mu.Unlock()
	dbPools.entries = make(map[string]*pgxpool.Pool, 2)
	if runtime != nil {
		dbPools.entries["runtime"] = runtime
	}
	if billing != nil {
		if billing == runtime {
			dbPools.entries["billing"] = billing
		} else {
			dbPools.entries["billing"] = billing
		}
	}
}

func (c *dbPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range dbPoolDescs {
		ch <- desc
	}
}

func (c *dbPoolCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	entries := make(map[string]*pgxpool.Pool, len(c.entries))
	for name, pool := range c.entries {
		entries[name] = pool
	}
	c.mu.RUnlock()

	for name, pool := range entries {
		if pool == nil {
			continue
		}
		stat := pool.Stat()
		ch <- prometheus.MustNewConstMetric(dbPoolDescs["acquired"], prometheus.GaugeValue, float64(stat.AcquiredConns()), name)
		ch <- prometheus.MustNewConstMetric(dbPoolDescs["idle"], prometheus.GaugeValue, float64(stat.IdleConns()), name)
		ch <- prometheus.MustNewConstMetric(dbPoolDescs["total"], prometheus.GaugeValue, float64(stat.TotalConns()), name)
		ch <- prometheus.MustNewConstMetric(dbPoolDescs["max"], prometheus.GaugeValue, float64(stat.MaxConns()), name)
		ch <- prometheus.MustNewConstMetric(dbPoolDescs["constructing"], prometheus.GaugeValue, float64(stat.ConstructingConns()), name)
		ch <- prometheus.MustNewConstMetric(dbPoolDescs["acquire_count"], prometheus.CounterValue, float64(stat.AcquireCount()), name)
		ch <- prometheus.MustNewConstMetric(dbPoolDescs["canceled_acquire"], prometheus.CounterValue, float64(stat.CanceledAcquireCount()), name)
		ch <- prometheus.MustNewConstMetric(dbPoolDescs["empty_acquire"], prometheus.CounterValue, float64(stat.EmptyAcquireCount()), name)
		ch <- prometheus.MustNewConstMetric(dbPoolDescs["acquire_duration"], prometheus.CounterValue, stat.AcquireDuration().Seconds(), name)
	}
}
