package state

import (
	"encoding/json"
	"os"
	"sync"

	"ctdaemon/internal/logger"
)

// State represents the persistent state of the daemon
type State struct {
	IsRunning bool `json:"is_running"`
}

// Manager handles state persistence
type Manager struct {
	filePath string
	state    *State
	mu       sync.RWMutex
}

var (
	instance *Manager
	once     sync.Once
)

// GetInstance returns the singleton instance of Manager
func GetInstance() *Manager {
	once.Do(func() {
		instance = &Manager{
			filePath: "state/daemon.state",
			state:    &State{IsRunning: false},
		}
		// Try to load existing state
		if err := instance.Load(); err != nil {
			logger.Get("state").Info("Starting with fresh state")
		}
	})
	return instance
}

// Save persists the state to file
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.filePath, data, 0644)
}

// Load reads the state from file
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, m.state); err != nil {
		logger.Get("state").Error("Failed to unmarshal state", "error", err)
		return err
	}

	logger.Get("state").Info("State loaded from file", "is_running", m.state.IsRunning)
	return nil
}

// SetRunning sets the running state and saves
func (m *Manager) SetRunning(running bool) error {
	m.mu.Lock()
	m.state.IsRunning = running
	m.mu.Unlock()

	logger.Get("state").Info("State changed", "is_running", running)
	return m.Save()
}

// IsRunning returns whether the daemon should be running
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.IsRunning
}
