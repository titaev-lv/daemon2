// Package manager содержит основную логику управления демоном
// Manager отвечает за:
// - Управление lifecycle приложения (старт, остановка, перезагрузка)
// - Синхронизацию между компонентами (Monitor, Trader, TaskFetcher и т.д.)
// - Обработку context для корректного завершения всех goroutine
package manager

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"trader/internal/config"
	"trader/internal/logger"
	"trader/internal/state"
	//"daemon2/internal/collectorevents"
	//"daemon2/internal/exchange"
	//"daemon2/internal/trade"
	//"daemon2/internal/tradedata"
)

// Ошибки для управления состоянием
var (
	// ErrAlreadyRunning - попытка запустить уже работающий менеджер
	ErrAlreadyRunning = errors.New("system is already running")
	// ErrNotRunning - попытка остановить не работающий менеджер
	ErrNotRunning = errors.New("system is not running")
)

// Manager - центральный координатор всех компонентов системы
// Координирует работу всех компонентов и управляет их жизненным циклом
type Manager struct {
	cfg *config.Config
	//	tradeData    *tradedata.Monitor
	//	exchangeExec *exchange.Monitor
	//	collector    *collectorevents.Monitor
	//	trade        *trade.Monitor
	// ctx/cancel - контекст для сигнализации о необходимости выключения всем goroutine
	ctx    context.Context
	cancel context.CancelFunc
	// wg - WaitGroup для отслеживания всех запущенных goroutine
	// Используется для корректного завершения при shutdown
	wg sync.WaitGroup
	// shutdownOnce - гарантирует что shutdown выполнится только один раз
	shutdownOnce sync.Once
	// isRunning - флаг текущего состояния (запущен или остановлен)
	isRunning bool
	// mu - мьютекс для защиты access к полям при многопоточности
	mu sync.RWMutex
	// startTime - время когда менеджер был запущен
	startTime time.Time
	// shutdownTime - время когда менеджер был остановлен
	shutdownTime time.Time
	// shutdownError - ошибка если произошла при shutdown
	shutdownError error
}

// GracefulShutdownTimeout - максимальное время для корректного завершения всех goroutine
// После этого они будут принудительно завершены
const (
	GracefulShutdownTimeout = 30 * time.Second
)

// New - создает новый менеджер с указанной конфигурацией
// Инициализирует контекст и состояние из сохраненного на диске
func New(cfg *config.Config) *Manager {
	// Создаем контекст который можно отменить (для shutdown)
	ctx, cancel := context.WithCancel(context.Background())

	// Инициализируем менеджер состояния (загружает из диска если файл существует)
	// Инициализируем менеджер состояния (загружает из диска если файл существует)
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

// Start - запускает все компоненты системы
// Возвращает ошибку если система уже работает
// Устанавливает startTime и сохраняет состояние в файл
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		logger.Get("manager").Info("System already running")
		return ErrAlreadyRunning
	}

	// Логируем что начинаем запуск
	logger.Get("manager").Info("Starting system components...")
	m.isRunning = true
	m.startTime = time.Now()

	// Сохраняем состояние на диск (чтобы при перезагрузке демона он автоматически стартанул)
	if err := state.GetInstance().SetRunning(true); err != nil {
		logger.Get("manager").Error("Failed to persist running state", "error", err)
	}

	// Запускаем компоненты в порядке зависимостей
	// Сначала те которые не зависят от других, потом те которые зависят
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

// Stop - корректно останавливает все компоненты системы
// Возвращает ошибку если система не работает
// Сохраняет состояние shutdown в файл
func (m *Manager) Stop() error {
	m.mu.RLock()
	isRunning := m.isRunning
	m.mu.RUnlock()

	if !isRunning {
		logger.Get("manager").Info("System not running, skipping shutdown")
		return ErrNotRunning
	}

	// shutdownOnce гарантирует что shutdown выполнится только один раз
	// даже если Stop() вызовут несколько раз одновременно
	m.shutdownOnce.Do(func() {
		m.doStop()
	})
	return m.shutdownError
}

// doStop - выполняет фактическое завершение работы
// Этап 1: помечает систему как остановленную
// Этап 2: отправляет сигнал всем goroutine через cancel()
// Этап 3: ждет их завершения с таймаутом
// Этап 4: если не завершились - принудительно отменяет контекст
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

	// Сохраняем на диск что система остановлена
	if err := state.GetInstance().SetRunning(false); err != nil {
		logger.Get("manager").Error("Failed to persist stopped state", "error", err)
	}

	// Создаем контекст с таймаутом для graceful shutdown
	// После истечения таймаута система будет принудительно выключена
	shutdownCtx, cancel := context.WithTimeout(context.Background(), GracefulShutdownTimeout)
	defer cancel()

	// Канал для получения результата shutdown
	done := make(chan error, 1)

	// Запускаем shutdown в отдельной goroutine чтобы иметь возможность таймаутировать
	go m.shutdownComponents(done)

	// Ждем либо завершения shutdown, либо истечения таймаута
	select {
	case err := <-done:
		if err != nil {
			m.shutdownError = err
			logger.Get("manager").Error("Shutdown error", "error", err)
		} else {
			logger.Get("manager").Info("Graceful shutdown completed successfully")
		}
	case <-shutdownCtx.Done():
		// Таймаут истек - все компоненты не завершились вовремя
		m.shutdownError = shutdownCtx.Err()
		logger.Get("manager").Error("Shutdown timeout, force stopping", "error", m.shutdownError)
		// Принудительно отменяем контекст чтобы все goroutine выключились
		m.cancel()
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
	// Форматируем uptime в понятный вид (дни, часы, минуты, секунды)
	if days > 0 {
		uptimeStr = fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	} else if hours > 0 {
		uptimeStr = fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		uptimeStr = fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		uptimeStr = fmt.Sprintf("%ds", seconds)
	}

	// Возвращаем информацию о статусе в виде map для REST API
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

// IsRunning - возвращает текущий статус (запущена ли система)
// Потокобезопасный доступ с использованием RLock
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isRunning
}

// GetContext - возвращает контекст менеджера для управления отменой
// Используется компонентами для получения сигнала о необходимости завершения
func (m *Manager) GetContext() context.Context {
	return m.ctx
}

func (m *Manager) Shutdown() {
	m.Stop()
	m.cancel()
	m.wg.Wait()
}
