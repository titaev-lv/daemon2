package manager

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"ctdaemon/internal/logger"
	//"daemon2/internal/collectorevents"
	"ctdaemon/internal/config"
	"ctdaemon/internal/state"
	//"daemon2/internal/exchange"
	//"daemon2/internal/trade"
	//"daemon2/internal/tradedata"
)

var (
	ErrAlreadyRunning = errors.New("system is already running")
	ErrNotRunning     = errors.New("system is not running")
)

type Manager struct {
	cfg *config.Config
	//	tradeData    *tradedata.Monitor
	//	exchangeExec *exchange.Monitor
	//	collector    *collectorevents.Monitor
	//	trade        *trade.Monitor
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	shutdownOnce  sync.Once
	isRunning     bool
	mu            sync.RWMutex
	startTime     time.Time
	shutdownTime  time.Time
	shutdownError error
}

const (
	// GracefulShutdownTimeout is the maximum time to wait for graceful shutdown
	GracefulShutdownTimeout = 30 * time.Second
)

// New creates a new manager
func New(cfg *config.Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize state manager (loads from disk if exists)
	stateMgr := state.GetInstance()
	logger.Get("manager").Info("State manager initialized", "is_running", stateMgr.IsRunning())

	//td := tradedata.NewMonitor(cfg)
	//ee := exchange.NewMonitor(cfg)
	//exchange.SetOrderBookConfig(cfg) // Initialize global config for exchange package
	//ce := collectorevents.NewMonitor(cfg)
	//t := trade.NewMonitor(cfg, td, ee)

	return &Manager{
		cfg: cfg,
		//	tradeData:    td,
		//	exchangeExec: ee,
		//	collector:    ce,
		//	trade:        t,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins all system components
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		logger.Get("manager").Info("System already running")
		return ErrAlreadyRunning
	}

	logger.Get("manager").Info("Starting system components...")
	m.isRunning = true
	m.startTime = time.Now()

	// Persist running state to disk
	if err := state.GetInstance().SetRunning(true); err != nil {
		logger.Get("manager").Error("Failed to persist running state", "error", err)
	}

	// Start components in order (dependency order)
	// m.tradeData.Start()
	// logger.Get("manager").Debug("Trade data monitor started")

	// m.exchangeExec.Start()
	// logger.Get("manager").Debug("Exchange executor monitor started")

	// m.collector.Start()
	// logger.Get("manager").Debug("Collector events monitor started")

	// m.trade.Start()
	// logger.Get("manager").Debug("Trade monitor started")

	logger.Get("manager").Info("All system components started successfully")
	return nil
}

// Stop gracefully stops all system components
func (m *Manager) Stop() error {
	m.mu.RLock()
	isRunning := m.isRunning
	m.mu.RUnlock()

	if !isRunning {
		logger.Get("manager").Info("System not running, skipping shutdown")
		return ErrNotRunning
	}

	m.shutdownOnce.Do(func() {
		m.doStop()
	})
	return m.shutdownError
}

// doStop performs the actual shutdown
func (m *Manager) doStop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		logger.Get("manager").Info("System not running, skipping shutdown")
		return
	}

	logger.Get("manager").Info("Initiating graceful shutdown...", "timeout", GracefulShutdownTimeout)
	m.isRunning = false
	m.shutdownTime = time.Now()

	// Persist stopped state to disk
	if err := state.GetInstance().SetRunning(false); err != nil {
		logger.Get("manager").Error("Failed to persist stopped state", "error", err)
	}

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), GracefulShutdownTimeout)
	defer cancel()

	// Channel to track shutdown completion
	done := make(chan error, 1)

	// Run shutdown in goroutine to allow timeout
	go m.shutdownComponents(done)

	// Wait for shutdown or timeout
	select {
	case err := <-done:
		if err != nil {
			m.shutdownError = err
			logger.Get("manager").Error("Shutdown error", "error", err)
		} else {
			logger.Get("manager").Info("Graceful shutdown completed successfully")
		}
	case <-shutdownCtx.Done():
		m.shutdownError = shutdownCtx.Err()
		logger.Get("manager").Error("Shutdown timeout, force stopping", "error", m.shutdownError)
		m.cancel() // Force cancel context
	}
}

// shutdownComponents stops all components in reverse order
func (m *Manager) shutdownComponents(done chan<- error) {
	var lastErr error
	log := logger.Get("manager")

	// Stop components in reverse dependency order
	// log.Info("Stopping trade monitor...")
	// m.trade.Stop()
	log.Info("SHUTDOWN MANAGER...")
	// log.Info("Stopping collector events monitor...")
	// m.collector.Stop()

	// log.Info("Stopping exchange executor monitor...")
	// m.exchangeExec.Stop()

	// log.Info("Stopping trade data monitor...")
	// m.tradeData.Stop()

	// Cancel main context after all components stopped
	m.cancel()

	done <- lastErr
}

// Status returns the current system status
func (m *Manager) Status() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	uptime := time.Duration(0)
	if m.isRunning {
		uptime = time.Since(m.startTime)
	} else if !m.startTime.IsZero() {
		uptime = m.shutdownTime.Sub(m.startTime)
	}

	// Format uptime as "1d 2h 23m 45s" with spaces
	totalSeconds := int64(uptime.Seconds())
	days := totalSeconds / 86400
	hours := (totalSeconds % 86400) / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	var uptimeStr string
	if days > 0 {
		uptimeStr = fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	} else if hours > 0 {
		uptimeStr = fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		uptimeStr = fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		uptimeStr = fmt.Sprintf("%ds", seconds)
	}

	return map[string]interface{}{
		"running": m.isRunning,
		"uptime":  uptimeStr,
		"start_time": func() interface{} {
			if m.startTime.IsZero() {
				return nil
			}
			return m.startTime
		}(),
		"shutdown_time": func() interface{} {
			if m.shutdownTime.IsZero() {
				return nil
			}
			return m.shutdownTime
		}(),
		"error": m.shutdownError,
	}
}

// IsRunning returns whether the system is currently running
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isRunning
}

// GetContext returns the manager's context for controlled cancellation
func (m *Manager) GetContext() context.Context {
	return m.ctx
}

func (m *Manager) Shutdown() {
	m.Stop()
	m.cancel()
	m.wg.Wait()
}
