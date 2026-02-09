package task

import (
	"fmt"
	"sync"

	"ctdaemon/internal/core/exchange"
	"ctdaemon/internal/core/ws"
)

// SubscriptionManager сравнивает новые задачи с предыдущими и вычисляет дельту
// На основе дельты выполняет подписку/отписку через WS Pool
type SubscriptionManager struct {
	lastMonitoring map[string]*exchange.MonitoringTask // key = GetMonitoringTaskKey()
	lastTrading    map[string]*exchange.TradingTask    // key = GetTradingTaskKey()

	wsPool *ws.Pool

	mu sync.RWMutex
}

// SubscriptionDiff содержит изменения которые нужно применить
type SubscriptionDiff struct {
	// Подписаться на новые пары
	ToSubscribe []*Subscription

	// Отписаться от удаленных пар
	Unsubscribe []*Subscription
}

// Subscription описывает одну группу пар на одной бирже/рынке
type Subscription struct {
	ExchangeID string   // binance, bybit и т.д.
	MarketType string   // spot или futures
	Pairs      []string // ["BTC/USDT", "ETH/USDT", ...]
	Depth      int      // 20, 50 или 0 (полная книга) - для мониторинга
}

// NewSubscriptionManager создает новый менеджер подписок
func NewSubscriptionManager(wsPool *ws.Pool) *SubscriptionManager {
	return &SubscriptionManager{
		lastMonitoring: make(map[string]*exchange.MonitoringTask),
		lastTrading:    make(map[string]*exchange.TradingTask),
		wsPool:         wsPool,
	}
}

// Merge сравнивает новые задачи с предыдущими и возвращает дельту
// Содержит список пар которые нужно подписать и отписать
func (sm *SubscriptionManager) Merge(newTasks *TasksData) (*SubscriptionDiff, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Строим новые maps из текущих задач
	newMonitoring := make(map[string]*exchange.MonitoringTask)
	newTrading := make(map[string]*exchange.TradingTask)

	for _, task := range newTasks.MonitoringTasks {
		key := exchange.GetMonitoringTaskKey(*task)
		newMonitoring[key] = task
	}

	for _, task := range newTasks.TradingTasks {
		key := exchange.GetTradingTaskKey(*task)
		newTrading[key] = task
	}

	// Вычисляем дельту
	toSubscribe := sm.computeSubscribe(newMonitoring, newTrading)
	unsubscribe := sm.computeUnsubscribe(newMonitoring, newTrading)

	// Обновляем текущее состояние
	sm.lastMonitoring = newMonitoring
	sm.lastTrading = newTrading

	return &SubscriptionDiff{
		ToSubscribe: toSubscribe,
		Unsubscribe: unsubscribe,
	}, nil
}

// computeSubscribe вычисляет новые подписки
// Возвращает пары которых не было в старом состоянии
func (sm *SubscriptionManager) computeSubscribe(
	newMonitoring map[string]*exchange.MonitoringTask,
	newTrading map[string]*exchange.TradingTask,
) []*Subscription {
	// Объединяем мониторинг и торговлю по (exchangeID, marketType)
	// Для каждой пары берем максимальную глубину
	pairsByExchangeMarket := make(map[string]map[string]int) // key1 = "exchange:market", key2 = pair, value = depth

	// Добавляем пары из мониторинга
	for key, task := range newMonitoring {
		if _, exists := sm.lastMonitoring[key]; !exists {
			// Это новая пара мониторинга
			emKey := fmt.Sprintf("%s:%s", task.ExchangeID, task.MarketType)
			if _, ok := pairsByExchangeMarket[emKey]; !ok {
				pairsByExchangeMarket[emKey] = make(map[string]int)
			}
			// Для мониторинга используем его глубину
			pairsByExchangeMarket[emKey][task.TradePair] = task.OrderbookDepth
		}
	}

	// Добавляем пары из торговли (они всегда нужны независимо от depth)
	for key, task := range newTrading {
		if _, exists := sm.lastTrading[key]; !exists {
			// Это новая пара торговли
			emKey := fmt.Sprintf("%s:%s", task.ExchangeID, task.MarketType)
			if _, ok := pairsByExchangeMarket[emKey]; !ok {
				pairsByExchangeMarket[emKey] = make(map[string]int)
			}
			// Для торговли глубина не так важна, но нужна какая-то
			// Если уже есть из мониторинга, берем его, иначе 50 по умолчанию
			if _, hasMonitoring := pairsByExchangeMarket[emKey][task.TradePair]; !hasMonitoring {
				pairsByExchangeMarket[emKey][task.TradePair] = 50
			}
		}
	}

	// Преобразуем в список Subscription
	var result []*Subscription
	for emKey, pairs := range pairsByExchangeMarket {
		if len(pairs) > 0 {
			var pairNames []string
			var depth int // берем первую (они одинаковые на одном em)

			for pairName, d := range pairs {
				pairNames = append(pairNames, pairName)
				if depth == 0 {
					depth = d
				}
			}

			// Парсим ключ
			var em string
			_, _ = fmt.Sscanf(emKey, "%s", &em)
			parts := splitExchangeMarket(emKey)
			if len(parts) == 2 {
				result = append(result, &Subscription{
					ExchangeID: parts[0],
					MarketType: parts[1],
					Pairs:      pairNames,
					Depth:      depth,
				})
			}
		}
	}

	return result
}

// computeUnsubscribe вычисляет отписки
// Возвращает пары которые были в старом состоянии но нет в новом
func (sm *SubscriptionManager) computeUnsubscribe(
	newMonitoring map[string]*exchange.MonitoringTask,
	newTrading map[string]*exchange.TradingTask,
) []*Subscription {
	pairsByExchangeMarket := make(map[string]map[string]bool) // key1 = "exchange:market", key2 = pair

	// Проверяем старые задачи мониторинга
	for key := range sm.lastMonitoring {
		if _, exists := newMonitoring[key]; !exists {
			// Эта задача была удалена
			task := sm.lastMonitoring[key]
			emKey := fmt.Sprintf("%s:%s", task.ExchangeID, task.MarketType)
			if _, ok := pairsByExchangeMarket[emKey]; !ok {
				pairsByExchangeMarket[emKey] = make(map[string]bool)
			}
			pairsByExchangeMarket[emKey][task.TradePair] = true
		}
	}

	// Проверяем старые задачи торговли
	for key := range sm.lastTrading {
		if _, exists := newTrading[key]; !exists {
			// Эта задача была удалена
			task := sm.lastTrading[key]
			emKey := fmt.Sprintf("%s:%s", task.ExchangeID, task.MarketType)
			if _, ok := pairsByExchangeMarket[emKey]; !ok {
				pairsByExchangeMarket[emKey] = make(map[string]bool)
			}
			pairsByExchangeMarket[emKey][task.TradePair] = true
		}
	}

	// Преобразуем в список Subscription
	var result []*Subscription
	for emKey, pairs := range pairsByExchangeMarket {
		if len(pairs) > 0 {
			var pairNames []string
			for pairName := range pairs {
				pairNames = append(pairNames, pairName)
			}

			parts := splitExchangeMarket(emKey)
			if len(parts) == 2 {
				result = append(result, &Subscription{
					ExchangeID: parts[0],
					MarketType: parts[1],
					Pairs:      pairNames,
				})
			}
		}
	}

	return result
}

// ApplyDiff применяет изменения через WS Pool
func (sm *SubscriptionManager) ApplyDiff(diff *SubscriptionDiff) error {
	// Подписаться на новые пары
	for _, sub := range diff.ToSubscribe {
		if err := sm.wsPool.Subscribe(sub.ExchangeID, sub.MarketType, sub.Pairs, sub.Depth); err != nil {
			return fmt.Errorf("subscribe failed for %s:%s: %w", sub.ExchangeID, sub.MarketType, err)
		}
	}

	// Отписаться от удаленных пар
	for _, sub := range diff.Unsubscribe {
		if err := sm.wsPool.Unsubscribe(sub.ExchangeID, sub.MarketType, sub.Pairs); err != nil {
			return fmt.Errorf("unsubscribe failed for %s:%s: %w", sub.ExchangeID, sub.MarketType, err)
		}
	}

	return nil
}

// splitExchangeMarket парсит ключ формата "exchange:market"
func splitExchangeMarket(key string) []string {
	parts := make([]string, 0)
	current := ""
	for _, ch := range key {
		if ch == ':' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
