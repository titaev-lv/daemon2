// Package db отвечает за подключение к базам данных
// Поддерживает MySQL и PostgreSQL с TLS шифрованием
// Реализует retry logic с экспоненциальной задержкой для обработки временных сбоев
package db

import (
	"ctdaemon/internal/config"
	"ctdaemon/internal/logger"
	"fmt"
	"math"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL драйвер (импортируем пустую строку для инициализации)
	_ "github.com/lib/pq"              // PostgreSQL драйвер
)

// driver - глобальная переменная для хранения текущего БД драйвера
// Заполняется в ConnectWithRetry, используется в Close
var driver DBDriver

// ConnectWithRetry - подключается к БД с retry logic и экспоненциальной задержкой
// Если первые 10 попыток неудачны, начинает использовать экспоненциальную задержку
// Это обеспечивает быстрое восстановление при временных сбоях сети
func ConnectWithRetry(cfg config.DatabaseConfig) error {
	log := logger.Get("db")
	// Устанавливаем MaxRetries, по умолчанию минимум 1 попытка
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	var lastErr error

	// Пытаемся подключиться несколько раз
	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Info("Database connection attempt", "attempt", attempt, "max_retries", maxRetries)

		// Создаем драйвер в зависимости от типа БД
		var d DBDriver

		if cfg.Type == "postgres" || cfg.Type == "postgresql" {
			// PostgreSQL драйвер
			d = &PostgresDriver{
				Host:           cfg.Host,
				Port:           cfg.Port,
				User:           cfg.User,
				Pass:           cfg.Password,
				Database:       cfg.Name,
				UseTLS:         cfg.UseTLS,
				CACert:         cfg.CACert,
				ClientCert:     cfg.ClientCert,
				ClientKey:      cfg.ClientKey,
				TLSSkipVerify:  cfg.TLSSkipVerify,
				ConnectTimeout: time.Duration(cfg.ConnectTimeoutSec) * time.Second,
			}
		} else {
			// MySQL драйвер (по умолчанию)
			d = &MySQLDriver{
				Host:           cfg.Host,
				Port:           cfg.Port,
				User:           cfg.User,
				Pass:           cfg.Password,
				Database:       cfg.Name,
				UseTLS:         cfg.UseTLS,
				CACert:         cfg.CACert,
				ClientCert:     cfg.ClientCert,
				ClientKey:      cfg.ClientKey,
				TLSSkipVerify:  cfg.TLSSkipVerify,
				ConnectTimeout: time.Duration(cfg.ConnectTimeoutSec) * time.Second,
			}
		}

		// Пытаемся подключиться
		if err := d.Connect(); err != nil {
			lastErr = err
			log.Warn("Database connection failed", "attempt", attempt, "error", err)

			// Если это не последняя попытка - ждем перед повтором
			if attempt < maxRetries {
				backoffInterval := calculateBackoffInterval(attempt)
				log.Info("Waiting before retry", "attempt", attempt, "backoff_seconds", backoffInterval)
				time.Sleep(time.Duration(backoffInterval) * time.Second)
			}
			continue
		}

		// Успешно подключились! Сохраняем драйвер
		driver = d
		log.Info("Database connected successfully", "attempt", attempt, "type", cfg.Type)
		return nil
	}

	// Все попытки исчерпаны
	return fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, lastErr)
}

// calculateBackoffInterval - рассчитывает задержку между попытками подключения
// Первые 10 попыток: без задержки (для быстрого восстановления от временных сбоев)
// После 10 попыток: экспоненциальная задержка 1s, 2s, 4s, 8s... с максимумом 300s
// Используется для избежания перегрузки БД при длительном сбое
func calculateBackoffInterval(attempt int) int {
	// Первые 10 попыток без задержки (быстрое восстановление)
	if attempt <= 10 {
		return 0
	}

	// После 10 попыток: экспоненциальная задержка
	// Попытка 11 → 1s, попытка 12 → 2s, попытка 13 → 4s и т.д.
	backoffMultiplier := attempt - 10
	interval := int(math.Pow(2, float64(backoffMultiplier-1)))

	// Ограничиваем максимум 300 секунд (5 минут)
	// Предотвращаем бесконечный рост задержки
	if interval > 300 {
		interval = 300
	}

	return interval
}

// Init - инициализирует БД подключение
// Проверяет TLS конфигурацию если включено и запускает retry подключение
func Init(cfg config.DatabaseConfig) error {
	// Если используется TLS, проверяем что все файлы есть и читаемы
	if cfg.UseTLS {
		if err := validateTLSConfig(cfg); err != nil {
			return err
		}
	}

	return ConnectWithRetry(cfg)
}

// validateTLSConfig - проверяет что все TLS сертификаты существуют и читаемы
// Вызывается если UseTLS=true перед попыткой подключения
// Гарантирует что попытка подключения не упадет из-за отсутствующего файла
func validateTLSConfig(cfg config.DatabaseConfig) error {
	// Проверяем наличие всех нужных файлов
	files := map[string]string{
		"ca_cert":     cfg.CACert,     // Сертификат центра сертификации
		"client_cert": cfg.ClientCert, // Сертификат клиента
		"client_key":  cfg.ClientKey,  // Приватный ключ клиента
	}

	for name, path := range files {
		// Проверяем что путь указан
		if path == "" {
			return fmt.Errorf("TLS is enabled but %s is not specified", name)
		}

		// Проверяем что файл существует
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("%s file not found: %s", name, path)
			}
			return fmt.Errorf("cannot access %s file at %s: %w", name, path, err)
		}

		// Проверяем что можем прочитать файл (права доступа корректны)
		if _, err := os.ReadFile(path); err != nil {
			return fmt.Errorf("cannot read %s file at %s: %w", name, path, err)
		}
	}

	return nil
}

// Close - закрывает соединение с БД
// Вызывается при завершении приложения (в defer из main.go)
func Close() {
	// Если драйвер инициализирован - закрываем его соединение
	if driver != nil {
		driver.Close()
	}
}
