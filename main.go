package main

import (
	"fmt"
	"reflect"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/afroash/visual-mtr/network"
	"github.com/afroash/visual-mtr/ui"
)

type VisualMTR struct {
	app           fyne.App
	window        fyne.Window
	hostnameEntry *widget.Entry
	startButton   *widget.Button
	stopButton    *widget.Button
	statusLabel   *widget.Label
	hopList       *widget.List
	scanner       *network.Scanner
	hops          []network.NetworkHop
	hopsMutex     sync.RWMutex
	updateChan    chan network.HopUpdate
}

func NewVisualMTR() *VisualMTR {
	myApp := app.New()

	window := myApp.NewWindow("Visual MTR - Network Path Health Monitor")
	window.Resize(fyne.NewSize(800, 600))

	vm := &VisualMTR{
		app:        myApp,
		window:     window,
		hops:       make([]network.NetworkHop, 0),
		updateChan: make(chan network.HopUpdate, 100),
	}

	vm.setupUI()
	vm.setupMenu()
	vm.setupCloseHandler()
	return vm
}

func (vm *VisualMTR) setupUI() {
	// Top section: Hostname entry and buttons
	vm.hostnameEntry = widget.NewEntry()
	vm.hostnameEntry.SetPlaceHolder("Enter hostname or IP address (e.g., google.com)")
	// Ensure entry is enabled and focusable
	vm.hostnameEntry.Enable()

	vm.startButton = widget.NewButton("Start", vm.onStart)
	vm.stopButton = widget.NewButton("Stop", vm.onStop)
	vm.stopButton.Disable()

	topBar := container.NewBorder(nil, nil, nil,
		container.NewHBox(vm.startButton, vm.stopButton),
		vm.hostnameEntry)

	// Status label - shows current operation state
	vm.statusLabel = widget.NewLabel("Ready - Enter a hostname and click Start")
	vm.statusLabel.TextStyle = fyne.TextStyle{Italic: true}

	// Status bar container with some padding
	statusBar := container.NewHBox(
		widget.NewLabel("Status:"),
		vm.statusLabel,
	)

	// Combine top bar and status into header section
	topSection := container.NewVBox(topBar, statusBar)

	// Hop list with custom data binding
	vm.hopList = widget.NewList(
		vm.hopListLength,
		vm.hopListCreateItem,
		vm.hopListUpdateItem,
	)

	// Create header row for table
	header := container.NewHBox(
		widget.NewLabel("Hop#"),
		widget.NewLabel("  "),
		widget.NewLabel("IP Address"),
		widget.NewLabel("  "),
		widget.NewLabel("Latency"),
		widget.NewLabel("  "),
		widget.NewLabel("Loss"),
		widget.NewLabel("  "),
		widget.NewLabel("Status"),
		widget.NewLabel("  "),
		widget.NewLabel("Latency Graph"),
	)
	headerTextStyle := fyne.TextStyle{Bold: true}
	for _, obj := range header.Objects {
		if label, ok := obj.(*widget.Label); ok {
			label.TextStyle = headerTextStyle
		}
	}

	// Scrollable container for hop data
	scrollContainer := container.NewScroll(vm.hopList)
	scrollContainer.SetMinSize(fyne.NewSize(0, 400))

	// Combine header and scrollable list
	listWithHeader := container.NewBorder(header, nil, nil, nil, scrollContainer)

	// Main layout
	content := container.NewBorder(topSection, nil, nil, nil, listWithHeader)
	vm.window.SetContent(content)
}

// setupMenu creates the application menu with File/Quit option
func (vm *VisualMTR) setupMenu() {
	quitItem := fyne.NewMenuItem("Quit", func() {
		vm.onQuit()
	})

	fileMenu := fyne.NewMenu("File", quitItem)
	mainMenu := fyne.NewMainMenu(fileMenu)
	vm.window.SetMainMenu(mainMenu)
}

// setupCloseHandler handles window close events
func (vm *VisualMTR) setupCloseHandler() {
	vm.window.SetCloseIntercept(func() {
		vm.onQuit()
	})
}

// onQuit handles application exit with proper cleanup
func (vm *VisualMTR) onQuit() {
	// Stop any running scanner
	if vm.scanner != nil {
		vm.hopsMutex.Lock()
		scanner := vm.scanner
		vm.scanner = nil
		vm.hopsMutex.Unlock()

		if scanner != nil {
			scanner.Stop()
		}
	}
	// Close the application
	vm.app.Quit()
}

// Callback functions for the list widget
func (vm *VisualMTR) hopListLength() int {
	vm.hopsMutex.RLock()
	defer vm.hopsMutex.RUnlock()
	return len(vm.hops)
}

func (vm *VisualMTR) hopListCreateItem() fyne.CanvasObject {
	// Create table-like layout with 6 columns: Hop#, IP, Latency, Loss, Status, Graph
	hopNumLabel := widget.NewLabel("")
	hopNumLabel.TextStyle = fyne.TextStyle{Bold: true}

	ipLabel := widget.NewLabel("")
	ipLabel.TextStyle = fyne.TextStyle{Bold: true}

	latencyLabel := widget.NewLabel("")
	lossLabel := widget.NewLabel("")
	statusLabel := widget.NewLabel("")

	// Create the latency graph widget
	graph := ui.NewLatencyGraph()

	// Use HBox with proper spacing for table-like appearance
	// Structure: [Hop#, IP, Latency, Loss, Status, Graph]
	return container.NewHBox(
		hopNumLabel,
		widget.NewLabel("  "), // Spacer
		ipLabel,
		widget.NewLabel("  "), // Spacer
		latencyLabel,
		widget.NewLabel("  "), // Spacer
		lossLabel,
		widget.NewLabel("  "), // Spacer
		statusLabel,
		widget.NewLabel("  "), // Spacer
		graph,
	)
}

func (vm *VisualMTR) hopListUpdateItem(id widget.ListItemID, obj fyne.CanvasObject) {
	vm.hopsMutex.RLock()
	defer vm.hopsMutex.RUnlock()

	if id >= len(vm.hops) {
		return
	}

	hop := vm.hops[id]

	// Access the HBox container's objects using reflection
	// container.NewHBox returns a container with an unexported Objects field
	// We use reflection to access it
	val := reflect.ValueOf(obj).Elem()
	objectsField := val.FieldByName("Objects")
	if !objectsField.IsValid() {
		return
	}

	// Safely convert to []fyne.CanvasObject with type assertion check
	objectsInterface := objectsField.Interface()
	objects, ok := objectsInterface.([]fyne.CanvasObject)
	if !ok || len(objects) < 11 {
		return
	}

	// Objects structure: [hopNumLabel, spacer, ipLabel, spacer, latencyLabel, spacer, lossLabel, spacer, statusLabel, spacer, graph]
	hopNumLabel := objects[0].(*widget.Label)
	ipLabel := objects[2].(*widget.Label)
	latencyLabel := objects[4].(*widget.Label)
	lossLabel := objects[6].(*widget.Label)
	statusLabel := objects[8].(*widget.Label)
	graph := objects[10].(*ui.LatencyGraph)

	// Column 1: Hop Number
	hopNumLabel.SetText(fmt.Sprintf("%d", id+1))

	// Column 2: IP Address
	ipLabel.SetText(hop.IP)

	// Column 3: Latency
	if hop.AvgLatency > 0 {
		latencyLabel.SetText(fmt.Sprintf("%.2f ms", hop.AvgLatency))
	} else {
		latencyLabel.SetText("N/A")
	}

	// Column 4: Packet Loss
	if hop.LossPercent > 0 {
		lossLabel.SetText(fmt.Sprintf("%.1f%%", hop.LossPercent))
	} else {
		lossLabel.SetText("0%")
	}

	// Column 5: Status (computed dynamically)
	status := vm.computeStatus(hop)
	statusLabel.SetText(status)

	// Column 6: Latency Graph - update with history data
	graph.SetData(hop.LatencyHistory)
}

// computeStatus determines the status of a hop based on its metrics
func (vm *VisualMTR) computeStatus(hop network.NetworkHop) string {
	if hop.AvgLatency > 0 {
		return "Active"
	}
	if hop.IP == "" {
		return "Unknown"
	}
	return "Timeout"
}

func (vm *VisualMTR) onStart() {
	hostname := vm.hostnameEntry.Text
	if hostname == "" {
		vm.statusLabel.SetText("Error: Please enter a hostname")
		return
	}

	// Update UI state only after validation passes
	vm.startButton.Disable()
	vm.hostnameEntry.Disable()
	vm.stopButton.Enable()
	vm.statusLabel.SetText("Starting...")

	// Create new scanner with mutex protection
	vm.hopsMutex.Lock()
	vm.scanner = network.NewScanner(hostname)
	scanner := vm.scanner
	vm.hopsMutex.Unlock()

	// Start scanning in background
	go func() {
		err := scanner.Start()
		if err != nil {
			fmt.Printf("Error starting scanner: %v\n", err)
			// Reset UI state on error - must use fyne.Do() from goroutine
			fyne.Do(func() {
				vm.startButton.Enable()
				vm.hostnameEntry.Enable()
				vm.stopButton.Disable()
				vm.statusLabel.SetText(fmt.Sprintf("Error: %v", err))
			})
			// Clear scanner reference
			vm.hopsMutex.Lock()
			vm.scanner = nil
			vm.hopsMutex.Unlock()
			return
		}
	}()

	// Start update handler goroutines
	go vm.handleUpdates()
	go vm.handleStatus()
}

func (vm *VisualMTR) onStop() {
	// Safely stop scanner with mutex protection
	vm.hopsMutex.Lock()
	scanner := vm.scanner
	vm.scanner = nil
	vm.hopsMutex.Unlock()

	if scanner != nil {
		scanner.Stop()
	}

	vm.startButton.Enable()
	vm.hostnameEntry.Enable()
	vm.stopButton.Disable()
	vm.statusLabel.SetText("Stopped - Enter a hostname and click Start")

	// Clear hops
	vm.hopsMutex.Lock()
	vm.hops = make([]network.NetworkHop, 0)
	vm.hopsMutex.Unlock()

	// Refresh UI
	vm.hopList.Refresh()
}

// handleUpdates processes hop updates from the scanner and updates the UI
// This runs in a background goroutine and uses Fyne's thread-safe UI update mechanism
func (vm *VisualMTR) handleUpdates() {
	// Safely get scanner reference to avoid nil pointer dereference
	// Use a local variable to hold the scanner while we get the updates channel
	vm.hopsMutex.RLock()
	scanner := vm.scanner
	vm.hopsMutex.RUnlock()

	if scanner == nil {
		return
	}

	updates := scanner.Updates()

	for update := range updates {
		vm.hopsMutex.Lock()

		// Ensure we have enough slots in the hops slice
		for len(vm.hops) <= update.Index {
			vm.hops = append(vm.hops, network.NetworkHop{})
		}

		// Update the hop data
		vm.hops[update.Index] = update.Hop
		vm.hopsMutex.Unlock()

		// Update UI on main thread using fyne.Do()
		// Since Fyne v2.6.0, all UI updates from goroutines must use fyne.Do()
		fyne.Do(func() {
			vm.hopList.Refresh()
		})
	}
}

// handleStatus processes status updates from the scanner and updates the status label
func (vm *VisualMTR) handleStatus() {
	vm.hopsMutex.RLock()
	scanner := vm.scanner
	vm.hopsMutex.RUnlock()

	if scanner == nil {
		return
	}

	statusChan := scanner.Status()

	for status := range statusChan {
		statusText := vm.formatStatus(status)
		fyne.Do(func() {
			vm.statusLabel.SetText(statusText)
		})
	}
}

// formatStatus converts a ScannerStatus to a user-friendly message
func (vm *VisualMTR) formatStatus(status network.ScannerStatus) string {
	vm.hopsMutex.RLock()
	hopCount := len(vm.hops)
	vm.hopsMutex.RUnlock()

	switch status {
	case network.StatusTracing:
		return "🔍 Tracing route to destination..."
	case network.StatusPinging:
		return fmt.Sprintf("📡 Monitoring %d hops...", hopCount)
	case network.StatusStopped:
		return "⏹ Stopped"
	case network.StatusError:
		return "❌ Error occurred"
	default:
		return string(status)
	}
}

func (vm *VisualMTR) Run() {
	vm.window.ShowAndRun()
}

func main() {
	vm := NewVisualMTR()
	vm.Run()
}
