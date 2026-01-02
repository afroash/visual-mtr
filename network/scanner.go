package network

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"os"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// Scanner manages the network path scanning operations
type Scanner struct {
	hostname   string
	hops       []NetworkHop
	updates    chan HopUpdate
	ctx        context.Context
	cancel     context.CancelFunc
	conn       *icmp.PacketConn // Connection for ICMP operations
	stopCalled bool             // Flag to prevent double-close of channel
}

// NewScanner creates a new scanner instance
func NewScanner(hostname string) *Scanner {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scanner{
		hostname: hostname,
		hops:     make([]NetworkHop, 0),
		updates:  make(chan HopUpdate, 100),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins the scanning process
// This function should:
// 1. Perform traceroute to identify all hops
// 2. Start continuous pinging of all hops in parallel
// 3. Send updates via the Updates() channel
func (s *Scanner) Start() error {
	// Perform traceroute to discover all hops, sending them in real-time via updates channel
	hops, err := performTraceroute(s.hostname, s.updates, s.ctx)
	if err != nil {
		return err
	}

	// Store the discovered hops
	s.hops = hops

	// Create ICMP connection for continuous monitoring
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return fmt.Errorf("failed to create ICMP connection for monitoring: %v", err)
	}
	s.conn = conn

	log.Printf("[DEBUG] Traceroute discovered %d hops, starting PING loop\n", len(s.hops))

	// Start the monitoring loop if we have hops
	if len(s.hops) > 0 {
		log.Printf("[DEBUG] Starting PING loop for continuous ping updates\n")
		go s.monitorLoop()
	}

	return nil
}

// extractIPFromAddr extracts the IP address from a net.Addr
// Handles both "ip:port" format and plain IP addresses
func extractIPFromAddr(addr net.Addr) string {
	addrStr := addr.String()
	host, _, err := net.SplitHostPort(addrStr)
	if err != nil {
		// If SplitHostPort fails, assume it's just an IP address
		return addrStr
	}
	return host
}

// Stop halts the scanning process
func (s *Scanner) Stop() {
	s.cancel()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	// Safely close the updates channel (only once)
	if !s.stopCalled {
		s.stopCalled = true
		close(s.updates)
	}
}

// Updates returns the channel that emits hop updates
func (s *Scanner) Updates() <-chan HopUpdate {
	return s.updates
}

// GetHops returns the current list of hops
func (s *Scanner) GetHops() []NetworkHop {
	return s.hops
}

// monitorLoop continuously pings all hops and sends updates
// This runs in a background goroutine
func (s *Scanner) monitorLoop() {
	ticker := time.NewTicker(1 * time.Second) // Update every second
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.pingAllHops()
		}
	}
}

// pingAllHops pings all hops in parallel and sends updates
func (s *Scanner) pingAllHops() {
	// Check if the hops are empty
	if len(s.hops) == 0 {
		return
	}

	for i, hop := range s.hops {
		latency, loss := s.pingHop(hop.IP)

		// Build updated latency history (rolling window of last MaxLatencyHistory samples)
		newHistory := make([]float64, 0, MaxLatencyHistory)
		if len(hop.LatencyHistory) > 0 {
			// Copy existing history
			if len(hop.LatencyHistory) >= MaxLatencyHistory {
				// Take last (MaxLatencyHistory-1) elements to make room for new one
				newHistory = append(newHistory, hop.LatencyHistory[1:]...)
			} else {
				newHistory = append(newHistory, hop.LatencyHistory...)
			}
		}
		// Append new latency (use -1 to indicate timeout/no response)
		if latency > 0 {
			newHistory = append(newHistory, latency)
		} else {
			newHistory = append(newHistory, -1) // -1 indicates timeout
		}

		// Calculate average from valid latencies in history
		avgLatency := calculateAverageLatency(newHistory)

		updatedHop := NetworkHop{
			IP:             hop.IP,
			AvgLatency:     avgLatency,
			LossPercent:    loss,
			LatencyHistory: newHistory,
		}

		// Update local hop data
		s.hops[i] = updatedHop

		s.updates <- HopUpdate{Index: i, Hop: updatedHop}
	}
}

// calculateAverageLatency calculates average from valid latency values (ignoring -1 timeouts)
func calculateAverageLatency(history []float64) float64 {
	if len(history) == 0 {
		return 0
	}
	var sum float64
	var count int
	for _, lat := range history {
		if lat > 0 {
			sum += lat
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func (s *Scanner) pingHop(ip string) (float64, float64) {
	// TODO: Implement actual ICMP ping logic here
	startTime := time.Now()

	log.Printf("[DEBUG] Sending PING packet to %s\n", ip)

	// Create ICMP Message. Type will be Echo Request
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   os.Getpid() % 0xFFFF,
			Seq:  1,
			Data: []byte("HELLO-PING"),
		},
	}

	// Marshal the message
	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		log.Fatalf("Failed to marshal message: %v", err)
	}

	// Send the message
	if _, err := s.conn.WriteTo(msgBytes, &net.IPAddr{IP: net.ParseIP(ip)}); err != nil {
		log.Fatalf("Failed to send message: %v", err)
	}
	log.Printf("[DEBUG] Sent PING packet to %s (ID=%d, Seq=%d)\n", ip, msg.Body.(*icmp.Echo).ID, msg.Body.(*icmp.Echo).Seq)

	// Receive the response
	buf := make([]byte, 1500) // MTU size
	s.conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	n, peerAddr, err := s.conn.ReadFrom(buf)
	if err != nil {
		log.Printf("[DEBUG] PING to %s: Timeout (no response within 3 seconds)\n", ip)
		return 0, 0
	}

	elapsed := time.Since(startTime)
	log.Printf("[DEBUG] Received response from %s (%.2fms)\n", peerAddr.String(), elapsed.Seconds()*1000)

	// Unmarshal the response
	recvMsg, err := icmp.ParseMessage(1, buf[:n]) // 1 for ICMPv4
	if err != nil {
		log.Printf("[DEBUG] PING to %s: Failed to parse response: %v\n", ip, err)
		return 0, 0
	}
	log.Printf("[DEBUG] PING to %s: Parsed ICMP message type: %v\n", ip, recvMsg.Type)

	// Handle the response and return the latency and loss percentage
	switch recvMsg.Type {
	case ipv4.ICMPTypeEchoReply:
		return elapsed.Seconds() * 1000, 0
	case ipv4.ICMPTypeTimeExceeded:
		return 0, 100
	case ipv4.ICMPTypeDestinationUnreachable:
		return 0, 100
	default:
		return 0, 0
	}
}

// performTraceroute performs a traceroute to the target hostname
// Sends hops to the updates channel as they're discovered (for real-time UI updates)
// Returns a slice of NetworkHop with IP addresses populated
func performTraceroute(hostname string, updates chan<- HopUpdate, ctx context.Context) ([]NetworkHop, error) {
	// Resolve the hostname to an IP address
	dstAddr, err := net.ResolveIPAddr("ip", hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve hostname: %v", err)
	}

	// Create a new ICMP connection
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {

		return nil, fmt.Errorf("failed to create ICMP connection: %v", err)
	}
	defer conn.Close()

	fmt.Printf("Starting traceroute to: %s on IP: %s\n", hostname, dstAddr.IP.String())

	// Local slice to collect hops
	hops := make([]NetworkHop, 0)

	// Perform traceroute
	destinationReached := false
	for ttl := 1; ttl <= 30; ttl++ {
		startTime := time.Now()

		log.Printf("[DEBUG] TTL=%d: Sending probe to %s\n", ttl, dstAddr.IP.String())

		// Set TTL
		if err := conn.IPv4PacketConn().SetTTL(ttl); err != nil {
			log.Fatalf("Failed to set TTL: %v", err)
		}

		// Create ICMP Message. Type will be Echo Request
		msg := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:   os.Getpid() % 0xFFFF,
				Seq:  ttl,
				Data: []byte("HELLO-TRACEROUTE"),
			},
		}

		// Marshal the message
		msgBytes, err := msg.Marshal(nil)
		if err != nil {
			log.Fatalf("Failed to marshal message: %v", err)
		}

		// Send the message
		if _, err := conn.WriteTo(msgBytes, dstAddr); err != nil {
			log.Fatalf("Failed to send message: %v", err)
		}
		log.Printf("[DEBUG] TTL=%d: Sent ICMP Echo Request (ID=%d, Seq=%d)\n", ttl, msg.Body.(*icmp.Echo).ID, msg.Body.(*icmp.Echo).Seq)

		// Receive the response
		buf := make([]byte, 1500) // MTU size
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))

		n, peerAddr, err := conn.ReadFrom(buf)
		if err != nil {
			fmt.Printf("%d\t*\t*\t*\n", ttl) // Timeout
			log.Printf("[DEBUG] TTL=%d: Timeout (no response within 3 seconds)\n", ttl)
			continue
		}

		elapsed := time.Since(startTime)
		log.Printf("[DEBUG] TTL=%d: Received response from %s (%.2fms)\n", ttl, peerAddr.String(), elapsed.Seconds()*1000)

		// Unmarshal the response
		recvMsg, err := icmp.ParseMessage(1, buf[:n]) // 1 for ICMPv4
		if err != nil {
			log.Printf("[DEBUG] TTL=%d: Failed to parse response: %v\n", ttl, err)
			continue
		}
		log.Printf("[DEBUG] TTL=%d: Parsed ICMP message type: %v\n", ttl, recvMsg.Type)

		// Handle the response and add to hops
		switch recvMsg.Type {
		case ipv4.ICMPTypeEchoReply:
			if recvMsg.Body.(*icmp.Echo).ID != os.Getpid()%0xFFFF {
				log.Printf("[DEBUG] TTL=%d: Received EchoReply with wrong ID, ignoring\n", ttl)
				continue // Not our message
			}
			reply := recvMsg.Body.(*icmp.Echo)
			// Extract IP from peerAddr (format: "ip:port" or just "ip")
			hopIP := extractIPFromAddr(peerAddr)
			fmt.Printf("%d\t%s\t%d\t%.2fms\n", ttl, hopIP, reply.Seq, elapsed.Seconds()*1000)
			hop := NetworkHop{IP: hopIP, AvgLatency: elapsed.Seconds() * 1000, LossPercent: 0}
			hops = append(hops, hop)
			log.Printf("[DEBUG] Added final hop: IP=%s, Latency=%.2fms\n", hopIP, elapsed.Seconds()*1000)

			// Send hop to UI in real-time
			hopIndex := len(hops) - 1
			select {
			case updates <- HopUpdate{Index: hopIndex, Hop: hop}:
				log.Printf("[DEBUG] Sent hop %d to UI: IP=%s\n", hopIndex+1, hopIP)
			case <-ctx.Done():
				return hops, nil
			}

			// Destination reached, traceroute complete
			destinationReached = true

		case ipv4.ICMPTypeTimeExceeded:
			// Extract IP from peerAddr (format: "ip:port" or just "ip")
			hopIP := extractIPFromAddr(peerAddr)
			fmt.Printf("%d\t%s\t%d\t%.2fms\n", ttl, hopIP, ttl, elapsed.Seconds()*1000)
			hop := NetworkHop{IP: hopIP, AvgLatency: elapsed.Seconds() * 1000, LossPercent: 0}
			hops = append(hops, hop)
			log.Printf("[DEBUG] TTL=%d: Received TimeExceeded from %s (%.2fms)\n", ttl, hopIP, elapsed.Seconds()*1000)
			log.Printf("[DEBUG] Added hop: IP=%s, Latency=%.2fms\n", hopIP, elapsed.Seconds()*1000)

			// Send hop to UI in real-time
			hopIndex := len(hops) - 1
			select {
			case updates <- HopUpdate{Index: hopIndex, Hop: hop}:
				log.Printf("[DEBUG] Sent hop %d to UI: IP=%s\n", hopIndex+1, hopIP)
			case <-ctx.Done():
				return hops, nil
			}

			// Continue to next TTL
			continue
		default:
			fmt.Printf("%d\t*\t*\t*\n", ttl) // Unknown type
			log.Printf("[DEBUG] TTL=%d: Unknown ICMP type: %v\n", ttl, recvMsg.Type)
			continue
		}

		// Break out of loop if destination reached
		if destinationReached {
			break
		}
	}

	log.Printf("[DEBUG] Traceroute complete: %d hops discovered\n", len(hops))

	return hops, nil
}
