package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// Latency threshold colors
var (
	ColorGood    = color.NRGBA{R: 34, G: 197, B: 94, A: 255}   // Green - < 50ms
	ColorMedium  = color.NRGBA{R: 251, G: 191, B: 36, A: 255}  // Amber - 50-150ms
	ColorHigh    = color.NRGBA{R: 239, G: 68, B: 68, A: 255}   // Red - > 150ms
	ColorTimeout = color.NRGBA{R: 156, G: 163, B: 175, A: 255} // Gray - timeout
	ColorBg      = color.NRGBA{R: 30, G: 30, B: 40, A: 255}    // Dark background
	ColorGrid    = color.NRGBA{R: 55, G: 55, B: 70, A: 255}    // Grid lines
)

// Latency thresholds in milliseconds
const (
	ThresholdGood   = 50.0
	ThresholdMedium = 150.0
)

// LatencyGraph is a custom widget that displays a mini line graph of latency history
type LatencyGraph struct {
	widget.BaseWidget
	data      []float64 // Latency history data
	maxPoints int       // Maximum number of points to display
	minSize   fyne.Size // Minimum size of the graph
}

// NewLatencyGraph creates a new latency graph widget
func NewLatencyGraph() *LatencyGraph {
	g := &LatencyGraph{
		data:      make([]float64, 0),
		maxPoints: 60,
		minSize:   fyne.NewSize(200, 40),
	}
	g.ExtendBaseWidget(g)
	return g
}

// SetData updates the latency data displayed in the graph
func (g *LatencyGraph) SetData(data []float64) {
	g.data = data
	g.Refresh()
}

// MinSize returns the minimum size of the widget
func (g *LatencyGraph) MinSize() fyne.Size {
	return g.minSize
}

// CreateRenderer creates the renderer for this widget
func (g *LatencyGraph) CreateRenderer() fyne.WidgetRenderer {
	return &latencyGraphRenderer{
		graph: g,
	}
}

// latencyGraphRenderer handles the drawing of the graph
type latencyGraphRenderer struct {
	graph   *LatencyGraph
	objects []fyne.CanvasObject
}

func (r *latencyGraphRenderer) Destroy() {}

func (r *latencyGraphRenderer) Layout(size fyne.Size) {
	// Objects are recreated on each refresh, no layout needed
}

func (r *latencyGraphRenderer) MinSize() fyne.Size {
	return r.graph.minSize
}

func (r *latencyGraphRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *latencyGraphRenderer) Refresh() {
	r.objects = r.createGraphObjects()
	canvas.Refresh(r.graph)
}

func (r *latencyGraphRenderer) createGraphObjects() []fyne.CanvasObject {
	size := r.graph.Size()
	if size.Width < 10 || size.Height < 10 {
		size = r.graph.minSize
	}

	objects := make([]fyne.CanvasObject, 0)

	// Background rectangle
	bg := canvas.NewRectangle(ColorBg)
	bg.Resize(size)
	bg.Move(fyne.NewPos(0, 0))
	objects = append(objects, bg)

	// Draw grid lines (horizontal)
	for i := 1; i < 4; i++ {
		y := size.Height * float32(i) / 4
		line := canvas.NewLine(ColorGrid)
		line.Position1 = fyne.NewPos(0, y)
		line.Position2 = fyne.NewPos(size.Width, y)
		line.StrokeWidth = 0.5
		objects = append(objects, line)
	}

	data := r.graph.data
	if len(data) < 2 {
		return objects
	}

	// Find max latency for scaling (ignore timeouts which are -1)
	maxLatency := 100.0 // Minimum scale of 100ms
	for _, lat := range data {
		if lat > maxLatency {
			maxLatency = lat
		}
	}
	// Add 20% headroom
	maxLatency *= 1.2

	// Calculate point spacing
	pointWidth := size.Width / float32(r.graph.maxPoints-1)
	startX := size.Width - float32(len(data)-1)*pointWidth

	// Draw the line graph
	padding := float32(4) // Padding from top/bottom
	graphHeight := size.Height - padding*2

	for i := 0; i < len(data)-1; i++ {
		lat1 := data[i]
		lat2 := data[i+1]

		x1 := startX + float32(i)*pointWidth
		x2 := startX + float32(i+1)*pointWidth

		// Skip if both points are timeouts
		if lat1 < 0 && lat2 < 0 {
			// Draw timeout indicator dots
			dot := canvas.NewCircle(ColorTimeout)
			dot.Resize(fyne.NewSize(3, 3))
			dot.Move(fyne.NewPos(x1-1.5, size.Height/2-1.5))
			objects = append(objects, dot)
			continue
		}

		// Handle timeout on first point
		if lat1 < 0 {
			// Draw timeout dot and start from middle
			dot := canvas.NewCircle(ColorTimeout)
			dot.Resize(fyne.NewSize(3, 3))
			dot.Move(fyne.NewPos(x1-1.5, size.Height/2-1.5))
			objects = append(objects, dot)
			continue
		}

		// Handle timeout on second point
		if lat2 < 0 {
			// Draw line to middle, then timeout dot
			y1 := padding + graphHeight*(1-float32(lat1/maxLatency))
			y2 := size.Height / 2

			lineColor := getLatencyColor(lat1)
			line := canvas.NewLine(lineColor)
			line.Position1 = fyne.NewPos(x1, y1)
			line.Position2 = fyne.NewPos(x2, y2)
			line.StrokeWidth = 2
			objects = append(objects, line)

			dot := canvas.NewCircle(ColorTimeout)
			dot.Resize(fyne.NewSize(3, 3))
			dot.Move(fyne.NewPos(x2-1.5, y2-1.5))
			objects = append(objects, dot)
			continue
		}

		// Both points have valid latency
		y1 := padding + graphHeight*(1-float32(lat1/maxLatency))
		y2 := padding + graphHeight*(1-float32(lat2/maxLatency))

		// Use gradient color based on the higher latency of the two points
		lineColor := getLatencyColor(max(lat1, lat2))

		line := canvas.NewLine(lineColor)
		line.Position1 = fyne.NewPos(x1, y1)
		line.Position2 = fyne.NewPos(x2, y2)
		line.StrokeWidth = 2
		objects = append(objects, line)
	}

	// Draw small dots at data points for visual clarity
	for i, lat := range data {
		if lat < 0 {
			continue
		}
		x := startX + float32(i)*pointWidth
		y := padding + graphHeight*(1-float32(lat/maxLatency))

		dotColor := getLatencyColor(lat)
		dot := canvas.NewCircle(dotColor)
		dot.Resize(fyne.NewSize(4, 4))
		dot.Move(fyne.NewPos(x-2, y-2))
		objects = append(objects, dot)
	}

	return objects
}

// getLatencyColor returns the appropriate color for a given latency value
func getLatencyColor(latency float64) color.Color {
	if latency < 0 {
		return ColorTimeout
	}
	if latency < ThresholdGood {
		return ColorGood
	}
	if latency < ThresholdMedium {
		return ColorMedium
	}
	return ColorHigh
}
