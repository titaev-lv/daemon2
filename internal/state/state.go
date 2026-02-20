// Package state отвечает за сохранение и восстановление состояния сервиса
// Позволяет сервису помнить был ли он запущен перед отключением
// При перезагрузке системы сервис автоматически продолжит работу если был включен
package state

import (
	"encoding/json"
	"os"
	"sync"

	"trader/internal/logger"
)

// State - структура для сохранения состояния сервиса на диск
// Сейчас содержит только IsRunning, но в будущем может содержать больше информации
// (например: текущие позиции, кэш конфигурации и т.д.)
type State struct {
	// IsRunning - был ли сервис запущен при последнем выключении
	IsRunning bool `json:"is_running"`
}

// Manager - управляет состоянием сервиса
// Обеспечивает потокобезопасный доступ к состоянию и его сохранение на диск
type Manager struct {
	// filePath - путь где сохраняется состояние (обычно state/trader.state)
	filePath string
	// state - текущее состояние в памяти
	state *State
	// mu - мьютекс для защиты доступа к state при многопоточности
	mu sync.RWMutex
}

// Глобальные переменные для Singleton паттерна
var (
	// instance - единственный экземпляр Manager во всем приложении
	instance *Manager
	// once - гарантирует что инициализация произойдет только один раз
	once sync.Once
)

// GetInstance - возвращает singleton экземпляр Manager
// Использует sync.Once чтобы гарантировать что Manager создается только один раз
// Автоматически загружает состояние с диска если файл существует
func GetInstance() *Manager {
	once.Do(func() {
		// Инициализируем с пустым состоянием (сервис не запущен)
		instance = &Manager{
			filePath: "state/trader.state",
			state:    &State{IsRunning: false},
		}
		// Пытаемся загрузить сохраненное состояние с диска
		if err := instance.Load(); err != nil {
			// Если файла нет - это нормально, начнем с чистого состояния
			logger.Get("state").Info("Starting with fresh state")
		}
	})
	return instance
}

// Save - сохраняет текущее состояние в JSON файл на диск
// Используется когда изменяется состояние (Start/Stop)
// JSON формат выбран для удобства просмотра файла человеком
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Переводим структуру State в JSON с форматированием для читаемости
	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}

	// Пишем JSON в файл (создает файл если его нет, перезаписывает если существует)
	return os.WriteFile(m.filePath, data, 0644)
}

// Load - загружает состояние из JSON файла с диска
// Вызывается при инициализации Manager
// Если файл не существует - возвращает ошибку и используется состояние по умолчанию
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Читаем весь файл в памяти
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return err
	}

	// Парсим JSON в структуру State
	if err := json.Unmarshal(data, m.state); err != nil {
		logger.Get("state").Error("Failed to unmarshal state", "error", err)
		return err
	}

	// Логируем что состояние успешно загружено
	logger.Get("state").Info("State loaded from file", "is_running", m.state.IsRunning)
	return nil
}

// SetRunning - устанавливает флаг IsRunning и сохраняет на диск
// Используется менеджером при Start() и Stop()
// Гарантирует что состояние всегда синхронизировано между памятью и диском
func (m *Manager) SetRunning(running bool) error {
	m.mu.Lock()
	m.state.IsRunning = running
	m.mu.Unlock()

	// Логируем изменение состояния
	logger.Get("state").Info("State changed", "is_running", running)
	// Сохраняем на диск
	return m.Save()
}

// IsRunning - возвращает текущий флаг IsRunning
// Потокобезопасно читает значение через мьютекс
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.IsRunning
}
