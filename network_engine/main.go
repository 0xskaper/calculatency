package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// Data structures for API communication with Java
type MeasurementRequest struct {
	ClientIP    string `json:"clientIP" binding:"required"`
	RequestID   string `json:"requestID,omitempty"`
	Timeout     int    `json:"timeout,omitempty"` // seconds
	ICMPCount   int    `json:"icmpCount,omitempty"`
	TraceMaxTTL int    `json:"traceMaxTTL,omitempty"`
}

type MeasurementResult struct {
	RequestID        string              `json:"requestID"`
	ClientIP         string              `json:"clientIP"`
	Success          bool                `json:"success"`
	WebSocketRTT     int64               `json:"webSocketRTT"`    // milliseconds
	TCPHandshakeRTT  int64               `json:"tcpHandshakeRTT"` // milliseconds
	ICMPMinRTT       int64               `json:"icmpMinRTT"`      // milliseconds
	ZeroTraceRTT     int64               `json:"zeroTraceRTT"`    // milliseconds
	RTTDifference    int64               `json:"rttDifference"`   // milliseconds
	IsProxy          bool                `json:"isProxy"`
	Confidence       float64             `json:"confidence"`
	Label            string              `json:"label"`
	Measurements     NetworkMeasurements `json:"measurements"`
	Error            string              `json:"error,omitempty"`
	ProcessingTimeMs int64               `json:"processingTimeMs"`
}

type NetworkMeasurements struct {
	ICMPResults      []ICMPResult     `json:"icmpResults"`
	ZeroTraceResults []ZeroTraceHop   `json:"zeroTraceResults"`
	TCPHandshake     TCPHandshakeData `json:"tcpHandshake"`
	Timestamp        time.Time        `json:"timestamp"`
}

type ICMPResult struct {
	Sequence int           `json:"sequence"`
	RTT      time.Duration `json:"rtt"`
	Success  bool          `json:"success"`
	Error    string        `json:"error,omitempty"`
}

type ZeroTraceHop struct {
	TTL     int           `json:"ttl"`
	IP      string        `json:"ip"`
	RTT     time.Duration `json:"rtt"`
	Success bool          `json:"success"`
	Error   string        `json:"error,omitempty"`
}

type TCPHandshakeData struct {
	SynTime time.Time     `json:"synTime"`
	AckTime time.Time     `json:"ackTime"`
	RTT     time.Duration `json:"rtt"`
	Success bool          `json:"success"`
}

// Core measurement components
type NetworkEngine struct {
	icmpPinger   *ICMPPinger
	zeroTracer   *ZeroTracer
	tcpMonitor   *TCPMonitor
	decisionTree *DecisionTree
	measurements map[string]*MeasurementResult
	mu           sync.RWMutex
}

// ICMP Pinger implementation
type ICMPPinger struct {
	conn *icmp.PacketConn
	mu   sync.Mutex
}

func NewICMPPinger() (*ICMPPinger, error) {
	// Try privileged first, fallback to unprivileged
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		// Try unprivileged UDP-based ICMP
		conn, err = icmp.ListenPacket("udp4", "0.0.0.0")
		if err != nil {
			return nil, fmt.Errorf("failed to create ICMP connection: %w", err)
		}
	}

	return &ICMPPinger{conn: conn}, nil
}

func (p *ICMPPinger) Ping(target string, count int) ([]ICMPResult, error) {
	dst, err := net.ResolveIPAddr("ip4", target)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", target, err)
	}

	results := make([]ICMPResult, count)

	for i := 0; i < count; i++ {
		result := ICMPResult{Sequence: i + 1}

		// Create ICMP message
		message := &icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:   os.Getpid() & 0xffff,
				Seq:  i + 1,
				Data: []byte(fmt.Sprintf("CalcuLatency-%d", i)),
			},
		}

		data, err := message.Marshal(nil)
		if err != nil {
			result.Error = err.Error()
			results[i] = result
			continue
		}

		// Send ping
		start := time.Now()
		p.mu.Lock()
		_, err = p.conn.WriteTo(data, dst)
		if err != nil {
			p.mu.Unlock()
			result.Error = err.Error()
			results[i] = result
			continue
		}

		// Set read deadline
		p.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		reply := make([]byte, 1500)
		_, _, err = p.conn.ReadFrom(reply)
		p.mu.Unlock()

		if err != nil {
			result.Error = err.Error()
			results[i] = result
			continue
		}

		result.RTT = time.Since(start)
		result.Success = true
		results[i] = result

		// Small delay between pings
		if i < count-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	return results, nil
}

func (p *ICMPPinger) Close() error {
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// Zero Trace implementation
type ZeroTracer struct {
	// Will implement raw socket based traceroute
}

func NewZeroTracer() *ZeroTracer {
	return &ZeroTracer{}
}

func (zt *ZeroTracer) Trace(target string, maxTTL int) ([]ZeroTraceHop, error) {
	// Enhanced implementation with multiple fallback strategies
	hops := make([]ZeroTraceHop, 0)

	// Strategy 1: Try HTTPS (port 443) first - more likely to work
	hop := ZeroTraceHop{TTL: 1}
	start := time.Now()

	conn, err := net.DialTimeout("tcp", target+":443", 3*time.Second)
	if err != nil {
		// Strategy 2: Try HTTP (port 80)
		conn, err = net.DialTimeout("tcp", target+":80", 3*time.Second)
		if err != nil {
			// Strategy 3: Try DNS (port 53)
			conn, err = net.DialTimeout("tcp", target+":53", 3*time.Second)
			if err != nil {
				// Strategy 4: Use ICMP-style measurement via system ping
				return zt.fallbackToPing(target)
			}
		}
	}

	if conn != nil {
		hop.RTT = time.Since(start)
		hop.Success = true
		hop.IP = conn.RemoteAddr().String()
		conn.Close()
	}

	hops = append(hops, hop)
	return hops, nil
}

// Fallback to system ping for basic connectivity measurement
func (zt *ZeroTracer) fallbackToPing(target string) ([]ZeroTraceHop, error) {
	hop := ZeroTraceHop{TTL: 1}

	// Simple ICMP ping using system command as last resort
	start := time.Now()
	cmd := exec.Command("ping", "-c", "1", "-W", "3", target)
	err := cmd.Run()

	if err != nil {
		hop.Error = fmt.Sprintf("All connection methods failed to %s", target)
		hop.Success = false
	} else {
		hop.RTT = time.Since(start)
		hop.Success = true
		hop.IP = target // We reached the target via ping
	}

	return []ZeroTraceHop{hop}, nil
}

// TCP Monitor implementation
type TCPMonitor struct {
	connections map[string]TCPHandshakeData
	mu          sync.RWMutex
}

func NewTCPMonitor() *TCPMonitor {
	return &TCPMonitor{
		connections: make(map[string]TCPHandshakeData),
	}
}

func (tm *TCPMonitor) RecordHandshake(clientIP string, synTime, ackTime time.Time) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.connections[clientIP] = TCPHandshakeData{
		SynTime: synTime,
		AckTime: ackTime,
		RTT:     ackTime.Sub(synTime),
		Success: true,
	}
}

func (tm *TCPMonitor) GetHandshakeRTT(clientIP string) (TCPHandshakeData, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	data, exists := tm.connections[clientIP]
	return data, exists
}

// Decision Tree implementation following the paper's logic
type DecisionTree struct{}

func NewDecisionTree() *DecisionTree {
	return &DecisionTree{}
}

func (dt *DecisionTree) Analyze(measurements NetworkMeasurements, webSocketRTT time.Duration) (bool, float64, string, int64) {
	const threshold = 50 * time.Millisecond

	// Get minimum RTTs
	var icmpMinRTT time.Duration = time.Hour // Initialize to large value
	for _, result := range measurements.ICMPResults {
		if result.Success && result.RTT < icmpMinRTT {
			icmpMinRTT = result.RTT
		}
	}

	var zeroTraceRTT time.Duration
	for _, hop := range measurements.ZeroTraceResults {
		if hop.Success {
			zeroTraceRTT = hop.RTT // Use last successful hop
		}
	}

	tcpRTT := measurements.TCPHandshake.RTT

	// Decision tree logic from the paper
	var networkLayerRTT time.Duration
	var label string

	// Check if TCP RTT ≈ WebSocket RTT (within 10ms)
	if abs(tcpRTT.Nanoseconds()-webSocketRTT.Nanoseconds()) < 10*time.Millisecond.Nanoseconds() {
		// Network layer proxy or direct connection
		if icmpMinRTT < time.Hour {
			networkLayerRTT = icmpMinRTT
			label = "WsICMP"
		} else {
			networkLayerRTT = zeroTraceRTT
			label = "Ws0TClient"
		}
	} else {
		// Possible application layer proxy
		if icmpMinRTT < time.Hour {
			networkLayerRTT = icmpMinRTT
			label = "WsICMPApp"
		} else {
			networkLayerRTT = zeroTraceRTT
			label = "Ws0TApp"
		}
	}

	// Calculate RTT difference
	rttDiff := webSocketRTT - networkLayerRTT
	rttDiffMs := rttDiff.Nanoseconds() / 1_000_000

	// Determine if proxy
	isProxy := rttDiff >= threshold

	// Calculate confidence
	confidence := dt.calculateConfidence(rttDiff, threshold)

	return isProxy, confidence, label, rttDiffMs
}

func (dt *DecisionTree) calculateConfidence(rttDiff, threshold time.Duration) float64 {
	// Simple confidence calculation
	// The further from threshold, the higher confidence
	diff := float64(abs(rttDiff.Nanoseconds() - threshold.Nanoseconds()))
	maxDiff := float64(threshold.Nanoseconds())

	confidence := diff / maxDiff
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.1 {
		confidence = 0.1
	}

	return confidence
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// Main Network Engine
func NewNetworkEngine() (*NetworkEngine, error) {
	icmpPinger, err := NewICMPPinger()
	if err != nil {
		return nil, fmt.Errorf("failed to create ICMP pinger: %w", err)
	}

	return &NetworkEngine{
		icmpPinger:   icmpPinger,
		zeroTracer:   NewZeroTracer(),
		tcpMonitor:   NewTCPMonitor(),
		decisionTree: NewDecisionTree(),
		measurements: make(map[string]*MeasurementResult),
	}, nil
}

func (ne *NetworkEngine) PerformMeasurement(req MeasurementRequest) *MeasurementResult {
	startTime := time.Now()

	result := &MeasurementResult{
		RequestID: req.RequestID,
		ClientIP:  req.ClientIP,
		Measurements: NetworkMeasurements{
			Timestamp: startTime,
		},
	}

	// Set defaults
	if req.ICMPCount == 0 {
		req.ICMPCount = 5
	}
	if req.TraceMaxTTL == 0 {
		req.TraceMaxTTL = 32
	}

	// Perform measurements in parallel
	var wg sync.WaitGroup

	// ICMP Ping
	wg.Add(1)
	go func() {
		defer wg.Done()
		icmpResults, err := ne.icmpPinger.Ping(req.ClientIP, req.ICMPCount)
		if err != nil {
			result.Error = fmt.Sprintf("ICMP error: %v", err)
		} else {
			result.Measurements.ICMPResults = icmpResults

			// Find minimum RTT
			var minRTT time.Duration = time.Hour
			for _, r := range icmpResults {
				if r.Success && r.RTT < minRTT {
					minRTT = r.RTT
				}
			}
			if minRTT < time.Hour {
				result.ICMPMinRTT = minRTT.Nanoseconds() / 1_000_000
			}
		}
	}()

	// Zero Trace
	wg.Add(1)
	go func() {
		defer wg.Done()
		traceResults, err := ne.zeroTracer.Trace(req.ClientIP, req.TraceMaxTTL)
		if err != nil {
			if result.Error == "" {
				result.Error = fmt.Sprintf("ZeroTrace error: %v", err)
			}
		} else {
			result.Measurements.ZeroTraceResults = traceResults

			// Get last successful hop RTT
			for i := len(traceResults) - 1; i >= 0; i-- {
				if traceResults[i].Success {
					result.ZeroTraceRTT = traceResults[i].RTT.Nanoseconds() / 1_000_000
					break
				}
			}
		}
	}()

	// Wait for all measurements
	wg.Wait()

	// Get TCP handshake data if available
	if tcpData, exists := ne.tcpMonitor.GetHandshakeRTT(req.ClientIP); exists {
		result.Measurements.TCPHandshake = tcpData
		result.TCPHandshakeRTT = tcpData.RTT.Nanoseconds() / 1_000_000
	}

	result.ProcessingTimeMs = time.Since(startTime).Nanoseconds() / 1_000_000
	result.Success = len(result.Measurements.ICMPResults) > 0 || len(result.Measurements.ZeroTraceResults) > 0

	// Store result
	ne.mu.Lock()
	ne.measurements[req.ClientIP] = result
	ne.mu.Unlock()

	return result
}

func (ne *NetworkEngine) AnalyzeWithWebSocket(clientIP string, webSocketRTT time.Duration) *MeasurementResult {
	ne.mu.RLock()
	result, exists := ne.measurements[clientIP]
	ne.mu.RUnlock()

	if !exists {
		return &MeasurementResult{
			ClientIP: clientIP,
			Error:    "No measurements found for client",
		}
	}

	// Perform decision tree analysis
	isProxy, confidence, label, rttDiff := ne.decisionTree.Analyze(result.Measurements, webSocketRTT)

	result.WebSocketRTT = webSocketRTT.Nanoseconds() / 1_000_000
	result.IsProxy = isProxy
	result.Confidence = confidence
	result.Label = label
	result.RTTDifference = rttDiff

	return result
}

func (ne *NetworkEngine) Close() error {
	if ne.icmpPinger != nil {
		return ne.icmpPinger.Close()
	}
	return nil
}

// HTTP API Handlers for Java integration
func (ne *NetworkEngine) measureHandler(c *gin.Context) {
	var req MeasurementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	result := ne.PerformMeasurement(req)
	c.JSON(200, result)
}

func (ne *NetworkEngine) analyzeHandler(c *gin.Context) {
	type AnalyzeRequest struct {
		ClientIP     string `json:"clientIP" binding:"required"`
		WebSocketRTT int64  `json:"webSocketRTT" binding:"required"` // milliseconds
	}

	var req AnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	webSocketRTT := time.Duration(req.WebSocketRTT) * time.Millisecond
	result := ne.AnalyzeWithWebSocket(req.ClientIP, webSocketRTT)

	c.JSON(200, result)
}

func (ne *NetworkEngine) healthHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":    "healthy",
		"service":   "CalcuLatency Go Network Engine",
		"timestamp": time.Now(),
		"version":   "1.0.0",
	})
}

func (ne *NetworkEngine) statusHandler(c *gin.Context) {
	ne.mu.RLock()
	measurementCount := len(ne.measurements)
	ne.mu.RUnlock()

	c.JSON(200, gin.H{
		"measurement_count": measurementCount,
		"uptime":            time.Since(time.Now()), // Would track actual uptime
		"components": gin.H{
			"icmp_pinger": ne.icmpPinger != nil,
			"zero_tracer": ne.zeroTracer != nil,
			"tcp_monitor": ne.tcpMonitor != nil,
		},
	})
}

func main() {
	// Check if running with required privileges
	if os.Geteuid() != 0 {
		log.Println("WARNING: Running without root privileges. ICMP may not work properly.")
		log.Println("Consider running with: sudo ./calculatency or setcap cap_net_raw+ep ./calculatency")
	}

	// Initialize network engine
	engine, err := NewNetworkEngine()
	if err != nil {
		log.Fatalf("Failed to initialize network engine: %v", err)
	}
	defer engine.Close()

	// Setup HTTP server
	router := gin.Default()

	// CORS middleware for Java integration
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// API endpoints
	v1 := router.Group("/api/v1")
	{
		v1.GET("/health", engine.healthHandler)
		v1.GET("/status", engine.statusHandler)
		v1.POST("/measure", engine.measureHandler)
		v1.POST("/analyze", engine.analyzeHandler)
	}

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("CalcuLatency Go Network Engine starting on port %s", port)
	log.Printf("API endpoints available at:")
	log.Printf("  POST /api/v1/measure - Perform network measurements")
	log.Printf("  POST /api/v1/analyze - Analyze with WebSocket RTT")
	log.Printf("  GET  /api/v1/health  - Health check")
	log.Printf("  GET  /api/v1/status  - Service status")

	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
