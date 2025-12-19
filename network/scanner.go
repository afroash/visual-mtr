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
	hostname string
	hops     []NetworkHop
	updates  chan HopUpdate
	ctx      context.Context
	cancel   context.CancelFunc
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
	// TODO: Implement traceroute logic here
	// This should populate s.hops with initial hop data
	dstAddr, err := net.ResolveIPAddr("ip", s.hostname)
	if err != nil {
		return err
	}

	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return err
	}

	defer conn.Close()

	fmt.Printf("Starting traceroute to: %s on IP: %s\n", s.hostname, dstAddr.IP.String())

	for ttl := 1; ttl <= 30; ttl++ {
		startTime := time.Now()

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

		// Receive the response
		buf := make([]byte, 1500) // MTU size
		if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
			log.Fatalf("SetReadDeadline failed: %s", err)
		}

		n, peerAddr, err := conn.ReadFrom(buf)
		if err != nil {
			fmt.Printf("%d\t*\t*\t*\n", ttl) // Timeout
			continue
		}

		elapsed := time.Since(startTime)

		// Unmarshal the response
		recvMsg, err := icmp.ParseMessage(1, buf[:n]) // 1 for ICMPv4
		if err != nil {
			log.Fatalf("Failed to parse response: %v", err)
		}

		// Handle the response and add to hops
		switch recvMsg.Type {
		case ipv4.ICMPTypeEchoReply:
			if recvMsg.Body.(*icmp.Echo).ID != os.Getpid()%0xFFFF {
				continue // Not our message
			}

			reply := recvMsg.Body.(*icmp.Echo)
			fmt.Printf("%d\t%s\t%d\t%.2fms\n", ttl, peerAddr.String(), reply.Seq, elapsed.Seconds()*1000)
			s.hops = append(s.hops, NetworkHop{IP: dstAddr.IP.String(), AvgLatency: elapsed.Seconds() * 1000, LossPercent: 0})
			return nil

		case ipv4.ICMPTypeTimeExceeded:
			// For simplicity, we assume any TimeExceeded message is for our probe.
			// A robust implementation would parse the body of the message
			// to verify the ID of the original packet. TODO: Implement this Later. For now, we assume any TimeExceeded message is for our probe.
			//fmt.Printf("%d\t%s\t%d\t%.2fms\n", ttl, dstAddr.IP.String(), recvMsg.Body.(*icmp.TimeExceeded).Record[0].Seq, elapsed.Seconds()*1000)
			//fmt.Printf("%d\t%s\t%d\t%.2fms\n", ttl, dstAddr.IP.String(), reply.Seq, elapsed.Seconds()*1000)

			s.hops = append(s.hops, NetworkHop{IP: peerAddr.String(), AvgLatency: elapsed.Seconds() * 1000, LossPercent: 0})
			return nil
		default:
			fmt.Printf("%d\t*\t*\t*\n", ttl) // Unknown type
			continue
		}
	}

	return nil // Success
}

// Stop halts the scanning process
func (s *Scanner) Stop() {
	s.cancel()
	close(s.updates)
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
// TODO: Implement actual ICMP ping logic here
func (s *Scanner) pingAllHops() {
	// TODO: For each hop in s.hops:
	// 1. Send ICMP ping packets
	// 2. Calculate average latency
	// 3. Calculate packet loss percentage
	// 4. Send update via s.updates channel

	// Placeholder: This is where you'll implement the actual ping logic
	// Example structure:
	// for i, hop := range s.hops {
	//     latency, loss := pingHop(hop.IP)
	//     updatedHop := NetworkHop{
	//         IP:          hop.IP,
	//         AvgLatency:  latency,
	//         LossPercent: loss,
	//     }
	//     s.updates <- HopUpdate{Index: i, Hop: updatedHop}
	// }
}

// performTraceroute performs a traceroute to the target hostname
// TODO: Implement actual traceroute logic here
// This should return a slice of NetworkHop with IP addresses populated
func performTraceroute(hostname string) ([]NetworkHop, error) {
	// TODO: Implement traceroute using ICMP or UDP packets
	// Return a slice of NetworkHop structs with IP addresses set
	// Latency and LossPercent can be initialized to 0

	// Placeholder return
	return []NetworkHop{}, nil
}
