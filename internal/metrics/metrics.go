package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry     *prometheus.Registry
	requests     *prometheus.CounterVec
	duration     *prometheus.HistogramVec
	inFlight     prometheus.Gauge
	redisErrors  *prometheus.CounterVec
	cacheResults *prometheus.CounterVec
}

func New() *Metrics {
	m := &Metrics{
		registry:     prometheus.NewRegistry(),
		requests:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "dialplan_manager_stats_http_requests_total", Help: "Total HTTP requests."}, []string{"method", "path", "status"}),
		duration:     prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "dialplan_manager_stats_http_request_duration_seconds", Help: "HTTP request duration.", Buckets: prometheus.DefBuckets}, []string{"method", "path", "status"}),
		inFlight:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "dialplan_manager_stats_http_requests_in_flight", Help: "HTTP requests currently being served."}),
		redisErrors:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "dialplan_manager_stats_redis_errors_total", Help: "Redis errors by operation."}, []string{"operation"}),
		cacheResults: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "dialplan_manager_stats_cache_results_total", Help: "Cache results by resource and result."}, []string{"resource", "result"}),
	}
	m.registry.MustRegister(m.requests, m.duration, m.inFlight, m.redisErrors, m.cacheResults)
	m.registry.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	return m
}

func (m *Metrics) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		m.inFlight.Inc()
		defer m.inFlight.Dec()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())
		m.requests.WithLabelValues(c.Request.Method, path, status).Inc()
		m.duration.WithLabelValues(c.Request.Method, path, status).Observe(time.Since(start).Seconds())
	}
}

func (m *Metrics) Handler() gin.HandlerFunc {
	handler := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
	return gin.WrapH(handler)
}

func (m *Metrics) RedisError(operation string) { m.redisErrors.WithLabelValues(operation).Inc() }
func (m *Metrics) Cache(resource, result string) {
	m.cacheResults.WithLabelValues(resource, result).Inc()
}
