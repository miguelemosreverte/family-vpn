package framework

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

// Extension is the interface that all extensions must implement
type Extension interface {
	// Name returns the extension name (e.g., "video", "screen-share")
	Name() string

	// Version returns the extension version
	Version() string

	// Start starts the extension
	Start() error

	// Stop stops the extension gracefully
	Stop() error

	// Health returns true if extension is healthy
	Health() bool
}

// ExtensionBase provides common functionality for all extensions
type ExtensionBase struct {
	name    string
	version string
	stopCh  chan bool
}

// NewExtensionBase creates a new extension base
func NewExtensionBase(name, version string) *ExtensionBase {
	return &ExtensionBase{
		name:    name,
		version: version,
		stopCh:  make(chan bool),
	}
}

// Name returns the extension name
func (e *ExtensionBase) Name() string {
	return e.name
}

// Version returns the extension version
func (e *ExtensionBase) Version() string {
	return e.version
}

// Run runs the extension with signal handling
func (e *ExtensionBase) Run(ext Extension) error {
	log.Printf("[%s] Starting extension v%s", ext.Name(), ext.Version())

	if err := ext.Start(); err != nil {
		return err
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Printf("[%s] Received shutdown signal", ext.Name())
	case <-e.stopCh:
		log.Printf("[%s] Received stop command", ext.Name())
	}

	log.Printf("[%s] Stopping extension...", ext.Name())
	if err := ext.Stop(); err != nil {
		log.Printf("[%s] Error during stop: %v", ext.Name(), err)
		return err
	}

	log.Printf("[%s] Stopped successfully", ext.Name())
	return nil
}

// RequestStop requests the extension to stop
func (e *ExtensionBase) RequestStop() {
	close(e.stopCh)
}
