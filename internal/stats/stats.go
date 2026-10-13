package stats

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "pgx_stats"
	subsystem = "conns"
)

type StatsGetter interface {
	Stat() *pgxpool.Stat
}

type StatsCollector struct {
	sg                       StatsGetter
	acquireCountDesc         *prometheus.Desc
	acquireDurationDesc      *prometheus.Desc
	acquiredConnsDesc        *prometheus.Desc
	canceledAcquireCountDesc *prometheus.Desc
	constructingConnsDesc    *prometheus.Desc
	emptyAcquireCountDesc    *prometheus.Desc
	idleConnsDesc            *prometheus.Desc
	maxConnsDesc             *prometheus.Desc
	totalConnsDesc           *prometheus.Desc
}

func NewStatsCollector(conn, dbName string, stats StatsGetter) *StatsCollector {
	labels := map[string]string{"db_name": dbName, "conn": conn}

	return &StatsCollector{
		sg: stats,
		acquireCountDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, "acquire_total"),
			"Cumulative count of successful acquires from the pool.",
			nil, labels,
		),
		acquireDurationDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, "acquire_duration_seconds"),
			"Total duration of all successful acquires from the pool.",
			nil, labels,
		),
		acquiredConnsDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, "acquired"),
			"Number of currently acquired connections in the pool.",
			nil, labels,
		),
		canceledAcquireCountDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, "canceled_acquire_total"),
			"Cumulative count of acquires from the pool that were canceled by a context.",
			nil, labels,
		),
		constructingConnsDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, "constructing"),
			"Number of conns with constructing status in the pool.",
			nil, labels,
		),
		emptyAcquireCountDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, "empty_acquire_total"),
			"Cumulative count of successful acquires from the pool that waited for a new connection.",
			nil, labels,
		),
		idleConnsDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, "idle"),
			"Number of currently idle conns in the pool.",
			nil, labels,
		),
		maxConnsDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, "max"),
			"Maximum size of the pool.",
			nil, labels,
		),
		totalConnsDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, "total"),
			"Total number of resources currently in the pool.",
			nil, labels,
		),
	}
}

func (c *StatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquireCountDesc
	ch <- c.acquireDurationDesc
	ch <- c.acquiredConnsDesc
	ch <- c.canceledAcquireCountDesc
	ch <- c.constructingConnsDesc
	ch <- c.emptyAcquireCountDesc
	ch <- c.idleConnsDesc
	ch <- c.maxConnsDesc
	ch <- c.totalConnsDesc
}

func (c *StatsCollector) Collect(ch chan<- prometheus.Metric) {
	poolStat := c.sg.Stat()

	sendCounter := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.CounterValue, v)
	}

	sendGauge := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	sendCounter(c.acquireCountDesc, float64(poolStat.AcquireCount()))
	sendCounter(c.acquireDurationDesc, poolStat.AcquireDuration().Seconds())
	sendGauge(c.acquiredConnsDesc, float64(poolStat.AcquiredConns()))
	sendCounter(c.canceledAcquireCountDesc, float64(poolStat.CanceledAcquireCount()))
	sendGauge(c.constructingConnsDesc, float64(poolStat.ConstructingConns()))
	sendCounter(c.emptyAcquireCountDesc, float64(poolStat.EmptyAcquireCount()))
	sendGauge(c.idleConnsDesc, float64(poolStat.IdleConns()))
	sendGauge(c.maxConnsDesc, float64(poolStat.MaxConns()))
	sendGauge(c.totalConnsDesc, float64(poolStat.TotalConns()))
}
