package manager

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// DaemonStateTracker отслеживает состояние демона и пишет heartbeat в БД
// Используется для обнаружения и восстановления после crash'а
type DaemonStateTracker struct {
	db *sql.DB

	daemonName string // hostname-pid, уникальный ID этого демона
	role       string // "monitor", "trader", "both"

	status             string // "STARTING", "RUNNING", "STOPPING", "STOPPED", "ERROR"
	activeMonitoringID *int   // Текущая конфигурация мониторинга (если есть)
	activeTradeID      *int   // Текущая конфигурация торговли (если есть)
	lastErrorMessage   string // Последняя ошибка (если status == "ERROR")

	recordID int64 // ID в таблице DAEMON_STATE

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// DaemonState представляет состояние демона из таблицы DAEMON_STATE
type DaemonState struct {
	ID                 int64
	DaemonName         string
	Status             string
	Role               string
	LastHeartbeat      int64 // Unix микросекунды
	ActiveMonitoringID *int
	ActiveTradeID      *int
	ErrorMessage       string
	DateCreate         time.Time
	DateModify         time.Time
}

// NewDaemonStateTracker создает новый трекер состояния демона
func NewDaemonStateTracker(db *sql.DB, daemonName string, role string) *DaemonStateTracker {
	return &DaemonStateTracker{
		db:         db,
		daemonName: daemonName,
		role:       role,
		status:     "STARTING",
	}
}

// Start инициализирует запись демона в БД и запускает heartbeat горутин
func (t *DaemonStateTracker) Start(ctx context.Context) error {
	t.ctx, t.cancel = context.WithCancel(ctx)

	// Создаем или обновляем запись в БД
	if err := t.initializeInDB(); err != nil {
		return fmt.Errorf("initialize daemon state failed: %w", err)
	}

	// Меняем статус на RUNNING
	if err := t.setStatus("RUNNING"); err != nil {
		return fmt.Errorf("set running status failed: %w", err)
	}

	// Запускаем heartbeat горутин (пишет каждые 5 секунд)
	t.wg.Add(1)
	go t.heartbeatLoop()

	return nil
}

// Stop останавливает трекер и обновляет статус в БД
func (t *DaemonStateTracker) Stop() error {
	t.cancel()
	t.wg.Wait()

	// Обновляем статус на STOPPED
	return t.setStatus("STOPPED")
}

// initializeInDB создает или обновляет запись демона в таблице DAEMON_STATE
func (t *DaemonStateTracker) initializeInDB() error {
	now := time.Now()
	heartbeatMicros := now.UnixMicro()

	// Проверяем существует ли уже запись
	var existingID int64
	query := `SELECT ID FROM DAEMON_STATE WHERE DAEMON_NAME = ?`
	err := t.db.QueryRowContext(t.ctx, query, t.daemonName).Scan(&existingID)

	if err == sql.ErrNoRows {
		// Создаем новую запись
		insertQuery := `
			INSERT INTO DAEMON_STATE 
			(DAEMON_NAME, STATUS, ROLE, LAST_HEARTBEAT, DATE_CREATE, DATE_MODIFY)
			VALUES (?, ?, ?, ?, NOW(), NOW())
		`

		result, err := t.db.ExecContext(t.ctx, insertQuery,
			t.daemonName, t.status, t.role, heartbeatMicros)
		if err != nil {
			return fmt.Errorf("insert daemon state failed: %w", err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			return err
		}

		t.recordID = id
		return nil

	} else if err == nil {
		// Обновляем существующую запись (мы переустартили)
		t.recordID = existingID

		updateQuery := `
			UPDATE DAEMON_STATE
			SET STATUS = ?, LAST_HEARTBEAT = ?, DATE_MODIFY = NOW()
			WHERE ID = ?
		`

		_, err := t.db.ExecContext(t.ctx, updateQuery, t.status, heartbeatMicros, t.recordID)
		if err != nil {
			return fmt.Errorf("update daemon state failed: %w", err)
		}

		return nil

	} else {
		return fmt.Errorf("query daemon state failed: %w", err)
	}
}

// heartbeatLoop периодически обновляет LAST_HEARTBEAT в БД
func (t *DaemonStateTracker) heartbeatLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			if err := t.writeHeartbeat(); err != nil {
				// TODO: логирование
				fmt.Printf("heartbeat write error: %v\n", err)
			}
		}
	}
}

// writeHeartbeat обновляет LAST_HEARTBEAT в БД
func (t *DaemonStateTracker) writeHeartbeat() error {
	now := time.Now()
	heartbeatMicros := now.UnixMicro()

	query := `
		UPDATE DAEMON_STATE
		SET LAST_HEARTBEAT = ?, DATE_MODIFY = NOW()
		WHERE ID = ?
	`

	_, err := t.db.ExecContext(t.ctx, query, heartbeatMicros, t.recordID)
	if err != nil {
		return fmt.Errorf("update heartbeat failed: %w", err)
	}

	return nil
}

// setStatus обновляет статус демона
func (t *DaemonStateTracker) setStatus(status string) error {
	t.mu.Lock()
	t.status = status
	t.mu.Unlock()

	now := time.Now()
	heartbeatMicros := now.UnixMicro()

	query := `
		UPDATE DAEMON_STATE
		SET STATUS = ?, LAST_HEARTBEAT = ?, DATE_MODIFY = NOW()
		WHERE ID = ?
	`

	_, err := t.db.ExecContext(t.ctx, query, status, heartbeatMicros, t.recordID)
	if err != nil {
		return fmt.Errorf("update status failed: %w", err)
	}

	return nil
}

// GetStatus возвращает текущий статус демона
func (t *DaemonStateTracker) GetStatus() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status
}

// SetActiveConfigs обновляет текущие активные конфигурации мониторинга/торговли
func (t *DaemonStateTracker) SetActiveConfigs(monitoringID *int, tradeID *int) error {
	t.mu.Lock()
	t.activeMonitoringID = monitoringID
	t.activeTradeID = tradeID
	t.mu.Unlock()

	query := `
		UPDATE DAEMON_STATE
		SET ACTIVE_MONITORING_ID = ?, ACTIVE_TRADE_ID = ?, DATE_MODIFY = NOW()
		WHERE ID = ?
	`

	_, err := t.db.ExecContext(t.ctx, query, monitoringID, tradeID, t.recordID)
	if err != nil {
		return fmt.Errorf("update active configs failed: %w", err)
	}

	return nil
}

// SetError обновляет статус на ERROR с сообщением об ошибке
func (t *DaemonStateTracker) SetError(errorMsg string) error {
	t.mu.Lock()
	t.status = "ERROR"
	t.lastErrorMessage = errorMsg
	t.mu.Unlock()

	now := time.Now()
	heartbeatMicros := now.UnixMicro()

	query := `
		UPDATE DAEMON_STATE
		SET STATUS = ?, ERROR_MESSAGE = ?, LAST_HEARTBEAT = ?, DATE_MODIFY = NOW()
		WHERE ID = ?
	`

	_, err := t.db.ExecContext(t.ctx, query, "ERROR", errorMsg, heartbeatMicros, t.recordID)
	if err != nil {
		return fmt.Errorf("update error status failed: %w", err)
	}

	return nil
}

// GetLastHeartbeat получает время последнего heartbeat
func (t *DaemonStateTracker) GetLastHeartbeat() (time.Time, error) {
	var heartbeatMicros int64

	query := `SELECT LAST_HEARTBEAT FROM DAEMON_STATE WHERE ID = ?`

	err := t.db.QueryRowContext(t.ctx, query, t.recordID).Scan(&heartbeatMicros)
	if err != nil {
		return time.Time{}, fmt.Errorf("query last heartbeat failed: %w", err)
	}

	// Конвертируем микросекунды в time.Time
	sec := heartbeatMicros / 1000000
	usec := heartbeatMicros % 1000000

	return time.Unix(sec, usec*1000), nil
}

// ========== Статические методы для работы с демонами ==========

// CheckDeadDaemon проверяет живо ли соединение демона (не завис ли он)
// Если последний heartbeat был более timeoutSec секунд назад, демон считается мертвым
func CheckDeadDaemon(db *sql.DB, daemonName string, timeoutSec int) (bool, error) {
	query := `
		SELECT LAST_HEARTBEAT FROM DAEMON_STATE 
		WHERE DAEMON_NAME = ? AND STATUS = 'RUNNING'
	`

	var heartbeatMicros int64

	err := db.QueryRow(query, daemonName).Scan(&heartbeatMicros)
	if err == sql.ErrNoRows {
		return true, nil // Демон не запущен
	} else if err != nil {
		return false, fmt.Errorf("query daemon status failed: %w", err)
	}

	// Конвертируем в время
	heartbeatTime := time.Unix(heartbeatMicros/1000000, (heartbeatMicros%1000000)*1000)
	timeSinceHeartbeat := time.Since(heartbeatTime)

	// Если heartbeat был давно, демон мертв
	isDead := timeSinceHeartbeat > time.Duration(timeoutSec)*time.Second

	return isDead, nil
}

// GetAllDaemonStates получает состояние всех демонов
func GetAllDaemonStates(db *sql.DB) ([]*DaemonState, error) {
	query := `
		SELECT ID, DAEMON_NAME, STATUS, ROLE, LAST_HEARTBEAT, 
		       ACTIVE_MONITORING_ID, ACTIVE_TRADE_ID, ERROR_MESSAGE, 
		       DATE_CREATE, DATE_MODIFY
		FROM DAEMON_STATE
		ORDER BY DATE_CREATE DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query all daemon states failed: %w", err)
	}
	defer rows.Close()

	var states []*DaemonState

	for rows.Next() {
		var state DaemonState
		var heartbeatMicros int64
		var monitoringID sql.NullInt64
		var tradeID sql.NullInt64
		var errorMsg sql.NullString

		if err := rows.Scan(
			&state.ID, &state.DaemonName, &state.Status, &state.Role,
			&heartbeatMicros, &monitoringID, &tradeID, &errorMsg,
			&state.DateCreate, &state.DateModify,
		); err != nil {
			return nil, fmt.Errorf("scan daemon state failed: %w", err)
		}

		state.LastHeartbeat = heartbeatMicros

		if monitoringID.Valid {
			state.ActiveMonitoringID = &[]int{int(monitoringID.Int64)}[0]
		}
		if tradeID.Valid {
			state.ActiveTradeID = &[]int{int(tradeID.Int64)}[0]
		}
		if errorMsg.Valid {
			state.ErrorMessage = errorMsg.String
		}

		states = append(states, &state)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return states, nil
}

// GetDaemonState получает состояние конкретного демона
func GetDaemonState(db *sql.DB, daemonName string) (*DaemonState, error) {
	query := `
		SELECT ID, DAEMON_NAME, STATUS, ROLE, LAST_HEARTBEAT, 
		       ACTIVE_MONITORING_ID, ACTIVE_TRADE_ID, ERROR_MESSAGE, 
		       DATE_CREATE, DATE_MODIFY
		FROM DAEMON_STATE
		WHERE DAEMON_NAME = ?
	`

	var state DaemonState
	var heartbeatMicros int64
	var monitoringID sql.NullInt64
	var tradeID sql.NullInt64
	var errorMsg sql.NullString

	err := db.QueryRow(query, daemonName).Scan(
		&state.ID, &state.DaemonName, &state.Status, &state.Role,
		&heartbeatMicros, &monitoringID, &tradeID, &errorMsg,
		&state.DateCreate, &state.DateModify,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Демон не найден
	} else if err != nil {
		return nil, fmt.Errorf("query daemon state failed: %w", err)
	}

	state.LastHeartbeat = heartbeatMicros

	if monitoringID.Valid {
		state.ActiveMonitoringID = &[]int{int(monitoringID.Int64)}[0]
	}
	if tradeID.Valid {
		state.ActiveTradeID = &[]int{int(tradeID.Int64)}[0]
	}
	if errorMsg.Valid {
		state.ErrorMessage = errorMsg.String
	}

	return &state, nil
}

// MarkDaemonStopped помечает демона как остановленного и очищает его
func MarkDaemonStopped(db *sql.DB, daemonName string) error {
	query := `
		UPDATE DAEMON_STATE
		SET STATUS = 'STOPPED', ERROR_MESSAGE = NULL, DATE_MODIFY = NOW()
		WHERE DAEMON_NAME = ?
	`

	_, err := db.Exec(query, daemonName)
	if err != nil {
		return fmt.Errorf("mark daemon stopped failed: %w", err)
	}

	return nil
}
