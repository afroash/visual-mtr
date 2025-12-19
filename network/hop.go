package network

// NetworkHop represents a single hop in the network path
type NetworkHop struct {
	IP          string  // IP address of the hop
	AvgLatency  float64 // Average latency in milliseconds
	LossPercent float64 // Packet loss percentage (0-100)
}

// HopUpdate is used to send hop updates from the scanner to the UI
type HopUpdate struct {
	Index int        // Index of the hop (0-based)
	Hop   NetworkHop // Updated hop data
}
