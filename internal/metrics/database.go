package metrics

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
)

type databaseCollector struct {
	stats            func() sql.DBStats
	connections      *prometheus.Desc
	maxOpen          *prometheus.Desc
	waits            *prometheus.Desc
	waitDuration     *prometheus.Desc
	connectionsClose *prometheus.Desc
}

func newDatabaseCollector(stats func() sql.DBStats) *databaseCollector {
	return &databaseCollector{
		stats: stats,
		connections: prometheus.NewDesc(
			"loomarr_database_connections", "Database connections by current pool state.",
			[]string{"state"}, nil,
		),
		maxOpen: prometheus.NewDesc(
			"loomarr_database_max_open_connections", "Configured maximum open database connections.",
			nil, nil,
		),
		waits: prometheus.NewDesc(
			"loomarr_database_connection_waits_total", "Database connection acquisitions that waited for capacity.",
			nil, nil,
		),
		waitDuration: prometheus.NewDesc(
			"loomarr_database_connection_wait_duration_seconds_total", "Total time spent waiting for database connection capacity.",
			nil, nil,
		),
		connectionsClose: prometheus.NewDesc(
			"loomarr_database_connections_closed_total", "Database connections closed by pool policy.",
			[]string{"reason"}, nil,
		),
	}
}

func (c *databaseCollector) Describe(descriptions chan<- *prometheus.Desc) {
	descriptions <- c.connections
	descriptions <- c.maxOpen
	descriptions <- c.waits
	descriptions <- c.waitDuration
	descriptions <- c.connectionsClose
}

func (c *databaseCollector) Collect(metrics chan<- prometheus.Metric) {
	stats := c.stats()
	for state, value := range map[string]int{
		"open": stats.OpenConnections, "in_use": stats.InUse, "idle": stats.Idle,
	} {
		metrics <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(value), state)
	}
	metrics <- prometheus.MustNewConstMetric(c.maxOpen, prometheus.GaugeValue, float64(stats.MaxOpenConnections))
	metrics <- prometheus.MustNewConstMetric(c.waits, prometheus.CounterValue, float64(stats.WaitCount))
	metrics <- prometheus.MustNewConstMetric(
		c.waitDuration, prometheus.CounterValue, stats.WaitDuration.Seconds(),
	)
	for reason, value := range map[string]int64{
		"idle_limit": stats.MaxIdleClosed,
		"idle_time":  stats.MaxIdleTimeClosed,
		"lifetime":   stats.MaxLifetimeClosed,
	} {
		metrics <- prometheus.MustNewConstMetric(
			c.connectionsClose, prometheus.CounterValue, float64(value), reason,
		)
	}
}
