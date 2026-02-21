// Package config отвечает за загрузку и парсинг конфигурации приложения.
// Конфигурация хранится в YAML формате.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config - главная структура конфигурации приложения
// Содержит все настройки для всех компонентов демона
type Config struct {
	// Logging - параметры логирования
	Logging LogConfig `yaml:"logging"`
	// Trade - параметры торговых операций
	Trade TradeConfig `yaml:"trade"`
	// OrderBook - параметры управления книгой ордеров
	OrderBook OrderBookConfig `yaml:"orderbook"`
	// Role - роль демона: "monitor" (сбор данных), "trader" (торговля) или "both" (оба)
	Role string `yaml:"role"`
	// Monitor - конфигурация для мониторинга (используется если Role = "monitor" или "both")
	Monitor MonitorConfig `yaml:"monitor"`
	// Trader - конфигурация для торговли (используется если Role = "trader" или "both")
	Trader TraderConfig `yaml:"trader"`
	// ClickHouse - параметры подключения к ClickHouse для исторических данных
	ClickHouse ClickHouseConfig `yaml:"clickhouse"`
}

// OrderBookConfig - настройки для управления книгой ордеров
type OrderBookConfig struct {
	// DebugLogRaw - логировать ли сырые сообщения от бирж (много данных!)
	DebugLogRaw bool `yaml:"debug_log_raw"`
	// DebugLogMsg - логировать ли обработанные сообщения (также много данных!)
	DebugLogMsg bool `yaml:"debug_log_msg"`
}

// LogConfig - конфигурация системы логирования
type LogConfig struct {
	// Level - уровень логирования (debug, info, warn, error)
	Level string `yaml:"level"`
	// Dir - папка куда писать логи
	Dir string `yaml:"dir"`
	// MaxFileSizeMB - максимальный размер одного лог файла в мегабайтах
	// При достижении размера файл ротируется с добавлением timestamp
	MaxFileSizeMB int `yaml:"max_size_mb"`
	// MaxBackups - сколько файлов хранить после ротации
	MaxBackups int `yaml:"max_backups"`
	// MaxAgeDays - сколько дней хранить rotated логи
	MaxAgeDays int `yaml:"max_age_days"`
	// Compress - сжимать rotated логи
	Compress bool `yaml:"compress"`
}

// TradeConfig - конфигурация торговых операций
type TradeConfig struct {
	// UpdateInterval - интервал обновления статуса торговых позиций в секундах
	UpdateInterval int `yaml:"update_interval"`
}

// MonitorConfig - конфигурация для режима Monitor
// Monitor собирает данные с бирж и сохраняет их в ClickHouse для анализа
type MonitorConfig struct {
	// OrderBookDepth - глубина книги ордеров которую мониторить
	// Возможные значения: 20, 50, 0 (full depth)
	// 20 = быстро но меньше данных
	// 50 = компромисс между скоростью и полнотой
	// 0 = полная книга ордеров (медленно, много данных)
	OrderBookDepth int `yaml:"orderbook_depth"`

	// BatchSize - количество обновлений собираемых в batch перед отправкой в ClickHouse
	// Больший размер = меньше запросов к БД, но больше памяти
	// Рекомендуется 100-1000
	BatchSize int `yaml:"batch_size"`

	// BatchInterval - максимальное время в секундах между отправками batch в ClickHouse
	// Даже если не собрали полный BatchSize, отправим через это время
	// Гарантирует что данные не залеживаются более чем на N секунд
	BatchInterval int `yaml:"batch_interval"`

	// RingBufferSize - размер ring buffer для хранения исторических данных в памяти
	// Ring buffer хранит последние N обновлений для быстрого доступа без запроса к БД
	// Рекомендуется 5000-50000 в зависимости от памяти
	RingBufferSize int `yaml:"ring_buffer_size"`

	// SaveInterval - интервал сохранения данных в ClickHouse в секундах
	// Как часто Monitor запускает batch send в БД
	SaveInterval int `yaml:"save_interval"`
}

// TraderConfig - конфигурация для режима Trader
// Trader выполняет торговые стратегии на основе данных мониторинга
type TraderConfig struct {
	// MaxOpenOrders - максимальное количество открытых ордеров одновременно
	// Предотвращает излишнее накопление ордеров при сбое стратегии
	MaxOpenOrders int `yaml:"max_open_orders"`

	// MaxPositionSize - максимальный размер позиции в USDT
	// Ограничивает риск одной позиции
	MaxPositionSize float64 `yaml:"max_position_size"`

	// DefaultStrategy - стратегия по умолчанию для новых пар
	// Возможные значения: "grid", "dca", "scalp" и т.д.
	DefaultStrategy string `yaml:"default_strategy"`

	// StrategyUpdateInterval - интервал обновления стратегий в секундах
	// Как часто Trader переоценивает стратегию для каждой пары
	StrategyUpdateInterval int `yaml:"strategy_update_interval"`

	// SlippagePercent - допустимое проскальзывание в процентах при исполнении ордера
	// Если ордер исполнится хуже на больший процент - отменяется и переставляется
	SlippagePercent float64 `yaml:"slippage_percent"`

	// EnableBacktest - включить ли режим бэктестирования (тестирование без реального исполнения)
	EnableBacktest bool `yaml:"enable_backtest"`
}

// ClickHouseConfig - конфигурация для подключения к ClickHouse
// ClickHouse используется для хранения больших объемов исторических данных
// В отличие от MySQL, ClickHouse оптимизирована для аналитики и огромных датасетов
type ClickHouseConfig struct {
	// Host - адрес хоста ClickHouse
	Host string `yaml:"host"`

	// Port - порт ClickHouse HTTP API (обычно 8123)
	Port int `yaml:"port"`

	// Database - название базы данных в ClickHouse
	Database string `yaml:"database"`

	// Username - имя пользователя для подключения
	Username string `yaml:"username"`

	// Password - пароль для подключения
	Password string `yaml:"password"`

	// UseTLS - использовать ли HTTPS для подключения
	UseTLS bool `yaml:"use_tls"`

	// TLSSkipVerify - пропустить проверку сертификата (небезопасно)
	TLSSkipVerify bool `yaml:"tls_skip_verify"`

	// ConnectTimeout - таймаут подключения в секундах
	ConnectTimeout int `yaml:"connect_timeout"`

	// MaxRetries - максимальное количество попыток подключения
	MaxRetries int `yaml:"max_retries"`

	// Compression - включить ли сжатие данных при отправке
	// Значительно снижает трафик для больших объемов данных
	Compression bool `yaml:"compression"`

	// MaxBatchSize - максимальный размер batch для отправки данных
	// ClickHouse эффективнее работает с большими batch, но нужна память
	MaxBatchSize int `yaml:"max_batch_size"`

	// ReplicationFactor - фактор репликации данных в ClickHouse
	// 1 = без репликации (быстро но рискованно)
	// 2+ = с репликацией (надежно но медленнее)
	ReplicationFactor int `yaml:"replication_factor"`
}

// Load загружает конфигурацию из YAML файла.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	c := defaultConfig()
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config: %w", err)
	}
	applyDefaults(c)
	applyEnvOverrides(c)

	return c, nil
}

func defaultConfig() *Config {
	return &Config{
		Logging: LogConfig{
			Level:         "info",
			Dir:           "./logs",
			MaxFileSizeMB: 10,
			MaxBackups:    10,
			MaxAgeDays:    30,
			Compress:      false,
		},
		Trade: TradeConfig{UpdateInterval: 5},
		OrderBook: OrderBookConfig{
			DebugLogRaw: false,
			DebugLogMsg: false,
		},
		Role: "monitor",
		Monitor: MonitorConfig{
			OrderBookDepth: 20,
			BatchSize:      500,
			BatchInterval:  5,
			RingBufferSize: 10000,
			SaveInterval:   5,
		},
		Trader: TraderConfig{
			MaxOpenOrders:          10,
			MaxPositionSize:        1000.0,
			DefaultStrategy:        "grid",
			StrategyUpdateInterval: 10,
			SlippagePercent:        0.5,
			EnableBacktest:         false,
		},
		ClickHouse: ClickHouseConfig{
			Host:              "localhost",
			Port:              8123,
			Database:          "crypto",
			UseTLS:            false,
			TLSSkipVerify:     false,
			ConnectTimeout:    10,
			MaxRetries:        3,
			Compression:       true,
			MaxBatchSize:      10000,
			ReplicationFactor: 1,
		},
	}
}

func applyDefaults(c *Config) {
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Dir == "" {
		c.Logging.Dir = "./logs"
	}
	if c.Logging.MaxFileSizeMB == 0 {
		c.Logging.MaxFileSizeMB = 10
	}
	if c.Logging.MaxBackups == 0 {
		c.Logging.MaxBackups = 10
	}
	if c.Logging.MaxAgeDays == 0 {
		c.Logging.MaxAgeDays = 30
	}

	if c.Trade.UpdateInterval == 0 {
		c.Trade.UpdateInterval = 5
	}

	if c.Role == "" {
		c.Role = "monitor"
	}

	if c.Monitor.OrderBookDepth == 0 {
		c.Monitor.OrderBookDepth = 20
	}
	if c.Monitor.BatchSize == 0 {
		c.Monitor.BatchSize = 500
	}
	if c.Monitor.BatchInterval == 0 {
		c.Monitor.BatchInterval = 5
	}
	if c.Monitor.RingBufferSize == 0 {
		c.Monitor.RingBufferSize = 10000
	}
	if c.Monitor.SaveInterval == 0 {
		c.Monitor.SaveInterval = 5
	}

	if c.Trader.MaxOpenOrders == 0 {
		c.Trader.MaxOpenOrders = 10
	}
	if c.Trader.MaxPositionSize == 0 {
		c.Trader.MaxPositionSize = 1000.0
	}
	if c.Trader.DefaultStrategy == "" {
		c.Trader.DefaultStrategy = "grid"
	}
	if c.Trader.StrategyUpdateInterval == 0 {
		c.Trader.StrategyUpdateInterval = 10
	}
	if c.Trader.SlippagePercent == 0 {
		c.Trader.SlippagePercent = 0.5
	}

	if c.ClickHouse.Host == "" {
		c.ClickHouse.Host = "localhost"
	}
	if c.ClickHouse.Port == 0 {
		c.ClickHouse.Port = 8123
	}
	if c.ClickHouse.Database == "" {
		c.ClickHouse.Database = "crypto"
	}
	if c.ClickHouse.ConnectTimeout == 0 {
		c.ClickHouse.ConnectTimeout = 10
	}
	if c.ClickHouse.MaxRetries == 0 {
		c.ClickHouse.MaxRetries = 3
	}
	if c.ClickHouse.MaxBatchSize == 0 {
		c.ClickHouse.MaxBatchSize = 10000
	}
	if c.ClickHouse.ReplicationFactor == 0 {
		c.ClickHouse.ReplicationFactor = 1
	}
}

func applyEnvOverrides(c *Config) {
	c.Role = envString("TRADER_ROLE", c.Role)

	c.Logging.Level = envString("TRADER_LOG_LEVEL", c.Logging.Level)
	c.Logging.Dir = envString("TRADER_LOG_DIR", c.Logging.Dir)
	c.Logging.MaxFileSizeMB = envInt("TRADER_LOG_MAX_SIZE_MB", c.Logging.MaxFileSizeMB)
	c.Logging.MaxBackups = envInt("TRADER_LOG_MAX_BACKUPS", c.Logging.MaxBackups)
	c.Logging.MaxAgeDays = envInt("TRADER_LOG_MAX_AGE_DAYS", c.Logging.MaxAgeDays)
	c.Logging.Compress = envBool("TRADER_LOG_COMPRESS", c.Logging.Compress)

	c.OrderBook.DebugLogRaw = envBool("TRADER_ORDERBOOK_DEBUG_LOG_RAW", c.OrderBook.DebugLogRaw)
	c.OrderBook.DebugLogMsg = envBool("TRADER_ORDERBOOK_DEBUG_LOG_MSG", c.OrderBook.DebugLogMsg)

	c.ClickHouse.Host = envString("TRADER_CLICKHOUSE_HOST", c.ClickHouse.Host)
	c.ClickHouse.Port = envInt("TRADER_CLICKHOUSE_PORT", c.ClickHouse.Port)
	c.ClickHouse.Database = envString("TRADER_CLICKHOUSE_DATABASE", c.ClickHouse.Database)
	c.ClickHouse.Username = envString("TRADER_CLICKHOUSE_USERNAME", c.ClickHouse.Username)
	c.ClickHouse.Password = envString("TRADER_CLICKHOUSE_PASSWORD", c.ClickHouse.Password)
	c.ClickHouse.UseTLS = envBool("TRADER_CLICKHOUSE_USE_TLS", c.ClickHouse.UseTLS)
	c.ClickHouse.TLSSkipVerify = envBool("TRADER_CLICKHOUSE_TLS_SKIP_VERIFY", c.ClickHouse.TLSSkipVerify)
	c.ClickHouse.ConnectTimeout = envInt("TRADER_CLICKHOUSE_CONNECT_TIMEOUT", c.ClickHouse.ConnectTimeout)
	c.ClickHouse.MaxRetries = envInt("TRADER_CLICKHOUSE_MAX_RETRIES", c.ClickHouse.MaxRetries)
}

func envString(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}
