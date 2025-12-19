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
)

type VisualMTR struct {
	app           fyne.App
	window        fyne.Window
	hostnameEntry *widget.Entry
	startButton   *widget.Button
	stopButton    *widget.Button
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
	return vm
}

func (vm *VisualMTR) setupUI() {
	// Top section: Hostname entry and buttons
	vm.hostnameEntry = widget.NewEntry()
	vm.hostnameEntry.SetPlaceHolder("Enter hostname or IP address (e.g., google.com)")

	vm.startButton = widget.NewButton("Start", vm.onStart)
	vm.stopButton = widget.NewButton("Stop", vm.onStop)
	vm.stopButton.Disable()

	topBar := container.NewBorder(nil, nil, nil,
		container.NewHBox(vm.startButton, vm.stopButton),
		vm.hostnameEntry)

	// Hop list with custom data binding
	vm.hopList = widget.NewList(
		vm.hopListLength,
		vm.hopListCreateItem,
		vm.
			hopListUpdateItem,
	)

	// Scrollable container for hop data
	scrollContainer := container.NewScroll(vm.hopList)
	scrollContainer.SetMinSize(fyne.NewSize(0, 400))

	// Main layout
	content := container.NewBorder(topBar, nil, nil, nil, scrollContainer)
	vm.window.SetContent(content)
}

// Callback functions for the list widget
func (vm *VisualMTR) hopListLength() int {
	vm.hopsMutex.RLock()
	defer vm.hopsMutex.RUnlock()
	return len(vm.hops)
}

func (vm *VisualMTR) hopListCreateItem() fyne.CanvasObject {
	// Create a container with IP, Latency, and Loss labels
	ipLabel := widget.NewLabel("")
	ipLabel.TextStyle = fyne.TextStyle{Bold: true}

	latencyLabel := widget.NewLabel("")
	lossLabel := widget.NewLabel("")

	// Use HBox for layout - structure: [ipLabel, "Latency:", latencyLabel, "Loss:", lossLabel]
	return container.NewHBox(
		ipLabel,
		widget.NewLabel("Latency:"),
		latencyLabel,
		widget.NewLabel("Loss:"),
		lossLabel,
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

	objects := objectsField.Interface().([]fyne.CanvasObject)
	if len(objects) < 5 {
		return
	}

	ipLabel := objects[0].(*widget.Label)
	ipLabel.SetText(fmt.Sprintf("Hop %d: %s", id+1, hop.IP))

	latencyLabel := objects[2].(*widget.Label)
	lossLabel := objects[4].(*widget.Label)

	if hop.AvgLatency > 0 {
		latencyLabel.SetText(fmt.Sprintf("%.2f ms", hop.AvgLatency))
	} else {
		latencyLabel.SetText("N/A")
	}

	if hop.LossPercent > 0 {
		lossLabel.SetText(fmt.Sprintf("%.1f%%", hop.LossPercent))
	} else {
		lossLabel.SetText("0%")
	}
}

func (vm *VisualMTR) onStart() {
	hostname := vm.hostnameEntry.Text
	if hostname == "" {
		// TODO: Show error dialog
		return
	}

	vm.startButton.Disable()
	vm.hostnameEntry.Disable()
	vm.stopButton.Enable()

	// Create new scanner
	vm.scanner = network.NewScanner(hostname)

	// Start scanning in background
	go func() {
		err := vm.scanner.Start()
		if err != nil {
			// TODO: Show error dialog
			fmt.Printf("Error starting scanner: %v\n", err)
			return
		}
	}()

	// Start update handler goroutine
	go vm.handleUpdates()
}

func (vm *VisualMTR) onStop() {
	if vm.scanner != nil {
		vm.scanner.Stop()
		vm.scanner = nil
	}

	vm.startButton.Enable()
	vm.hostnameEntry.Enable()
	vm.stopButton.Disable()

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
	updates := vm.scanner.Updates()

	for update := range updates {
		vm.hopsMutex.Lock()

		// Ensure we have enough slots in the hops slice
		for len(vm.hops) <= update.Index {
			vm.hops = append(vm.hops, network.NetworkHop{})
		}

		// Update the hop data
		vm.hops[update.Index] = update.Hop
		vm.hopsMutex.Unlock()

		// Update UI on main thread (Fyne handles thread safety)
		vm.hopList.Refresh()
	}
}

func (vm *VisualMTR) Run() {
	vm.window.ShowAndRun()
}

func main() {
	vm := NewVisualMTR()
	vm.Run()
}
