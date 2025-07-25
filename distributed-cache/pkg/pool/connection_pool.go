package pool

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Connection represents a pooled connection
type Connection interface {
	// Write sends data to the connection
	Write(data []byte) (int, error)
	// Read reads data from the connection
	Read(data []byte) (int, error)
	// Close closes the connection
	Close() error
	// IsAlive checks if connection is still alive
	IsAlive() bool
	// LastUsed returns the last time this connection was used
	LastUsed() time.Time
	// MarkUsed updates the last used time
	MarkUsed()
}

// PooledConnection wraps a network connection with pool metadata
type PooledConnection struct {
	conn     net.Conn
	pool     *ConnectionPool
	lastUsed int64 // Unix timestamp
	inUse    int32 // 0 = free, 1 = in use
}

func (pc *PooledConnection) Write(data []byte) (int, error) {
	atomic.StoreInt64(&pc.lastUsed, time.Now().Unix())
	return pc.conn.Write(data)
}

func (pc *PooledConnection) Read(data []byte) (int, error) {
	atomic.StoreInt64(&pc.lastUsed, time.Now().Unix())
	return pc.conn.Read(data)
}

func (pc *PooledConnection) Close() error {
	if pc.pool != nil {
		pc.pool.Put(pc)
		return nil
	}
	return pc.conn.Close()
}

func (pc *PooledConnection) IsAlive() bool {
	if pc.conn == nil {
		return false
	}

	// Set a short deadline for the test
	pc.conn.SetReadDeadline(time.Now().Add(time.Millisecond))
	defer pc.conn.SetReadDeadline(time.Time{})

	// Try to read 0 bytes
	_, err := pc.conn.Read(make([]byte, 0))
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return true // Timeout means connection is alive
		}
		return false
	}
	return true
}

func (pc *PooledConnection) LastUsed() time.Time {
	return time.Unix(atomic.LoadInt64(&pc.lastUsed), 0)
}

func (pc *PooledConnection) MarkUsed() {
	atomic.StoreInt64(&pc.lastUsed, time.Now().Unix())
}

// ConnectionFactory creates new connections
type ConnectionFactory interface {
	CreateConnection(address string) (net.Conn, error)
}

// TCPConnectionFactory creates TCP connections
type TCPConnectionFactory struct {
	timeout time.Duration
}

func NewTCPConnectionFactory(timeout time.Duration) *TCPConnectionFactory {
	return &TCPConnectionFactory{timeout: timeout}
}

func (tcf *TCPConnectionFactory) CreateConnection(address string) (net.Conn, error) {
	return net.DialTimeout("tcp", address, tcf.timeout)
}

// PoolConfig holds configuration for connection pool
type PoolConfig struct {
	MaxConnections    int
	MinConnections    int
	MaxIdleTime      time.Duration
	ConnectionTimeout time.Duration
	IdleCheckInterval time.Duration
	TestOnBorrow     bool
	TestOnReturn     bool
}

// DefaultPoolConfig returns a default pool configuration
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MaxConnections:    100,
		MinConnections:    5,
		MaxIdleTime:      30 * time.Minute,
		ConnectionTimeout: 5 * time.Second,
		IdleCheckInterval: 1 * time.Minute,
		TestOnBorrow:     true,
		TestOnReturn:     false,
	}
}

// ConnectionPool manages a pool of connections
type ConnectionPool struct {
	address    string
	factory    ConnectionFactory
	config     *PoolConfig

	mu         sync.RWMutex
	connections chan *PooledConnection
	allConns   map[*PooledConnection]bool

	// Statistics
	stats      PoolStats

	// Lifecycle
	ctx        context.Context
	cancel     context.CancelFunc
	closed     int32
}

// PoolStats holds pool statistics
type PoolStats struct {
	TotalConnections   int64
	ActiveConnections  int64
	IdleConnections    int64
	RequestsTotal      int64
	RequestsSuccessful int64
	RequestsFailed     int64
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(address string, factory ConnectionFactory, config *PoolConfig) *ConnectionPool {
	if config == nil {
		config = DefaultPoolConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &ConnectionPool{
		address:     address,
		factory:     factory,
		config:      config,
		connections: make(chan *PooledConnection, config.MaxConnections),
		allConns:    make(map[*PooledConnection]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Initialize minimum connections
	go pool.initialize()

	// Start idle connection cleaner
	go pool.cleanupIdleConnections()

	return pool
}

func (cp *ConnectionPool) initialize() {
	for i := 0; i < cp.config.MinConnections; i++ {
		if conn, err := cp.createConnection(); err == nil {
			cp.connections <- conn
		}
	}
}

func (cp *ConnectionPool) createConnection() (*PooledConnection, error) {
	conn, err := cp.factory.CreateConnection(cp.address)
	if err != nil {
		return nil, err
	}

	pooledConn := &PooledConnection{
		conn:     conn,
		pool:     cp,
		lastUsed: time.Now().Unix(),
	}

	cp.mu.Lock()
	cp.allConns[pooledConn] = true
	atomic.AddInt64(&cp.stats.TotalConnections, 1)
	cp.mu.Unlock()

	return pooledConn, nil
}

// Get retrieves a connection from the pool
func (cp *ConnectionPool) Get() (*PooledConnection, error) {
	if atomic.LoadInt32(&cp.closed) == 1 {
		return nil, fmt.Errorf("connection pool is closed")
	}

	atomic.AddInt64(&cp.stats.RequestsTotal, 1)

	select {
	case conn := <-cp.connections:
		// Test connection if configured
		if cp.config.TestOnBorrow && !conn.IsAlive() {
			conn.conn.Close()
			cp.removeConnection(conn)
			return cp.Get() // Retry
		}

		atomic.StoreInt32(&conn.inUse, 1)
		conn.MarkUsed()
		atomic.AddInt64(&cp.stats.ActiveConnections, 1)
		atomic.AddInt64(&cp.stats.IdleConnections, -1)
		atomic.AddInt64(&cp.stats.RequestsSuccessful, 1)
		return conn, nil

	default:
		// No idle connections available, try to create new one
		if cp.getActiveConnections() < int64(cp.config.MaxConnections) {
			if conn, err := cp.createConnection(); err == nil {
				atomic.StoreInt32(&conn.inUse, 1)
				conn.MarkUsed()
				atomic.AddInt64(&cp.stats.ActiveConnections, 1)
				atomic.AddInt64(&cp.stats.RequestsSuccessful, 1)
				return conn, nil
			}
		}

		// Wait for a connection to become available
		select {
		case conn := <-cp.connections:
			if cp.config.TestOnBorrow && !conn.IsAlive() {
				conn.conn.Close()
				cp.removeConnection(conn)
				return cp.Get() // Retry
			}

			atomic.StoreInt32(&conn.inUse, 1)
			conn.MarkUsed()
			atomic.AddInt64(&cp.stats.ActiveConnections, 1)
			atomic.AddInt64(&cp.stats.IdleConnections, -1)
			atomic.AddInt64(&cp.stats.RequestsSuccessful, 1)
			return conn, nil

		case <-time.After(cp.config.ConnectionTimeout):
			atomic.AddInt64(&cp.stats.RequestsFailed, 1)
			return nil, fmt.Errorf("connection timeout")
		}
	}
}

// Put returns a connection to the pool
func (cp *ConnectionPool) Put(conn *PooledConnection) {
	if atomic.LoadInt32(&cp.closed) == 1 {
		conn.conn.Close()
		return
	}

	// Test connection if configured
	if cp.config.TestOnReturn && !conn.IsAlive() {
		conn.conn.Close()
		cp.removeConnection(conn)
		return
	}

	atomic.StoreInt32(&conn.inUse, 0)
	conn.MarkUsed()
	atomic.AddInt64(&cp.stats.ActiveConnections, -1)
	atomic.AddInt64(&cp.stats.IdleConnections, 1)

	select {
	case cp.connections <- conn:
		// Successfully returned to pool
	default:
		// Pool is full, close the connection
		conn.conn.Close()
		cp.removeConnection(conn)
	}
}

func (cp *ConnectionPool) removeConnection(conn *PooledConnection) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if cp.allConns[conn] {
		delete(cp.allConns, conn)
		atomic.AddInt64(&cp.stats.TotalConnections, -1)
		if atomic.LoadInt32(&conn.inUse) == 0 {
			atomic.AddInt64(&cp.stats.IdleConnections, -1)
		} else {
			atomic.AddInt64(&cp.stats.ActiveConnections, -1)
		}
	}
}

func (cp *ConnectionPool) cleanupIdleConnections() {
	ticker := time.NewTicker(cp.config.IdleCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cp.ctx.Done():
			return
		case <-ticker.C:
			cp.cleanupIdle()
		}
	}
}

func (cp *ConnectionPool) cleanupIdle() {
	now := time.Now()
	var toRemove []*PooledConnection

	// Check all idle connections
	for {
		select {
		case conn := <-cp.connections:
			if now.Sub(conn.LastUsed()) > cp.config.MaxIdleTime || !conn.IsAlive() {
				toRemove = append(toRemove, conn)
			} else {
				// Put back healthy connection
				cp.connections <- conn
				return // Stop checking to avoid infinite loop
			}
		default:
			// No more connections to check
			goto cleanup
		}
	}

cleanup:
	// Close and remove idle connections
	for _, conn := range toRemove {
		conn.conn.Close()
		cp.removeConnection(conn)
	}

	// Ensure minimum connections
	currentIdle := len(cp.connections)
	for currentIdle < cp.config.MinConnections {
		if conn, err := cp.createConnection(); err == nil {
			cp.connections <- conn
			currentIdle++
		} else {
			break
		}
	}
}

// GetStats returns current pool statistics
func (cp *ConnectionPool) GetStats() PoolStats {
	return PoolStats{
		TotalConnections:   atomic.LoadInt64(&cp.stats.TotalConnections),
		ActiveConnections:  atomic.LoadInt64(&cp.stats.ActiveConnections),
		IdleConnections:    atomic.LoadInt64(&cp.stats.IdleConnections),
		RequestsTotal:      atomic.LoadInt64(&cp.stats.RequestsTotal),
		RequestsSuccessful: atomic.LoadInt64(&cp.stats.RequestsSuccessful),
		RequestsFailed:     atomic.LoadInt64(&cp.stats.RequestsFailed),
	}
}

func (cp *ConnectionPool) getActiveConnections() int64 {
	return atomic.LoadInt64(&cp.stats.TotalConnections)
}

// Close closes the connection pool and all connections
func (cp *ConnectionPool) Close() error {
	if !atomic.CompareAndSwapInt32(&cp.closed, 0, 1) {
		return nil // Already closed
	}

	cp.cancel()

	// Close all connections
	close(cp.connections)
	for conn := range cp.connections {
		conn.conn.Close()
	}

	cp.mu.Lock()
	for conn := range cp.allConns {
		conn.conn.Close()
	}
	cp.allConns = make(map[*PooledConnection]bool)
	cp.mu.Unlock()

	return nil
}

// PoolManager manages multiple connection pools
type PoolManager struct {
	mu    sync.RWMutex
	pools map[string]*ConnectionPool
}

func NewPoolManager() *PoolManager {
	return &PoolManager{
		pools: make(map[string]*ConnectionPool),
	}
}

func (pm *PoolManager) GetPool(address string, factory ConnectionFactory, config *PoolConfig) *ConnectionPool {
	pm.mu.RLock()
	pool, exists := pm.pools[address]
	pm.mu.RUnlock()

	if exists {
		return pool
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Double-check after acquiring write lock
	if pool, exists := pm.pools[address]; exists {
		return pool
	}

	pool = NewConnectionPool(address, factory, config)
	pm.pools[address] = pool
	return pool
}

func (pm *PoolManager) ClosePool(address string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pool, exists := pm.pools[address]; exists {
		delete(pm.pools, address)
		return pool.Close()
	}

	return nil
}

func (pm *PoolManager) CloseAll() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var lastError error
	for address, pool := range pm.pools {
		if err := pool.Close(); err != nil {
			lastError = err
		}
		delete(pm.pools, address)
	}

	return lastError
}

// LoadBalancer interface for different load balancing strategies
type LoadBalancer interface {
	SelectPool(pools []*ConnectionPool) *ConnectionPool
	UpdatePoolMetrics(pool *ConnectionPool, responseTime time.Duration, success bool)
}

// RoundRobinLoadBalancer implements round-robin load balancing
type RoundRobinLoadBalancer struct {
	counter int64
}

func (rr *RoundRobinLoadBalancer) SelectPool(pools []*ConnectionPool) *ConnectionPool {
	if len(pools) == 0 {
		return nil
	}

	index := atomic.AddInt64(&rr.counter, 1) % int64(len(pools))
	return pools[index]
}

func (rr *RoundRobinLoadBalancer) UpdatePoolMetrics(pool *ConnectionPool, responseTime time.Duration, success bool) {
	// Round-robin doesn't use metrics
}

// WeightedLeastConnectionsLoadBalancer selects pools based on active connections and performance
type WeightedLeastConnectionsLoadBalancer struct {
	poolMetrics map[*ConnectionPool]*PoolMetrics
	mutex       sync.RWMutex
}

type PoolMetrics struct {
	ActiveConnections int64
	AverageResponse   time.Duration
	SuccessRate       float64
	Weight            float64
	mutex             sync.RWMutex
}

func NewWeightedLeastConnectionsLoadBalancer() *WeightedLeastConnectionsLoadBalancer {
	return &WeightedLeastConnectionsLoadBalancer{
		poolMetrics: make(map[*ConnectionPool]*PoolMetrics),
	}
}

func (wlc *WeightedLeastConnectionsLoadBalancer) SelectPool(pools []*ConnectionPool) *ConnectionPool {
	if len(pools) == 0 {
		return nil
	}

	wlc.mutex.RLock()
	defer wlc.mutex.RUnlock()

	var bestPool *ConnectionPool
	bestScore := float64(-1)

	for _, pool := range pools {
		metrics, exists := wlc.poolMetrics[pool]
		if !exists {
			// New pool, give it a chance
			return pool
		}

		metrics.mutex.RLock()
		// Calculate score based on connections, response time, and success rate
		score := metrics.SuccessRate * metrics.Weight / (float64(metrics.ActiveConnections+1) * float64(metrics.AverageResponse.Milliseconds()+1))
		metrics.mutex.RUnlock()

		if score > bestScore {
			bestScore = score
			bestPool = pool
		}
	}

	return bestPool
}

func (wlc *WeightedLeastConnectionsLoadBalancer) UpdatePoolMetrics(pool *ConnectionPool, responseTime time.Duration, success bool) {
	wlc.mutex.Lock()
	metrics, exists := wlc.poolMetrics[pool]
	if !exists {
		metrics = &PoolMetrics{
			Weight: 1.0,
		}
		wlc.poolMetrics[pool] = metrics
	}
	wlc.mutex.Unlock()

	metrics.mutex.Lock()
	defer metrics.mutex.Unlock()

	// Update active connections
	metrics.ActiveConnections = int64(pool.ActiveCount())

	// Update average response time (exponential moving average)
	if metrics.AverageResponse == 0 {
		metrics.AverageResponse = responseTime
	} else {
		metrics.AverageResponse = time.Duration(0.9*float64(metrics.AverageResponse) + 0.1*float64(responseTime))
	}

	// Update success rate (exponential moving average)
	successValue := 0.0
	if success {
		successValue = 1.0
	}

	if metrics.SuccessRate == 0 {
		metrics.SuccessRate = successValue
	} else {
		metrics.SuccessRate = 0.9*metrics.SuccessRate + 0.1*successValue
	}
}

// AdvancedPoolManager extends PoolManager with load balancing and health checking
type AdvancedPoolManager struct {
	*PoolManager
	loadBalancer    LoadBalancer
	healthChecker   *HealthChecker
	pools          map[string][]*ConnectionPool // address -> pools
	poolsMutex     sync.RWMutex
}

func NewAdvancedPoolManager(loadBalancer LoadBalancer) *AdvancedPoolManager {
	if loadBalancer == nil {
		loadBalancer = &RoundRobinLoadBalancer{}
	}

	return &AdvancedPoolManager{
		PoolManager:   NewPoolManager(),
		loadBalancer:  loadBalancer,
		healthChecker: NewHealthChecker(30*time.Second), // Health check every 30 seconds
		pools:         make(map[string][]*ConnectionPool),
	}
}

func (apm *AdvancedPoolManager) GetPoolWithLoadBalancing(address string) *ConnectionPool {
	apm.poolsMutex.RLock()
	pools, exists := apm.pools[address]
	apm.poolsMutex.RUnlock()

	if !exists || len(pools) == 0 {
		return nil
	}

	// Filter healthy pools
	healthyPools := make([]*ConnectionPool, 0, len(pools))
	for _, pool := range pools {
		if apm.healthChecker.IsHealthy(address) {
			healthyPools = append(healthyPools, pool)
		}
	}

	if len(healthyPools) == 0 {
		// No healthy pools, return the first one anyway
		return pools[0]
	}

	return apm.loadBalancer.SelectPool(healthyPools)
}

func (apm *AdvancedPoolManager) AddPool(address string, pool *ConnectionPool) {
	apm.poolsMutex.Lock()
	defer apm.poolsMutex.Unlock()

	apm.pools[address] = append(apm.pools[address], pool)
	apm.healthChecker.AddEndpoint(address)
}

// Health checker for monitoring pool health
type HealthChecker struct {
	endpoints    map[string]*EndpointHealth
	checkInterval time.Duration
	mutex        sync.RWMutex
	stop         chan struct{}
	running      int32
}

type EndpointHealth struct {
	Address         string
	IsHealthy       bool
	LastCheck       time.Time
	SuccessiveFailures int
	mutex           sync.RWMutex
}

func NewHealthChecker(checkInterval time.Duration) *HealthChecker {
	return &HealthChecker{
		endpoints:     make(map[string]*EndpointHealth),
		checkInterval: checkInterval,
		stop:          make(chan struct{}),
	}
}

func (hc *HealthChecker) AddEndpoint(address string) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	if _, exists := hc.endpoints[address]; !exists {
		hc.endpoints[address] = &EndpointHealth{
			Address:   address,
			IsHealthy: true,
			LastCheck: time.Now(),
		}
	}
}

func (hc *HealthChecker) IsHealthy(address string) bool {
	hc.mutex.RLock()
	endpoint, exists := hc.endpoints[address]
	hc.mutex.RUnlock()

	if !exists {
		return true // Assume healthy if not monitored
	}

	endpoint.mutex.RLock()
	defer endpoint.mutex.RUnlock()
	return endpoint.IsHealthy
}

func (hc *HealthChecker) Start(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&hc.running, 0, 1) {
		return
	}

	go hc.healthCheckLoop(ctx)
}

func (hc *HealthChecker) Stop() {
	if atomic.CompareAndSwapInt32(&hc.running, 1, 0) {
		close(hc.stop)
	}
}

func (hc *HealthChecker) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(hc.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-hc.stop:
			return
		case <-ticker.C:
			hc.performHealthChecks()
		}
	}
}

func (hc *HealthChecker) performHealthChecks() {
	hc.mutex.RLock()
	endpoints := make([]*EndpointHealth, 0, len(hc.endpoints))
	for _, endpoint := range hc.endpoints {
		endpoints = append(endpoints, endpoint)
	}
	hc.mutex.RUnlock()

	var wg sync.WaitGroup
	for _, endpoint := range endpoints {
		wg.Add(1)
		go func(ep *EndpointHealth) {
			defer wg.Done()
			hc.checkEndpoint(ep)
		}(endpoint)
	}
	wg.Wait()
}

func (hc *HealthChecker) checkEndpoint(endpoint *EndpointHealth) {
	// Simple TCP connection test
	conn, err := net.DialTimeout("tcp", endpoint.Address, 5*time.Second)

	endpoint.mutex.Lock()
	defer endpoint.mutex.Unlock()

	endpoint.LastCheck = time.Now()

	if err != nil {
		endpoint.SuccessiveFailures++
		if endpoint.SuccessiveFailures >= 3 {
			endpoint.IsHealthy = false
		}
	} else {
		conn.Close()
		endpoint.SuccessiveFailures = 0
		endpoint.IsHealthy = true
	}
}
