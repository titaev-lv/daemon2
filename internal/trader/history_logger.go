package trader

import (
"database/sql"
"fmt"
"sync"
"time"
)

// TradeHistoryLogger логирует ордера в TRADE_HISTORY
type TradeHistoryLogger struct {
db        *sql.DB
buffer    []*OrderExecution
bufferMu  sync.Mutex
maxBuffer int
}

// OrderExecution описывает исполненный ордер
type OrderExecution struct {
TradeID           int
OrderID           string
ExchangeID        string
TradePairID       int
TradePair         string
ExchangeAccountID int
Side              string
Price             float64
Amount            float64
Commission        float64
CommissionAsset   string
Status            string
ExecutedAtMicros  int64
ProfitLoss        *float64
}

// NewTradeHistoryLogger создает новый логгер
func NewTradeHistoryLogger(db *sql.DB, maxBuffer int) *TradeHistoryLogger {
return &TradeHistoryLogger{
db:        db,
buffer:    make([]*OrderExecution, 0, maxBuffer),
maxBuffer: maxBuffer,
}
}

// LogOrderExecution логирует ордер
func (l *TradeHistoryLogger) LogOrderExecution(order *OrderExecution) error {
if order == nil {
return fmt.Errorf("order cannot be nil")
}

l.bufferMu.Lock()
defer l.bufferMu.Unlock()

l.buffer = append(l.buffer, order)

if len(l.buffer) >= l.maxBuffer {
return l.flushUnsafe()
}

return nil
}

// Flush сохраняет буфер в БД
func (l *TradeHistoryLogger) Flush() error {
l.bufferMu.Lock()
defer l.bufferMu.Unlock()
return l.flushUnsafe()
}

// flushUnsafe сохраняет (под блокировкой)
func (l *TradeHistoryLogger) flushUnsafe() error {
if len(l.buffer) == 0 {
return nil
}

query := `INSERT INTO TRADE_HISTORY 
(TRADE_ID, ORDER_ID, PAIR_ID, EAID, SIDE, PRICE, AMOUNT, 
 COMMISSION, COMMISSION_ASSET, STATUS, EXECUTED_AT, PROFIT_LOSS, DATE_CREATE)
VALUES`

var values []interface{}
var valueStrings []string

for _, order := range l.buffer {
executedAtSec := order.ExecutedAtMicros / 1000000
executedAtMicros := order.ExecutedAtMicros % 1000000

valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())")

values = append(values,
order.TradeID, order.OrderID, order.TradePairID,
order.ExchangeAccountID, order.Side, order.Price,
order.Amount, order.Commission, order.CommissionAsset,
order.Status, time.Unix(executedAtSec, int64(executedAtMicros)*1000),
order.ProfitLoss,
)
}

finalQuery := query
for i, vs := range valueStrings {
if i > 0 {
finalQuery += ", "
}
finalQuery += vs
}

result, err := l.db.Exec(finalQuery, values...)
if err != nil {
return fmt.Errorf("batch insert failed: %w", err)
}

rowsAffected, err := result.RowsAffected()
if err != nil {
return err
}

if rowsAffected == int64(len(l.buffer)) {
l.buffer = make([]*OrderExecution, 0, l.maxBuffer)
}

return nil
}

// GetTotalProfitLoss вычисляет общий P&L
func (l *TradeHistoryLogger) GetTotalProfitLoss(tradeID int) (float64, error) {
var totalPL float64
err := l.db.QueryRow(
`SELECT COALESCE(SUM(PROFIT_LOSS), 0) FROM TRADE_HISTORY WHERE TRADE_ID = ? AND PROFIT_LOSS IS NOT NULL`,
tradeID).Scan(&totalPL)
return totalPL, err
}
