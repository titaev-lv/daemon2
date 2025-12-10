package db

// DBDriver defines the interface for database drivers.
type DBDriver interface {
	Connect() error
	Close() error
	Ping() error
	// GetActiveTrades() ([]TradeCase, error)
	// GetActivePairsForDataMonitor() ([]DataMonitorPair, error)
	// GetExchangeByName(name string) (*Exchange, error)
	// GetExchanges() ([]Exchange, error)             // Получить все активные биржи
	// GetExchangesForTradeData() ([]Exchange, error) // Получить биржи для TradeData Monitor
	// GetActiveTradesForTrade() ([]Trade, error)     // Получить активные торги
	// Методы для работы с произвольными запросами
	// Query(query string, args ...interface{}) (*sql.Rows, error)
	// BeginTx() (*sql.Tx, error)
	// GetType() string
}
