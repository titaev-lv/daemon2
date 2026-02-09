package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"ctdaemon/internal/core/exchange"
)

// Fetcher периодически загружает задачи мониторинга и торговли из MySQL
type Fetcher struct {
	db       *sql.DB
	interval time.Duration

	lastMonitoring []*exchange.MonitoringTask
	lastTrading    []*exchange.TradingTask

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu sync.RWMutex
}

// TasksData объединяет все задачи при загрузке из БД
type TasksData struct {
	Timestamp       int64
	MonitoringTasks []*exchange.MonitoringTask
	TradingTasks    []*exchange.TradingTask
}

// NewFetcher создает новый Fetcher
func NewFetcher(db *sql.DB, interval time.Duration) *Fetcher {
	return &Fetcher{
		db:             db,
		interval:       interval,
		lastMonitoring: make([]*exchange.MonitoringTask, 0),
		lastTrading:    make([]*exchange.TradingTask, 0),
	}
}

// Start запускает фоновый горутин для периодической загрузки задач
func (f *Fetcher) Start(ctx context.Context) error {
	f.ctx, f.cancel = context.WithCancel(ctx)

	// Сначала загружаем один раз при старте
	if err := f.fetchTasks(); err != nil {
		return fmt.Errorf("initial fetch failed: %w", err)
	}

	f.wg.Add(1)
	go f.fetchLoop()

	return nil
}

// Stop останавливает фоновый горутин
func (f *Fetcher) Stop() error {
	f.cancel()
	f.wg.Wait()
	return nil
}

// GetLast возвращает последние загруженные данные
func (f *Fetcher) GetLast() *TasksData {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return &TasksData{
		Timestamp:       time.Now().Unix(),
		MonitoringTasks: copyMonitoringTasks(f.lastMonitoring),
		TradingTasks:    copyTradingTasks(f.lastTrading),
	}
}

// fetchLoop периодически вызывает fetchTasks
func (f *Fetcher) fetchLoop() {
	defer f.wg.Done()

	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-ticker.C:
			// Ошибки логируем, но не прерываем цикл
			if err := f.fetchTasks(); err != nil {
				// TODO: логирование
				fmt.Printf("fetch error: %v\n", err)
			}
		}
	}
}

// fetchTasks загружает задачи из MySQL
func (f *Fetcher) fetchTasks() error {
	monitoring, err := f.fetchMonitoringTasks()
	if err != nil {
		return fmt.Errorf("fetch monitoring tasks failed: %w", err)
	}

	trading, err := f.fetchTradingTasks()
	if err != nil {
		return fmt.Errorf("fetch trading tasks failed: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.lastMonitoring = monitoring
	f.lastTrading = trading

	return nil
}

// fetchMonitoringTasks загружает конфигурации мониторинга из MONITORING таблицы
func (f *Fetcher) fetchMonitoringTasks() ([]*exchange.MonitoringTask, error) {
	query := `
		SELECT 
			m.ID,
			m.UID,
			m.ACTIVE,
			e.EXCHANGE_ID,
			e.NAME as EXCHANGE_NAME,
			mtp.PAIR_ID,
			tp.BASE_CURRENCY_ID,
			tp.QUOTE_CURRENCY_ID,
			tp.MARKET_TYPE,
			c1.SYMBOL as BASE_SYMBOL,
			c2.SYMBOL as QUOTE_SYMBOL,
			m.ORDERBOOK_DEPTH,
			m.BATCH_SIZE,
			m.BATCH_INTERVAL_SEC,
			m.RING_BUFFER_SIZE,
			m.SAVE_INTERVAL_SEC
		FROM MONITORING m
		JOIN MONITORING_TRADE_PAIRS mtp ON m.ID = mtp.MONITORING_ID
		JOIN TRADE_PAIR tp ON mtp.PAIR_ID = tp.ID
		JOIN EXCHANGE e ON tp.EXCHANGE_ID = e.ID
		JOIN COIN c1 ON tp.BASE_CURRENCY_ID = c1.ID
		JOIN COIN c2 ON tp.QUOTE_CURRENCY_ID = c2.ID
		WHERE m.ACTIVE = 1
		ORDER BY m.ID
	`

	rows, err := f.db.QueryContext(f.ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*exchange.MonitoringTask

	for rows.Next() {
		var (
			id               int
			uid              int
			active           bool
			exchangeID       string
			exchangeName     string
			pairID           int
			baseCurrencyID   int
			quoteCurrencyID  int
			marketType       string
			baseSymbol       string
			quoteSymbol      string
			orderbookDepth   int
			batchSize        int
			batchIntervalSec int
			ringBufferSize   int
			saveIntervalSec  int
		)

		if err := rows.Scan(
			&id, &uid, &active, &exchangeID, &exchangeName, &pairID,
			&baseCurrencyID, &quoteCurrencyID, &marketType,
			&baseSymbol, &quoteSymbol,
			&orderbookDepth, &batchSize, &batchIntervalSec,
			&ringBufferSize, &saveIntervalSec,
		); err != nil {
			return nil, fmt.Errorf("scan monitoring task failed: %w", err)
		}

		pair := fmt.Sprintf("%s/%s", baseSymbol, quoteSymbol)

		task := &exchange.MonitoringTask{
			ID:               id,
			UID:              uid,
			ExchangeID:       exchangeID,
			ExchangeName:     exchangeName,
			MarketType:       marketType,
			TradePairID:      pairID,
			TradePair:        pair,
			OrderbookDepth:   orderbookDepth,
			BatchSize:        batchSize,
			BatchIntervalSec: batchIntervalSec,
			RingBufferSize:   ringBufferSize,
			SaveIntervalSec:  saveIntervalSec,
		}

		tasks = append(tasks, task)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

// fetchTradingTasks загружает конфигурации торговли из TRADE таблицы
func (f *Fetcher) fetchTradingTasks() ([]*exchange.TradingTask, error) {
	query := `
		SELECT 
			t.ID,
			t.UID,
			t.TYPE,
			t.ACTIVE,
			e.EXCHANGE_ID,
			e.NAME as EXCHANGE_NAME,
			tp2.PAIR_ID,
			tr.BASE_CURRENCY_ID,
			tr.QUOTE_CURRENCY_ID,
			tr.MARKET_TYPE,
			c1.SYMBOL as BASE_SYMBOL,
			c2.SYMBOL as QUOTE_SYMBOL,
			tt.NAME as STRATEGY_ID,
			t.MAX_AMOUNT_TRADE,
			t.MAX_OPEN_ORDERS,
			t.MAX_POSITION_SIZE,
			t.STRATEGY_UPDATE_INTERVAL_SEC,
			t.SLIPPAGE_PERCENT,
			t.ENABLE_BACKTEST,
			t.FIN_PROTECTION,
			t.BBO_ONLY,
			ea.ID as EXCHANGE_ACCOUNT_ID
		FROM TRADE t
		JOIN TRADE_PAIRS tp2 ON t.ID = tp2.TRADE_ID
		JOIN TRADE_PAIR tr ON tp2.PAIR_ID = tr.ID
		JOIN EXCHANGE e ON tr.EXCHANGE_ID = e.ID
		JOIN EXCHANGE_ACCOUNTS ea ON tp2.EAID = ea.ID
		JOIN COIN c1 ON tr.BASE_CURRENCY_ID = c1.ID
		JOIN COIN c2 ON tr.QUOTE_CURRENCY_ID = c2.ID
		JOIN TRADE_TYPE tt ON t.TYPE = tt.ID
		WHERE t.ACTIVE = 1
		ORDER BY t.ID
	`

	rows, err := f.db.QueryContext(f.ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*exchange.TradingTask

	for rows.Next() {
		var (
			id                     int
			uid                    int
			tradeType              int
			active                 bool
			exchangeID             string
			exchangeName           string
			pairID                 int
			baseCurrencyID         int
			quoteCurrencyID        int
			marketType             string
			baseSymbol             string
			quoteSymbol            string
			strategyID             string
			maxAmountTrade         float64
			maxOpenOrders          int
			maxPositionSize        float64
			strategyUpdateInterval int
			slippagePercent        float64
			enableBacktest         bool
			finProtection          bool
			bboOnly                bool
			exchangeAccountID      int
		)

		if err := rows.Scan(
			&id, &uid, &tradeType, &active,
			&exchangeID, &exchangeName, &pairID,
			&baseCurrencyID, &quoteCurrencyID, &marketType,
			&baseSymbol, &quoteSymbol, &strategyID,
			&maxAmountTrade, &maxOpenOrders, &maxPositionSize,
			&strategyUpdateInterval, &slippagePercent,
			&enableBacktest, &finProtection, &bboOnly,
			&exchangeAccountID,
		); err != nil {
			return nil, fmt.Errorf("scan trading task failed: %w", err)
		}

		pair := fmt.Sprintf("%s/%s", baseSymbol, quoteSymbol)

		// Упаковываем параметры в JSON
		params := map[string]interface{}{
			"max_amount_trade":             maxAmountTrade,
			"max_open_orders":              maxOpenOrders,
			"max_position_size":            maxPositionSize,
			"strategy_update_interval_sec": strategyUpdateInterval,
			"slippage_percent":             slippagePercent,
			"enable_backtest":              enableBacktest,
			"fin_protection":               finProtection,
			"bbo_only":                     bboOnly,
		}

		paramsJSON, _ := json.Marshal(params)

		task := &exchange.TradingTask{
			ID:                id,
			UID:               uid,
			TradeType:         tradeType,
			ExchangeID:        exchangeID,
			ExchangeName:      exchangeName,
			MarketType:        marketType,
			TradePairID:       pairID,
			TradePair:         pair,
			StrategyID:        strategyID,
			StrategyParams:    string(paramsJSON),
			ExchangeAccountID: exchangeAccountID,
		}

		tasks = append(tasks, task)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

// Вспомогательные функции для копирования (избегаем race conditions)

func copyMonitoringTasks(tasks []*exchange.MonitoringTask) []*exchange.MonitoringTask {
	result := make([]*exchange.MonitoringTask, len(tasks))
	for i, t := range tasks {
		taskCopy := *t
		result[i] = &taskCopy
	}
	return result
}

func copyTradingTasks(tasks []*exchange.TradingTask) []*exchange.TradingTask {
	result := make([]*exchange.TradingTask, len(tasks))
	for i, t := range tasks {
		taskCopy := *t
		result[i] = &taskCopy
	}
	return result
}
