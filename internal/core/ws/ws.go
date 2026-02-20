package ws

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"trader/internal/logger"
)

const (
	correlationTTL             = 24 * time.Hour
	correlationCleanupInterval = 1 * time.Hour
)

type correlationEntry struct {
	requestID string
	createdAt time.Time
}

// Pool управляет пулом WebSocket соединений
type Pool struct {
	mu               sync.RWMutex
	eventToRequestID map[string]correlationEntry
	wsInLog          *slog.Logger
	wsOutLog         *slog.Logger
}

// NewPool создает новый WS pool с логгерами ws_in/ws_out
func NewPool() *Pool {
	pool := &Pool{
		eventToRequestID: make(map[string]correlationEntry),
		wsInLog:          logger.GetWSIn("ws_in"),
		wsOutLog:         logger.GetWSOut("ws_out"),
	}

	go pool.correlationCleanupLoop()
	return pool
}

// Subscribe подписывает на пары
func (p *Pool) Subscribe(exchangeID, marketType string, pairs []string, depth int) error {
	_, err := p.SubscribeWithRequestID(exchangeID, marketType, pairs, depth, "")
	return err
}

// SubscribeWithRequestID подписывает на пары и прокидывает request_id в ws_out
// Возвращает event_id для корреляции входящих WS событий.
func (p *Pool) SubscribeWithRequestID(exchangeID, marketType string, pairs []string, depth int, requestID string) (string, error) {
	if len(pairs) == 0 {
		return "", fmt.Errorf("pairs list is empty")
	}

	eventID := newEventID("ws-sub")
	p.rememberCorrelation(eventID, requestID)

	p.wsOutLog.Info(
		"ws subscribe",
		"event_id", eventID,
		"request_id", requestID,
		"exchange_id", exchangeID,
		"market_type", marketType,
		"pairs", strings.Join(pairs, ","),
		"depth", depth,
	)

	return eventID, nil
}

// Unsubscribe отписывает от пар
func (p *Pool) Unsubscribe(exchangeID, marketType string, pairs []string) error {
	_, err := p.UnsubscribeWithRequestID(exchangeID, marketType, pairs, "")
	return err
}

// UnsubscribeWithRequestID отписывает пары и прокидывает request_id в ws_out
// Возвращает event_id для корреляции входящих WS событий.
func (p *Pool) UnsubscribeWithRequestID(exchangeID, marketType string, pairs []string, requestID string) (string, error) {
	if len(pairs) == 0 {
		return "", fmt.Errorf("pairs list is empty")
	}

	eventID := newEventID("ws-unsub")
	p.rememberCorrelation(eventID, requestID)

	p.wsOutLog.Info(
		"ws unsubscribe",
		"event_id", eventID,
		"request_id", requestID,
		"exchange_id", exchangeID,
		"market_type", marketType,
		"pairs", strings.Join(pairs, ","),
	)

	return eventID, nil
}

// LogInboundMessage логирует входящее WS событие в ws_in.
// Если request_id пустой, пытается восстановить его по event_id.
func (p *Pool) LogInboundMessage(exchangeID, marketType, messageType, eventID, requestID string, payloadSize int, status string) {
	if requestID == "" && eventID != "" {
		requestID = p.requestIDByEvent(eventID)
	}

	p.wsInLog.Info(
		"ws inbound",
		"event_id", eventID,
		"request_id", requestID,
		"exchange_id", exchangeID,
		"market_type", marketType,
		"message_type", messageType,
		"payload_size", payloadSize,
		"status", status,
	)
}

func (p *Pool) rememberCorrelation(eventID, requestID string) {
	if eventID == "" || requestID == "" {
		return
	}
	p.mu.Lock()
	p.eventToRequestID[eventID] = correlationEntry{
		requestID: requestID,
		createdAt: time.Now().UTC(),
	}
	p.mu.Unlock()
}

func (p *Pool) requestIDByEvent(eventID string) string {
	p.mu.Lock()
	entry, ok := p.eventToRequestID[eventID]
	if !ok {
		p.mu.Unlock()
		return ""
	}

	if time.Since(entry.createdAt) > correlationTTL {
		delete(p.eventToRequestID, eventID)
		p.mu.Unlock()
		return ""
	}

	delete(p.eventToRequestID, eventID)
	p.mu.Unlock()
	return entry.requestID
}

func (p *Pool) correlationCleanupLoop() {
	ticker := time.NewTicker(correlationCleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		p.cleanupExpiredCorrelations(time.Now().UTC())
	}
}

func (p *Pool) cleanupExpiredCorrelations(now time.Time) {
	cutoff := now.Add(-correlationTTL)

	p.mu.Lock()
	for eventID, entry := range p.eventToRequestID {
		if entry.createdAt.Before(cutoff) {
			delete(p.eventToRequestID, eventID)
		}
	}
	p.mu.Unlock()
}

func newEventID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err == nil {
		return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}
