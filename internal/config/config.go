// Package config отвечает за загрузку и парсинг конфигурации приложения
// Конфигурация хранится в INI формате для удобства чтения и редактирования
// Все параметры разбиты по секциям: database, server, log, trade, orderbook
package config

import (
	"fmt"

	"github.com/go-ini/ini"
)

// Config - главная структура конфигурации приложения
// Содержит все настройки для всех компонентов демона
type Config struct {
	// Database - параметры подключения к БД (MySQL/PostgreSQL)
	Database DatabaseConfig
	// Server - настройки REST API сервера
	Server ServerConfig
	// Log - параметры логирования
	Log LogConfig
	// Trade - параметры торговых операций
	Trade TradeConfig
	// OrderBook - параметры управления книгой ордеров
	OrderBook OrderBookConfig
	// Role - роль демона: "monitor" (сбор данных), "trader" (торговля) или "both" (оба)
	Role string
	// Monitor - конфигурация для мониторинга (используется если Role = "monitor" или "both")
	Monitor MonitorConfig
	// Trader - конфигурация для торговли (используется если Role = "trader" или "both")
	Trader TraderConfig
	// ClickHouse - параметры подключения к ClickHouse для исторических данных
	ClickHouse ClickHouseConfig
}

// OrderBookConfig - настройки для управления книгой ордеров
type OrderBookConfig struct {
	// DebugLogRaw - логировать ли сырые сообщения от бирж (много данных!)
	DebugLogRaw bool
	// DebugLogMsg - логировать ли обработанные сообщения (также много данных!)
	DebugLogMsg bool
}

// DatabaseConfig - параметры подключения к базе данных
// Поддерживает MySQL и PostgreSQL
type DatabaseConfig struct {
	// Type - тип базы данных ("mysql" или "postgres")
	Type string
	// User - имя пользователя для подключения
	User string
	// Password - пароль для подключения
	Password string
	// Host - адрес хоста БД (IP или имя хоста)
	Host string
	// Port - порт подключения (MySQL по умолчанию 3306, PostgreSQL 5432)
	Port int
	// Name - название базы данных
	Name string
	// UseTLS - использовать ли TLS/SSL для защищенного подключения
	UseTLS bool
	// CACert - путь к сертификату CA для проверки сертификата сервера
	CACert string
	// ClientCert - путь к сертификату клиента для клиентской аутентификации
	ClientCert string
	// ClientKey - путь к приватному ключу клиента
	ClientKey string
	// TLSSkipVerify - пропустить проверку сертификата (небезопасно, для IP адресов)
	TLSSkipVerify bool
	// ConnectTimeoutSec - таймаут подключения в секундах
	ConnectTimeoutSec int
	// MaxRetries - максимальное количество попыток подключения при ошибке
	MaxRetries int
}

// ServerConfig - конфигурация REST API сервера
type ServerConfig struct {
	// Port - порт на котором запускается HTTP сервер
	Port int
	// StateFile - путь к файлу для сохранения состояния демона
	StateFile string
}

// LogConfig - конфигурация системы логирования
type LogConfig struct {
	// Level - уровень логирования (debug, info, warn, error)
	Level string
	// Dir - папка куда писать логи
	Dir string
	// MaxFileSizeMB - максимальный размер одного лог файла в мегабайтах
	// При достижении размера файл ротируется с добавлением timestamp
	MaxFileSizeMB int
}

// TradeConfig - конфигурация торговых операций
type TradeConfig struct {
	// UpdateInterval - интервал обновления статуса торговых позиций в секундах
	UpdateInterval int
}

// MonitorConfig - конфигурация для режима Monitor
// Monitor собирает данные с бирж и сохраняет их в ClickHouse для анализа
type MonitorConfig struct {
	// OrderBookDepth - глубина книги ордеров которую мониторить
	// Возможные значения: 20, 50, 0 (full depth)
	// 20 = быстро но меньше данных
	// 50 = компромисс между скоростью и полнотой
	// 0 = полная книга ордеров (медленно, много данных)
	OrderBookDepth int

	// BatchSize - количество обновлений собираемых в batch перед отправкой в ClickHouse
	// Больший размер = меньше запросов к БД, но больше памяти
	// Рекомендуется 100-1000
	BatchSize int

	// BatchIntervalSec - максимальное время в секундах между отправками batch в ClickHouse
	// Даже если не собрали полный BatchSize, отправим через это время
	// Гарантирует что данные не залеживаются более чем на N секунд
	BatchIntervalSec int

	// RingBufferSize - размер ring buffer для хранения исторических данных в памяти
	// Ring buffer хранит последние N обновлений для быстрого доступа без запроса к БД
	// Рекомендуется 5000-50000 в зависимости от памяти
	RingBufferSize int

	// SaveInterval - интервал сохранения данных в ClickHouse в секундах
	// Как часто Monitor запускает batch send в БД
	SaveInterval int
}

// TraderConfig - конфигурация для режима Trader
// Trader выполняет торговые стратегии на основе данных мониторинга
type TraderConfig struct {
	// MaxOpenOrders - максимальное количество открытых ордеров одновременно
	// Предотвращает излишнее накопление ордеров при сбое стратегии
	MaxOpenOrders int

	// MaxPositionSize - максимальный размер позиции в USDT
	// Ограничивает риск одной позиции
	MaxPositionSize float64

	// DefaultStrategy - стратегия по умолчанию для новых пар
	// Возможные значения: "grid", "dca", "scalp" и т.д.
	DefaultStrategy string

	// StrategyUpdateInterval - интервал обновления стратегий в секундах
	// Как часто Trader переоценивает стратегию для каждой пары
	StrategyUpdateInterval int

	// SlippagePercent - допустимое проскальзывание в процентах при исполнении ордера
	// Если ордер исполнится хуже на больший процент - отменяется и переставляется
	SlippagePercent float64

	// EnableBacktest - включить ли режим бэктестирования (тестирование без реального исполнения)
	EnableBacktest bool
}

// ClickHouseConfig - конфигурация для подключения к ClickHouse
// ClickHouse используется для хранения больших объемов исторических данных
// В отличие от MySQL, ClickHouse оптимизирована для аналитики и огромных датасетов
type ClickHouseConfig struct {
	// Host - адрес хоста ClickHouse
	Host string

	// Port - порт ClickHouse HTTP API (обычно 8123)
	Port int

	// Database - название базы данных в ClickHouse
	Database string

	// Username - имя пользователя для подключения
	Username string

	// Password - пароль для подключения
	Password string

	// UseTLS - использовать ли HTTPS для подключения
	UseTLS bool

	// TLSSkipVerify - пропустить проверку сертификата (небезопасно)
	TLSSkipVerify bool

	// ConnectTimeoutSec - таймаут подключения в секундах
	ConnectTimeoutSec int

	// MaxRetries - максимальное количество попыток подключения
	MaxRetries int

	// Compression - включить ли сжатие данных при отправке
	// Значительно снижает трафик для больших объемов данных
	Compression bool

	// MaxBatchSize - максимальный размер batch для отправки данных
	// ClickHouse эффективнее работает с большими batch, но нужна память
	MaxBatchSize int

	// ReplicationFactor - фактор репликации данных в ClickHouse
	// 1 = без репликации (быстро но рискованно)
	// 2+ = с репликацией (надежно но медленнее)
	ReplicationFactor int
}

// Load - загружает конфигурацию из INI файла
// Парсит все секции и устанавливает значения по умолчанию если параметр не указан
// Возвращает ошибку если файл не найден или содержит невалидные данные
func Load(path string) (*Config, error) {
	// Загружаем INI файл
	cfg, err := ini.Load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	c := &Config{}

	// ========== DATABASE СЕКЦИЯ ==========
	// Парсим все параметры подключения к БД
	// MustString("default") вернет default если ключ не найден
	// MustInt(default) вернет default если ключ не найден или невалидный
	c.Database.Type = cfg.Section("database").Key("type").MustString("mysql")
	c.Database.User = cfg.Section("database").Key("user").String()
	c.Database.Password = cfg.Section("database").Key("password").String()
	c.Database.Host = cfg.Section("database").Key("host").String()
	c.Database.Port = cfg.Section("database").Key("port").MustInt(3306)
	c.Database.Name = cfg.Section("database").Key("name").String()
	c.Database.UseTLS = cfg.Section("database").Key("use_tls").MustBool(false)
	c.Database.CACert = cfg.Section("database").Key("ca_cert").String()
	c.Database.ClientCert = cfg.Section("database").Key("client_cert").String()
	c.Database.ClientKey = cfg.Section("database").Key("client_key").String()
	c.Database.TLSSkipVerify = cfg.Section("database").Key("tls_skip_verify").MustBool(false)
	c.Database.ConnectTimeoutSec = cfg.Section("database").Key("connect_timeout_sec").MustInt(10)
	// MaxRetries - КРИТИЧНЫЙ параметр! Без него демон падает при первой ошибке подключения
	c.Database.MaxRetries = cfg.Section("database").Key("max_retries").MustInt(0)

	// ========== SERVER СЕКЦИЯ ==========
	// REST API сервер слушает на этом порту
	c.Server.Port = cfg.Section("server").Key("port").MustInt(8080)
	c.Server.StateFile = cfg.Section("server").Key("state_file").MustString("state.json")

	// ========== LOG СЕКЦИЯ ==========
	// Параметры логирования влияют на объем и детальность логов
	c.Log.Level = cfg.Section("log").Key("level").MustString("info")
	c.Log.Dir = cfg.Section("log").Key("dir").MustString("./logs")
	c.Log.MaxFileSizeMB = cfg.Section("log").Key("max_file_size_mb").MustInt(10)

	// ========== TRADE СЕКЦИЯ ==========
	// Параметры торговли
	c.Trade.UpdateInterval = cfg.Section("trade").Key("update_interval").MustInt(5)

	// ========== ORDERBOOK СЕКЦИЯ ==========
	// Параметры отладки для отслеживания изменений в книге ордеров
	// ВНИМАНИЕ: Включение этих флагов создает ОГРОМНОЕ количество логов!
	c.OrderBook.DebugLogRaw = cfg.Section("orderbook").Key("debug_log_raw").MustBool(false)
	c.OrderBook.DebugLogMsg = cfg.Section("orderbook").Key("debug_log_msg").MustBool(false)

	// ========== ROLE СЕКЦИЯ ==========
	// Определяет основную роль демона
	// monitor = только сбор данных в ClickHouse
	// trader = только торговля на основе заранее загруженных данных
	// both = и мониторинг и торговля одновременно
	c.Role = cfg.Section("role").Key("mode").MustString("monitor")

	// ========== MONITOR СЕКЦИЯ ==========
	// Параметры мониторинга (для Monitor компонента)
	c.Monitor.OrderBookDepth = cfg.Section("monitor").Key("orderbook_depth").MustInt(20)
	c.Monitor.BatchSize = cfg.Section("monitor").Key("batch_size").MustInt(500)
	c.Monitor.BatchIntervalSec = cfg.Section("monitor").Key("batch_interval_sec").MustInt(5)
	c.Monitor.RingBufferSize = cfg.Section("monitor").Key("ring_buffer_size").MustInt(10000)
	c.Monitor.SaveInterval = cfg.Section("monitor").Key("save_interval").MustInt(5)

	// ========== TRADER СЕКЦИЯ ==========
	// Параметры торговли (для Trader компонента)
	c.Trader.MaxOpenOrders = cfg.Section("trader").Key("max_open_orders").MustInt(10)
	c.Trader.MaxPositionSize = cfg.Section("trader").Key("max_position_size").MustFloat64(1000.0)
	c.Trader.DefaultStrategy = cfg.Section("trader").Key("default_strategy").MustString("grid")
	c.Trader.StrategyUpdateInterval = cfg.Section("trader").Key("strategy_update_interval").MustInt(10)
	c.Trader.SlippagePercent = cfg.Section("trader").Key("slippage_percent").MustFloat64(0.5)
	c.Trader.EnableBacktest = cfg.Section("trader").Key("enable_backtest").MustBool(false)

	// ========== CLICKHOUSE СЕКЦИЯ ==========
	// Параметры подключения к ClickHouse для исторических данных
	c.ClickHouse.Host = cfg.Section("clickhouse").Key("host").MustString("localhost")
	c.ClickHouse.Port = cfg.Section("clickhouse").Key("port").MustInt(8123)
	c.ClickHouse.Database = cfg.Section("clickhouse").Key("database").MustString("crypto")
	c.ClickHouse.Username = cfg.Section("clickhouse").Key("username").String()
	c.ClickHouse.Password = cfg.Section("clickhouse").Key("password").String()
	c.ClickHouse.UseTLS = cfg.Section("clickhouse").Key("use_tls").MustBool(false)
	c.ClickHouse.TLSSkipVerify = cfg.Section("clickhouse").Key("tls_skip_verify").MustBool(false)
	c.ClickHouse.ConnectTimeoutSec = cfg.Section("clickhouse").Key("connect_timeout_sec").MustInt(10)
	c.ClickHouse.MaxRetries = cfg.Section("clickhouse").Key("max_retries").MustInt(3)
	c.ClickHouse.Compression = cfg.Section("clickhouse").Key("compression").MustBool(true)
	c.ClickHouse.MaxBatchSize = cfg.Section("clickhouse").Key("max_batch_size").MustInt(10000)
	c.ClickHouse.ReplicationFactor = cfg.Section("clickhouse").Key("replication_factor").MustInt(1)

	return c, nil
}
