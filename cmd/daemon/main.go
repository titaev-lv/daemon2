// Package main - точка входа в приложение ctdaemon
// ctdaemon - это демон для мониторинга и торговли на криптобиржах
// Может работать в режиме монитора (сбор данных), трейдера (исполнение стратегий) или обоих одновременно
package main

import (
	// "encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"ctdaemon/internal/api"
	"ctdaemon/internal/config"
	"ctdaemon/internal/db"
	"ctdaemon/internal/logger"
	"ctdaemon/internal/manager"
	"ctdaemon/internal/state"
)

// Version - текущая версия приложения
// Используется для логирования при старте и в API ответах
const (
	Version = "2.0.2"
)

// main - основная функция приложения
// Порядок инициализации критичен:
// 1. Загрузить конфигурацию (нужна для всех компонентов)
// 2. Инициализировать логирование (нужно для отладки всего остального)
// 3. Подключиться к базе данных
// 4. Инициализировать менеджер задач и сервер API
// 5. Обработать сигналы OS для корректного завершения
func main() {
	// Парсируем флаги командной строки
	// Использование: ctdaemon -c path/to/config.ini
	configFile := flag.String("c", "conf/config.ini", "Path to configuration file")
	flag.Parse()

	// 1. ЗАГРУЗКА КОНФИГУРАЦИИ
	// Конфигурация хранится в INI файле с параметрами для всех компонентов:
	// - database: параметры подключения к MySQL/PostgreSQL
	// - server: порт для REST API и другие настройки сервера
	// - log: уровень логирования, папка для логов
	// - trade: параметры торговли (интервал обновления и т.д.)
	cfg, err := config.Load(*configFile)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 2. ИНИЦИАЛИЗАЦИЯ ЛОГИРОВАНИЯ
	// Логирование - самый важный компонент для отладки проблем в production
	// Создает логи в папке (по умолчанию ./logs) и ротирует файлы по размеру
	// Поддерживает разные уровни: debug, info, warn, error
	if err := logger.Init(cfg.Log.Level, cfg.Log.Dir, cfg.Log.MaxFileSizeMB); err != nil {
		fmt.Printf("Failed to init logger: %+v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// Получаем логгер для main модуля
	// Все логи будут помечены префиксом "main" для удобства поиска
	log := logger.Get("main")
	log.Info("\n\n")
	log.Info("==========================================================")
	log.Info("INIT START ctdaemon", "version", Version)
	log.Info("Starting ctdaemon", "config", *configFile)

	// 3. ИНИЦИАЛИЗАЦИЯ БАЗЫ ДАННЫХ
	// Подключаемся к MySQL/PostgreSQL с параметрами из конфигурации
	// Поддерживает:
	// - Множественные попытки подключения (retry logic)
	// - TLS/SSL шифрование
	// - Таймауты подключения
	// При ошибке демон завершает работу (DB обязательна)
	if err := db.Init(cfg.Database); err != nil {
		log.Error("Failed to init database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// 4. ИНИЦИАЛИЗАЦИЯ МЕНЕДЖЕРА
	// Менеджер - это сердце приложения
	// Отвечает за:
	// - Загрузку задач мониторинга и торговли из БД
	// - Управление подписками на WebSocket потоки
	// - Запуск/остановку Monitor и Trader компонентов
	mgr := manager.New(cfg)

	// 5. ВОССТАНОВЛЕНИЕ СОСТОЯНИЯ
	// Если демон был завершен во время работы, восстанавливаем его состояние
	// Это обеспечивает непрерывность мониторинга/торговли при перезагрузке
	if state.GetInstance().IsRunning() {
		log.Info("Auto-starting daemon based on saved state")
		if err := mgr.Start(); err != nil {
			log.Error("Failed to auto-start daemon", "error", err)
		}
	}

	// 6. ИНИЦИАЛИЗАЦИЯ REST API СЕРВЕРА
	// Сервер предоставляет HTTP endpoints для управления демоном:
	// - /api/status - текущий статус
	// - /api/start - запустить мониторинг/торговлю
	// - /api/stop - остановить мониторинг/торговлю
	// - /api/config - получить текущую конфигурацию
	apiServer := api.New(cfg.Server, mgr, Version)
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Error("API server failed", "error", err)
			os.Exit(1)
		}
	}()

	// 7. ОБРАБОТКА СИГНАЛОВ ОС
	// Создаем канал для получения сигналов от операционной системы
	// Поддерживаем:
	// - SIGINT (Ctrl+C) - мягкое завершение
	// - SIGTERM (kill -15) - мягкое завершение
	// Это позволяет корректно выключить демон и сохранить состояние
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Ждем сигнала завершения
	sig := <-sigChan
	log.Info("Received signal, shutting down...", "signal", sig)

	// 8. КОРРЕКТНОЕ ЗАВЕРШЕНИЕ
	// Выполняем graceful shutdown - останавливаем все компоненты в правильном порядке
	// 1. Останавливаем менеджер (прекращает мониторинг/торговлю)
	// 2. Закрываем DB соединение (в defer уже)
	// 3. Закрываем логирование (в defer уже)
	// Это гарантирует, что все данные сохранены и соединения закрыты
	if err := mgr.Stop(); err != nil {
		log.Error("Error during shutdown", "error", err)
	}
	log.Info("Shutdown complete")

	// jsonData, _ := json.MarshalIndent(cfg, "", "  ")
	// fmt.Println(string(jsonData))
}
