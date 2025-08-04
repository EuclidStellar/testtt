package metrics

import (
	"context"
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/yourusername/distributed-cache/internal/cache"
)

// Metrics holds all Prometheus metrics for the cache
type Metrics struct {
	// Cache operation metrics
	cacheOperationDuration *prometheus.HistogramVec
	cacheHits              prometheus.Counter
	cacheMisses            prometheus.Counter
	cacheEvictions         prometheus.Counter
	cacheSize              prometheus.Gauge
	
	// Cluster metrics
	clusterNodes           prometheus.Gauge
	clusterLeader          prometheus.Gauge
	
	// Raft metrics
	raftTerm               prometheus.Gauge
	raftState              *prometheus.GaugeVec
	raftElections          prometheus.Counter
	
	// System metrics
	memoryUsage            prometheus.Gauge
	goroutines             prometheus.Gauge
	
	// HTTP metrics
	httpRequestDuration    *prometheus.HistogramVec
	httpRequestsTotal      *prometheus.CounterVec
}

// NewMetrics creates a new metrics instance
func NewMetrics(nodeID string) *Metrics {
	m := &Metrics{
		cacheOperationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "cache_operation_duration_seconds",
				Help: "Duration of cache operations",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
			},
			[]string{"node_id", "operation"},
		),
		cacheHits: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "cache_hits_total",
				Help: "Total number of cache hits",
				ConstLabels: prometheus.Labels{"node_id": nodeID},
			},
		),
		cacheMisses: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "cache_misses_total",
				Help: "Total number of cache misses",
				ConstLabels: prometheus.Labels{"node_id": nodeID},
			},
		),
		cacheEvictions: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "cache_evictions_total",
				Help: "Total number of cache evictions",
				ConstLabels: prometheus.Labels{"node_id": nodeID},
			},
		),
		cacheSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "cache_size_bytes",
				Help: "Current cache size in bytes",
				ConstLabels: prometheus.Labels{"node_id": nodeID},
			},
		),
		clusterNodes: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "cluster_nodes_total",
				Help: "Total number of nodes in the cluster",
				ConstLabels: prometheus.Labels{"node_id": nodeID},
			},
		),
		clusterLeader: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "cluster_is_leader",
				Help: "Whether this node is the cluster leader (1) or not (0)",
				ConstLabels: prometheus.Labels{"node_id": nodeID},
			},
		),
		raftTerm: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "raft_term",
				Help: "Current Raft term",
				ConstLabels: prometheus.Labels{"node_id": nodeID},
			},
		),
		raftState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "raft_state",
				Help: "Current Raft state (follower=0, candidate=1, leader=2)",
			},
			[]string{"node_id", "state"},
		),
		raftElections: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "raft_elections_total",
				Help: "Total number of Raft elections started",
				ConstLabels: prometheus.Labels{"node_id": nodeID},
			},
		),
		memoryUsage: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "memory_usage_bytes",
				Help: "Current memory usage in bytes",
				ConstLabels: prometheus.Labels{"node_id": nodeID},
			},
		),
		goroutines: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "goroutines_total",
				Help: "Current number of goroutines",
				ConstLabels: prometheus.Labels{"node_id": nodeID},
			},
		),
		httpRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "http_request_duration_seconds",
				Help: "Duration of HTTP requests",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "endpoint", "status"},
		),
		httpRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "endpoint", "status"},
		),
	}
	
	// Register all metrics
	prometheus.MustRegister(
		m.cacheOperationDuration,
		m.cacheHits,
		m.cacheMisses,
		m.cacheEvictions,
		m.cacheSize,
		m.clusterNodes,
		m.clusterLeader,
		m.raftTerm,
		m.raftState,
		m.raftElections,
		m.memoryUsage,
		m.goroutines,
		m.httpRequestDuration,
		m.httpRequestsTotal,
	)
	
	return m
}

// MetricsCollector collects and updates metrics from cache
type MetricsCollector struct {
	metrics   *Metrics
	nodeID    string
	metricsCh <-chan cache.CacheMetric
	stopCh    chan struct{}
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(nodeID string, metricsCh <-chan cache.CacheMetric) *MetricsCollector {
	return &MetricsCollector{
		metrics:   NewMetrics(nodeID),
		nodeID:    nodeID,
		metricsCh: metricsCh,
		stopCh:    make(chan struct{}),
	}
}

// Start begins collecting metrics
func (mc *MetricsCollector) Start(ctx context.Context) {
	go mc.collectMetrics(ctx)
	go mc.updateSystemMetrics(ctx)
}

// Stop stops the metrics collector
func (mc *MetricsCollector) Stop() {
	close(mc.stopCh)
}

// collectMetrics processes cache metrics
func (mc *MetricsCollector) collectMetrics(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-mc.stopCh:
			return
		case metric := <-mc.metricsCh:
			mc.processMetric(metric)
		}
	}
}

// processMetric processes a single cache metric
func (mc *MetricsCollector) processMetric(metric cache.CacheMetric) {
	switch metric.Type {
	case "cache_operation_duration":
		operation := metric.Labels["operation"]
		mc.metrics.cacheOperationDuration.WithLabelValues(mc.nodeID, operation).Observe(metric.Value)
		
	case "cache_hits":
		mc.metrics.cacheHits.Add(metric.Value)
		
	case "cache_misses":
		mc.metrics.cacheMisses.Add(metric.Value)
		
	case "cache_evictions":
		mc.metrics.cacheEvictions.Add(metric.Value)
		
	case "cache_size":
		mc.metrics.cacheSize.Set(metric.Value)
		
	case "cluster_nodes":
		mc.metrics.clusterNodes.Set(metric.Value)
		
	case "cluster_leader":
		mc.metrics.clusterLeader.Set(metric.Value)
		
	case "raft_term":
		mc.metrics.raftTerm.Set(metric.Value)
		
	case "raft_state":
		state := metric.Labels["state"]
		mc.metrics.raftState.WithLabelValues(mc.nodeID, state).Set(metric.Value)
		
	case "raft_elections":
		mc.metrics.raftElections.Add(metric.Value)
	}
}

// updateSystemMetrics updates system-level metrics
func (mc *MetricsCollector) updateSystemMetrics(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-mc.stopCh:
			return
		case <-ticker.C:
			mc.updateMemoryUsage()
			mc.updateGoroutineCount()
		}
	}
}

// updateMemoryUsage updates memory usage metrics
func (mc *MetricsCollector) updateMemoryUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	mc.metrics.memoryUsage.Set(float64(m.Alloc))
}

// updateGoroutineCount updates goroutine count metrics
func (mc *MetricsCollector) updateGoroutineCount() {
	mc.metrics.goroutines.Set(float64(runtime.NumGoroutine()))
}

// HTTPMiddleware provides HTTP metrics middleware
func (mc *MetricsCollector) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Wrap the response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}
		
		next.ServeHTTP(wrapped, r)
		
		duration := time.Since(start).Seconds()
		status := http.StatusText(wrapped.statusCode)
		
		mc.metrics.httpRequestDuration.WithLabelValues(
			r.Method,
			r.URL.Path,
			status,
		).Observe(duration)
		
		mc.metrics.httpRequestsTotal.WithLabelValues(
			r.Method,
			r.URL.Path,
			status,
		).Inc()
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// StartMetricsServer starts the Prometheus metrics HTTP server
func StartMetricsServer(ctx context.Context, port string, collector *MetricsCollector) {
	mux := http.NewServeMux()
	
	// Add metrics middleware
	handler := collector.HTTPMiddleware(promhttp.Handler())
	mux.Handle("/metrics", handler)
	
	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	
	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}
	
	go func() {
		log.Printf("Starting metrics server on port %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()
	
	// Graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Metrics server shutdown error: %v", err)
		}
	}()
}
