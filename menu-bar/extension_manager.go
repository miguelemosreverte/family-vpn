package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// ExtensionInfo represents a managed extension
type ExtensionInfo struct {
	Name       string
	BinaryPath string
	Args       []string
	Process    *exec.Cmd
	Running    bool
	LastStart  time.Time
	RestartCh  chan bool
}

// ExtensionManager manages extension processes
type ExtensionManager struct {
	extensions map[string]*ExtensionInfo
	mutex      sync.RWMutex
	repoDir    string
}

// NewExtensionManager creates a new extension manager
func NewExtensionManager(repoDir string) *ExtensionManager {
	return &ExtensionManager{
		extensions: make(map[string]*ExtensionInfo),
		repoDir:    repoDir,
	}
}

// RegisterExtension registers an extension to be managed
func (m *ExtensionManager) RegisterExtension(name, binaryPath string, args []string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.extensions[name] = &ExtensionInfo{
		Name:       name,
		BinaryPath: binaryPath,
		Args:       args,
		Running:    false,
		RestartCh:  make(chan bool, 1),
	}
}

// StartExtension starts a registered extension
func (m *ExtensionManager) StartExtension(name string) error {
	m.mutex.Lock()
	ext, exists := m.extensions[name]
	if !exists {
		m.mutex.Unlock()
		return fmt.Errorf("extension %s not registered", name)
	}
	m.mutex.Unlock()

	if ext.Running {
		log.Printf("[EXT] Extension %s is already running", name)
		return nil
	}

	// Check if binary exists
	if _, err := os.Stat(ext.BinaryPath); os.IsNotExist(err) {
		return fmt.Errorf("extension binary not found: %s", ext.BinaryPath)
	}

	// Start the extension process
	cmd := exec.Command(ext.BinaryPath, ext.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start extension %s: %v", name, err)
	}

	ext.Process = cmd
	ext.Running = true
	ext.LastStart = time.Now()

	log.Printf("[EXT] Started extension: %s (PID %d)", name, cmd.Process.Pid)

	// Monitor process in background
	go m.monitorExtension(name, ext, cmd)

	return nil
}

// StopExtension stops a running extension
func (m *ExtensionManager) StopExtension(name string) error {
	m.mutex.Lock()
	ext, exists := m.extensions[name]
	if !exists {
		m.mutex.Unlock()
		return fmt.Errorf("extension %s not registered", name)
	}
	m.mutex.Unlock()

	if !ext.Running {
		return nil
	}

	if ext.Process != nil && ext.Process.Process != nil {
		log.Printf("[EXT] Stopping extension: %s", name)
		if err := ext.Process.Process.Kill(); err != nil {
			log.Printf("[EXT] Failed to kill %s: %v", name, err)
		}
		ext.Process.Wait()
	}

	ext.Running = false
	ext.Process = nil

	return nil
}

// RestartExtension restarts an extension (rebuild + restart)
func (m *ExtensionManager) RestartExtension(name string) error {
	log.Printf("[EXT] Restarting extension: %s", name)

	// Stop the extension first
	if err := m.StopExtension(name); err != nil {
		log.Printf("[EXT] Failed to stop %s: %v", name, err)
	}

	// Wait a moment for cleanup
	time.Sleep(500 * time.Millisecond)

	// Rebuild the extension
	if err := m.rebuildExtension(name); err != nil {
		return fmt.Errorf("failed to rebuild %s: %v", name, err)
	}

	// Start it again
	if err := m.StartExtension(name); err != nil {
		return fmt.Errorf("failed to start %s: %v", name, err)
	}

	log.Printf("[EXT] Successfully restarted extension: %s", name)
	return nil
}

// getGoBinary returns the path to the go binary
func getGoBinary() string {
	// Try go in PATH first
	if _, err := exec.LookPath("go"); err == nil {
		return "go"
	}

	// Try common installation locations
	commonPaths := []string{
		"/usr/local/go/bin/go",
		"/usr/bin/go",
		"/opt/homebrew/bin/go",
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Default to "go" and let it fail with a clear error
	return "go"
}

// rebuildExtension rebuilds an extension binary
func (m *ExtensionManager) rebuildExtension(name string) error {
	log.Printf("[EXT] Rebuilding extension: %s", name)

	// Pull latest changes first
	pullCmd := exec.Command("git", "-C", m.repoDir, "pull", "origin", "main")
	if output, err := pullCmd.CombinedOutput(); err != nil {
		log.Printf("[EXT] Git pull output: %s", string(output))
		return fmt.Errorf("git pull failed: %v", err)
	}

	// Build the extension using the correct go binary
	goBin := getGoBinary()
	log.Printf("[EXT] Using go binary: %s", goBin)

	extDir := filepath.Join(m.repoDir, "extensions", name)
	buildCmd := exec.Command(goBin, "build", "-o", name+"-extension", ".")
	buildCmd.Dir = extDir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("build failed: %v", err)
	}

	log.Printf("[EXT] Successfully rebuilt: %s", name)
	return nil
}

// monitorExtension monitors an extension process for crashes
func (m *ExtensionManager) monitorExtension(name string, ext *ExtensionInfo, cmd *exec.Cmd) {
	// Wait for process to exit
	err := cmd.Wait()

	m.mutex.Lock()
	wasRunning := ext.Running
	ext.Running = false
	ext.Process = nil
	m.mutex.Unlock()

	if wasRunning {
		// Process crashed or was killed
		log.Printf("[EXT] Extension %s exited: %v", name, err)

		// Auto-restart if it crashed (not manually stopped)
		if time.Since(ext.LastStart) > 10*time.Second {
			log.Printf("[EXT] Auto-restarting crashed extension: %s", name)
			time.Sleep(2 * time.Second)
			if err := m.StartExtension(name); err != nil {
				log.Printf("[EXT] Failed to auto-restart %s: %v", name, err)
			}
		}
	}
}

// StartAll starts all registered extensions
func (m *ExtensionManager) StartAll() error {
	m.mutex.RLock()
	names := make([]string, 0, len(m.extensions))
	for name := range m.extensions {
		names = append(names, name)
	}
	m.mutex.RUnlock()

	for _, name := range names {
		if err := m.StartExtension(name); err != nil {
			log.Printf("[EXT] Failed to start %s: %v", name, err)
		}
	}

	return nil
}

// StopAll stops all running extensions
func (m *ExtensionManager) StopAll() error {
	m.mutex.RLock()
	names := make([]string, 0, len(m.extensions))
	for name := range m.extensions {
		names = append(names, name)
	}
	m.mutex.RUnlock()

	for _, name := range names {
		if err := m.StopExtension(name); err != nil {
			log.Printf("[EXT] Failed to stop %s: %v", name, err)
		}
	}

	return nil
}

// IsRunning checks if an extension is running
func (m *ExtensionManager) IsRunning(name string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	ext, exists := m.extensions[name]
	if !exists {
		return false
	}
	return ext.Running
}
