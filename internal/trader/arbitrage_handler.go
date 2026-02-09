package trader

import (
"context"
"database/sql"
"fmt"
"sync"
"time"
)

// ArbitrageTransHandler отслеживает ARBITRAGE_TRANS записи
type ArbitrageTransHandler struct {
db            *sql.DB
lastCheckedID int64
ctx           context.Context
cancel        context.CancelFunc
wg            sync.WaitGroup
}

// NewArbitrageTransHandler создает новый обработчик
func NewArbitrageTransHandler(db *sql.DB) *ArbitrageTransHandler {
return &ArbitrageTransHandler{
db:            db,
lastCheckedID: 0,
}
}

// Start запускает фоновый горутин
func (h *ArbitrageTransHandler) Start(ctx context.Context, pollInterval time.Duration) error {
h.ctx, h.cancel = context.WithCancel(ctx)
h.wg.Add(1)
go h.pollLoop(pollInterval)
return nil
}

// Stop останавливает мониторинг
func (h *ArbitrageTransHandler) Stop() error {
h.cancel()
h.wg.Wait()
return nil
}

// pollLoop периодически проверяет новые записи
func (h *ArbitrageTransHandler) pollLoop(pollInterval time.Duration) {
defer h.wg.Done()
ticker := time.NewTicker(pollInterval)
defer ticker.Stop()

for {
select {
case <-h.ctx.Done():
return
case <-ticker.C:
if err := h.checkNewTransactions(); err != nil {
fmt.Printf("check arbitrage error: %v\n", err)
}
}
}
}

// checkNewTransactions загружает новые транзакции
func (h *ArbitrageTransHandler) checkNewTransactions() error {
query := `SELECT ID, TRADE_ID, STATUS, AMOUNT, CALC_PRFIT, DATE_CREATE, DATE_MODIFY
FROM ARBITRAGE_TRANS WHERE STATUS = 1 AND ID > ? ORDER BY ID ASC`

rows, err := h.db.QueryContext(h.ctx, query, h.lastCheckedID)
if err != nil {
return err
}
defer rows.Close()

for rows.Next() {
var id, tradeID int64
var status int
var amount, profit sql.NullFloat64
var created, modified time.Time

if err := rows.Scan(&id, &tradeID, &status, &amount, &profit, &created, &modified); err != nil {
return err
}

h.lastCheckedID = id
}

return rows.Err()
}

// RecoverSuspendedTransactions восстанавливает транзакции
func (h *ArbitrageTransHandler) RecoverSuspendedTransactions() (int, error) {
result, err := h.db.ExecContext(h.ctx, 
`UPDATE ARBITRAGE_TRANS SET STATUS = 1, DATE_MODIFY = NOW() WHERE STATUS = 3`)
if err != nil {
return 0, err
}

affected, err := result.RowsAffected()
return int(affected), err
}
