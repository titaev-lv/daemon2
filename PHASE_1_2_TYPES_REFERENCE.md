# Phase 1.2: Базовые типы - Справочник структур

## Быстрые ссылки

**Файлы**:
- `internal/core/exchange/types.go` - основные структуры
- `internal/core/messaging/message.go` - унифицированный формат сообщений

---

## Структуры обмена (exchange/types.go)

### Level - один уровень цены
```go
type Level struct {
    Price  float64  // Цена (45000.00)
    Amount float64  // Объем (2.5)
}
// Пример: Level{45000, 2.5} = "2.5 BTC по цене 45000 USDT"
```

### OrderBook - текущая книга ордеров
```go
type OrderBook struct {
    ExchangeID string  // "binance", "bybit" и т.д.
    Pair       string  // "BTC/USDT"
    MarketType string  // "spot" или "futures"
    Bids       []Level // Покупатели (сортировка вниз)
    Asks       []Level // Продавцы (сортировка вверх)
    Depth      int     // 20, 50 или 0 (full)
    Timestamp  int64   // Unix миллисекунды
    SeqNum     int64   // Номер обновления от биржи
}
```

### MonitoringTask - что мониторить
```go
type MonitoringTask struct {
    ExchangeID   string // "binance"
    ExchangeName string // "Binance"
    MarketType   string // "spot"
    TradePairID  int    // ID в БД
    TradePair    string // "BTC/USDT"
}
// Источник: MySQL MONITORING таблица
// Использование: Monitor подписывается на эти пары
```

### TradingTask - что торговать
```go
type TradingTask struct {
    ExchangeID     string                 // "binance"
    ExchangeName   string                 // "Binance"
    MarketType     string                 // "spot"
    TradePairID    int                    // ID в БД
    TradePair      string                 // "BTC/USDT"
    StrategyID     string                 // "grid", "dca" и т.д.
    StrategyParams map[string]interface{} // {"grid_step": 0.5, ...}
}
// Источник: MySQL TRADE таблица
// Использование: Trader применяет стратегию к этим парам
```

### TasksData - объединение задач
```go
type TasksData struct {
    Timestamp       int64
    MonitoringTasks []MonitoringTask
    TradingTasks    []TradingTask
}
// Возвращается TaskFetcher из MySQL
```

---

## Структуры сообщений (messaging/message.go)

### Message - унифицированный формат
```go
type Message struct {
    Timestamp  int64            // Unix миллисекунды
    ExchangeID string           // "binance"
    MarketType string           // "spot" или "futures"
    Type       string           // "orderbook", "trade", "position", "order"
    Pair       string           // "BTC/USDT"
    SeqNum     int64            // Номер от биржи
    
    // Только одно из этих заполнено:
    OrderBook  *OrderBookData
    Trade      *TradeData
    Position   *PositionData
    Order      *OrderData
}
```

### OrderBookData - для типа "orderbook"
```go
type OrderBookData struct {
    Bids  []Level // Покупатели
    Asks  []Level // Продавцы
    Depth int     // 20, 50 или 0
}
```

### TradeData - для типа "trade"
```go
type TradeData struct {
    Price  float64 // Цена сделки
    Amount float64 // Объем
    Side   string  // "buy" или "sell"
}
```

### PositionData - для типа "position" (приватное)
```go
type PositionData struct {
    Side         string  // "long" или "short"
    Amount       float64 // Объем позиции
    EntryPrice   float64 // Цена входа
    CurrentPrice float64 // Текущая цена
    PnL          float64 // Прибыль/убыток
}
```

### OrderData - для типа "order" (приватное)
```go
type OrderData struct {
    OrderID    string  // ID на бирже
    Side       string  // "buy" или "sell"
    Price      float64
    Amount     float64
    Filled     float64
    Status     string  // "open", "filled", "cancelled" и т.д.
    Commission float64
}
```

---

## Вспомогательные функции

```go
// Ключ для OrderBook в map
GetOrderBookKey("binance", "BTC/USDT", "spot")
// → "binance:spot:BTC/USDT"

// Ключ для Monitoring задачи
GetMonitoringTaskKey(task)
// → "binance:spot:BTC/USDT"

// Ключ для Trading задачи
GetTradingTaskKey(task)
// → "binance:spot:BTC/USDT:grid"

// Ключ для Message (логирование)
GetMessageKey(msg)
// → "binance:spot:BTC/USDT:orderbook"
```

---

## Примеры использования

### Monitor получает обновление OrderBook

```
Binance WS сообщение
  ↓
BinanceConverter.ParseMessage()
  ↓
Message{
  Type: "orderbook",
  OrderBook: &OrderBookData{
    Bids: [{45000, 2.5}, {44999.5, 1.2}],
    Asks: [{45001, 3.0}, {45001.5, 1.5}],
  }
}
  ↓
OrderBook Manager.UpdateOrderBook()
  ↓
Monitor.OnMessage() → Ring Buffer
  ↓
ClickHouse
```

### Trader следит за исполнением

```
OKX Private WS сообщение
  ↓
OKXConverter.ParseMessage()
  ↓
Message{
  Type: "order",
  Order: &OrderData{
    OrderID: "123456",
    Status: "filled",
    ...
  }
}
  ↓
Trader.OnMessage() → обновить Portfolio
  ↓
MySQL TRADE_HISTORY
```

---

## Ключевые отличия типов

| Тип | Источник | Назначение | Приватное |
|-----|----------|-----------|----------|
| OrderBook | WS orderbook | Хранение текущего состояния | Нет |
| MonitoringTask | CTS-Core task flow | Monitor подписка | Нет |
| TradingTask | CTS-Core task flow | Trader подписка | Нет |
| Message | Конвертер | Унифицированный формат | Зависит от Type |
| OrderBookData | Message | Обновление OB | Нет |
| TradeData | Message | Анализ сделок | Нет |
| PositionData | Message | Текущая позиция трейдера | Да |
| OrderData | Message | Исполнение ордера | Да |

---

## Константы

### Exchange IDs
- `exchange.Binance` = "binance"
- `exchange.Bybit` = "bybit"
- `exchange.OKX` = "okx"
- `exchange.Kucoin` = "kucoin"
- `exchange.Coinex` = "coinex"
- `exchange.HTX` = "htx"
- `exchange.MEXC` = "mexc"
- `exchange.DEX` = "dex"

### Market Types
- `exchange.MarketSpot` = "spot"
- `exchange.MarketFutures` = "futures"

### Message Types
- `messaging.TypeOrderBook` = "orderbook"
- `messaging.TypeTrade` = "trade"
- `messaging.TypePosition` = "position"
- `messaging.TypeOrder` = "order"
