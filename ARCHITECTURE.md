# Архитектура ct-system daemon2

## 1. Обзор системы

### Назначение
Высокопроизводительная система мониторинга и торговли на множественных криптобиржах (CEX и DEX) с поддержкой параллельных инстансов, работающих на разных хостах с общей базой данных.

### Ключевые характеристики
- **Масштабируемость**: множество демонов на разных хостах, одна БД
- **Гибкость**: каждый демон может быть мониторором, трейдером или обоим
- **Надежность**: восстановление соединений, переподписка при обрывах
- **Производительность**: асинхронная обработка, пулы WebSocket соединений
- **Универсальность**: единый формат сообщений для всех бирж

### Поддерживаемые биржи
- CEX: Binance, Bybit, OKX, Kucoin, Coinex, HTX, MEXC
- DEX: (расширяемая архитектура)

---

## 2. Декомпозиция задачи

### 2.1 Основные компоненты системы

```
┌─────────────────────────────────────────────────────────────┐
│                    DAEMON INSTANCE                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              CONFIGURATION LOADER                    │  │
│  │  • Load role config (monitor/trader/both)           │  │
│  │  • Load exchange endpoints                          │  │
│  │  • Load monitoring/trading tasks from MySQL         │  │
│  └──────────────────────────────────────────────────────┘  │
│                         ↓                                   │
│  ┌──────────────────────────────────────────────────────┐  │
│  │        EXCHANGE CONNECTION POOL MANAGER             │  │
│  │  • Create/manage WebSocket connections              │  │
│  │  • Pool management (max 30-50 pairs per WS)        │  │
│  │  • Ping/pong heartbeat                              │  │
│  │  • Reconnection with exponential backoff            │  │
│  │  • Unified message format conversion                │  │
│  └──────────────────────────────────────────────────────┘  │
│         ↓                                    ↓              │
│  ┌──────────────────────┐         ┌─────────────────────┐  │
│  │  ORDER BOOK MANAGER  │         │  MESSAGE ROUTER     │  │
│  │  • Ring buffers      │         │  • Format conv.     │  │
│  │  • Data updates      │         │  • Dispatch to      │  │
│  │  • Pub/sub system    │         │    subscribers      │  │
│  └──────────────────────┘         └─────────────────────┘  │
│         ↑                                    ↑              │
│  ┌──────────────────────┐         ┌─────────────────────┐  │
│  │     MONITOR          │         │      TRADER         │  │
│  │  • Sub to orderbook  │         │  • Sub to orderbook │  │
│  │  • Buffer history    │         │  • Apply strategy   │  │
│  │  • Batch to CH       │         │  • Execute orders   │  │
│  └──────────────────────┘         └─────────────────────┘  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
         ↓                                              ↓
    ┌─────────────┐                          ┌────────────────┐
    │  ClickHouse │                          │  MySQL Remote  │
    │ (history)   │                          │  (config, data)│
    └─────────────┘                          └────────────────┘
```

### 2.2 Поток данных

```
[TASK PERIODICAL FETCH]  (every 5-10sec)
         ↓
[MONITORING SQL] + [TRADING SQL]
         ↓
[MERGE & DEDUPLICATE]
         ↓
[DIFF WITH PREVIOUS STATE]
         ↓
[SUBSCRIBE/UNSUBSCRIBE ACTIONS]
         ↓
[WS CONNECTION POOL MANAGER]
         ↓
[WEBSOCKET SUBSCRIPTIONS] (Spot/Futures per exchange)
         ↓
[UNIFIED MESSAGE FORMAT]
         ↓
[MESSAGE ROUTER]
    ↙        ↓        ↖
[MONITOR] [ORDERBOOK] [TRADER]
    ↓        ↓          ↓
   CH      Ring      Strategy
          Buffers    Execution
```

---

## 3. Архитектура модулей

### 3.1 Структура папок

```
internal/
├── api/                      # REST API сервер
├── config/                   # Конфигурация
├── db/                       # Database drivers
├── logger/                   # Логирование
├── state/                    # Состояние демона
│
├── core/                     # === ОБЩИЕ КОМПОНЕНТЫ ===
│   ├── exchange/             # Работа с биржами
│   │   ├── types.go          # Общие типы (ExchangeID, Market, etc)
│   │   ├── factory.go        # Factory для создания драйверов
│   │   ├── driver.go         # Interface для драйвера биржи
│   │   └── drivers/
│   │       ├── binance/
│   │       ├── bybit/
│   │       ├── okx/
│   │       ├── kucoin/
│   │       ├── coinex/
│   │       ├── htx/
│   │       ├── mexc/
│   │       └── dex/
│   │
│   ├── orderbook/            # Order book управление
│   │   ├── types.go          # OrderBook, Tick, Level
│   │   ├── manager.go        # Управление множественными OB
│   │   └── ringbuffer.go     # Ring buffer для истории
│   │
│   ├── messaging/            # Единый формат сообщений
│   │   ├── message.go        # Unified message format
│   │   └── converters/       # Конвертеры для каждой биржи
│   │       ├── binance_converter.go
│   │       ├── bybit_converter.go
│   │       └── ...
│   │
│   ├── ws/                   # WebSocket управление
│   │   ├── connection.go     # Одно WS соединение
│   │   ├── pool.go           # Пул соединений
│   │   ├── manager.go        # Менеджер пулов
│   │   ├── heartbeat.go      # Ping/pong логика
│   │   └── reconnect.go      # Восстановление соединений
│   │
│   └── pubsub/               # Pub/sub система
│       ├── publisher.go      # Publisher для orderbook
│       ├── subscriber.go     # Subscriber interface
│       └── router.go         # Маршрутизация событий
│
├── task/                     # === ЗАДАЧИ ===
│   ├── fetcher.go            # Периодич. загрузка из MySQL
│   ├── monitor_fetcher.go    # Загрузка MONITORING таблицы
│   ├── trader_fetcher.go     # Загрузка TRADE таблицы
│   ├── merger.go             # Слияние/дедупликация задач
│   └── subscription_manager.go # Управление подписками
│
├── monitor/                  # ===== MONITOR ROLE =====
│   ├── monitor.go            # Главный контроллер монитора
│   ├── collector.go          # Сборка данных из orderbook
│   ├── buffer.go             # Буферизация истории
│   ├── snapshots.go          # Периодич. снимки
│   └── clickhouse/
│       ├── client.go         # ClickHouse client
│       ├── schema.go         # SQL схемы таблиц
│       └── writer.go         # Запись дельт и снимков
│
├── trader/                   # ===== TRADER ROLE =====
│   ├── trader.go             # Главный контроллер трейдера
│   ├── portfolio.go          # Портфель позиций
│   ├── strategies/           # Стратегии торговли
│   │   ├── strategy.go       # Interface стратегии
│   │   └── implementations/
│   │       └── grid.go       # Grid стратегия (пример)
│   ├── executor.go           # Выполнение ордеров
│   ├── private_ws.go         # Приватный WS (заполнения, позиции)
│   └── history.go            # История торговли
│
└── manager/                  # ===== ГЛАВНЫЙ ОРКЕСТРАТОР =====
    ├── manager.go            # Главный Manager
    ├── factory.go            # Factory для создания компонентов
    └── lifecycle.go          # Управление жизненным циклом
```

### 3.2 Ключевые интерфейсы

#### Core Interfaces

```go
// core/exchange/driver.go
type ExchangeDriver interface {
    GetExchangeID() string
    GetName() string
    
    // WebSocket endpoints
    GetSpotWSEndpoint() string
    GetFuturesWSEndpoint() string
    
    // REST endpoints
    GetOrderBookEndpoint() string
    
    // Subscribe/Unsubscribe messages
    CreateSubscribeMessage(pairs []string, marketType string) ([]byte, error)
    CreateUnsubscribeMessage(pairs []string, marketType string) ([]byte, error)
    
    // Message parsing
    ParseMessage(data []byte) (*unified.Message, error)
}

// core/messaging/message.go
type Message struct {
    Timestamp  int64       // Unix milliseconds
    ExchangeID string
    MarketType string      // "spot", "futures"
    Type       string      // "orderbook", "trade", "position", "order"
    Pair       string      // e.g., "BTC/USDT"
    
    // OrderBook specific
    OrderBook *OrderBookData
    
    // Trade specific
    Trade *TradeData
}

type OrderBookData struct {
    Bids   []Level        // [price, amount]
    Asks   []Level
    Depth  int            // 20, 50, or full
    Sequence int64        // Exchange sequence number
}

// core/orderbook/types.go
type OrderBookManager interface {
    UpdateOrderBook(msg *unified.Message) error
    GetOrderBook(exchangeID, pair, marketType string) *OrderBook
    Subscribe(subscriber Subscriber, exchangeID, pair, marketType string)
    Unsubscribe(subscriber Subscriber, exchangeID, pair, marketType string)
}

// core/ws/pool.go
type WSPool interface {
    // Subscribe pair on exchange (finds or creates WS connection)
    Subscribe(exchangeID, marketType string, pairs []string) error
    
    // Unsubscribe pair from exchange
    Unsubscribe(exchangeID, marketType string, pairs []string) error
    
    // Get active subscriptions
    GetSubscriptions(exchangeID, marketType string) []string
}

// core/pubsub/subscriber.go
type Subscriber interface {
    OnMessage(msg *unified.Message)
    OnError(err error)
    GetID() string
}
```

#### Task Interfaces

```go
// task/fetcher.go
type TaskFetcher interface {
    // Fetch fresh data from MySQL
    Fetch() (*TasksData, error)
    
    // Get last fetched data
    GetLast() *TasksData
}

type TasksData struct {
    Timestamp int64
    
    // Monitoring tasks (for monitor role)
    MonitoringTasks []MonitoringTask
    
    // Trading tasks (for trader role)
    TradingTasks []TradingTask
}

type MonitoringTask struct {
    ExchangeID  string
    ExchangeName string
    MarketType  string // "spot", "futures"
    TradePairID int
    TradePair   string // e.g., "BTC/USDT"
}

type TradingTask struct {
    ExchangeID   string
    ExchangeName string
    MarketType   string
    TradePairID  int
    TradePair    string
    StrategyID   string
    StrategyParams map[string]interface{}
}

// task/subscription_manager.go
type SubscriptionManager interface {
    // Compare new tasks with previous, return diff
    Merge(newTasks *TasksData) *SubscriptionDiff
    
    // Apply diff (subscribe/unsubscribe)
    ApplyDiff(diff *SubscriptionDiff) error
}

type SubscriptionDiff struct {
    ToSubscribe   []Subscription
    Unsubscribe   []Subscription
}

type Subscription struct {
    ExchangeID string
    MarketType string
    Pairs      []string
}
```

#### Monitor & Trader Interfaces

```go
// monitor/monitor.go
type MonitorRole interface {
    Start(ctx context.Context) error
    Stop() error
    
    // Called when new orderbook data available
    OnOrderBookUpdate(msg *unified.Message)
}

// trader/trader.go
type TraderRole interface {
    Start(ctx context.Context) error
    Stop() error
    
    // Called when orderbook updated
    OnOrderBookUpdate(msg *unified.Message)
    
    // Called when order execution/position update
    OnPrivateWSMessage(msg *unified.Message)
}

// trader/strategies/strategy.go
type Strategy interface {
    GetID() string
    
    // Evaluate market data and return action
    Evaluate(orderbook *OrderBook, portfolio *Portfolio) *TradeAction
    
    // Update portfolio after execution
    OnExecuted(order *Order)
}

type TradeAction struct {
    Type   string  // "buy", "sell", "close", "none"
    Price  float64
    Amount float64
    Reason string  // Debug info
}
```

---

## 4. Ключевые компоненты: детальное описание

### 4.1 Exchange Connection Pool Manager

**Назначение**: управление WebSocket соединениями для каждой биржи/marketType с поддержкой до 30-50 пар на соединение.

**Поток работы**:
1. Получить запрос на подписку (пары + exchangeID + marketType)
2. Проверить существующие пулы
3. Если нет подходящей реальной связи или достигнут лимит пар - создать новую
4. Отправить сообщение подписки
5. Отслеживать heartbeat (ping/pong) согласно протоколу биржи

**Особенности по биржам**:
- Binance: отправляем ping, ждем pong, интервал 5-10 сек
- Bybit: отправляем ping, ждем pong
- OKX: отправляем ping, ждем pong
- Kucoin: сервер отправляет ping, мы отправляем pong
- Coinex: отправляем ping, ждем pong
- HTX: отправляем ping, ждем pong
- MEXC: отправляем ping, ждем pong

### 4.2 Unified Message Format

**Назначение**: преобразовать все сообщения от разных бирж в единый формат.

```go
type Message struct {
    // Meta
    Timestamp   int64         // Unix milliseconds (стандартизовано)
    ExchangeID  string        // "binance", "bybit", etc
    MarketType  string        // "spot" или "futures"
    
    // Pair info
    Pair        string        // Всегда "BTC/USDT" формат
    
    // Type-specific data
    Type        string        // "orderbook", "trade", "position", "order"
    Data        interface{}   // Специфичные данные
}

// Для orderbook
type OrderBookData struct {
    Bids     []Level  // Sorted by price desc
    Asks     []Level  // Sorted by price asc
    Depth    int      // 20, 50 или full
    SeqNum   int64    // Sequence number от биржи
}

type Level struct {
    Price  float64
    Amount float64
}
```

### 4.3 Task Periodical Fetcher

**Назначение**: каждые 5-10 сек загружать из MySQL новые задачи мониторинга/торговли.

**SQL запросы**:
```sql
-- MONITORING таблица
SELECT 
    EXCHANGE_ID, EXCHANGE_NAME, MARKET_TYPE, TRADE_PAIR_ID, TRADE_PAIR
FROM MONITORING
WHERE ENABLED = 1 AND DAEMON_PRIORITY > 0
ORDER BY DAEMON_PRIORITY DESC;

-- TRADE таблица
SELECT 
    EXCHANGE_ID, EXCHANGE_NAME, MARKET_TYPE, TRADE_PAIR_ID, TRADE_PAIR,
    STRATEGY_ID, STRATEGY_PARAMS
FROM TRADE
WHERE ENABLED = 1 AND DAEMON_PRIORITY > 0
ORDER BY DAEMON_PRIORITY DESC;
```

**Логика слияния**:
1. Загрузить новые данные
2. Дедупликация (по exchangeID + marketType + pair)
3. Сравнить с предыдущим состоянием
4. Найти:
   - ToSubscribe: новые пары
   - Unsubscribe: удаленные пары
5. Применить изменения к WS пулам

### 4.4 Order Book Manager with Pub/Sub

**Назначение**: управлять множественными order books, буферизировать историю, распределять обновления подписчикам.

**Компоненты**:
- **Ring Buffer** (для Monitor): сохраняет последние N обновлений orderbook
- **Current State** (для Trader): только текущее состояние orderbook
- **Pub/Sub Router**: распределяет обновления подписчикам

**Сценарии использования**:
- Monitor подписан → получает обновление → добавляет в ring buffer → батчит в ClickHouse
- Trader подписан → получает обновление → применяет стратегию → может выполнить ордер

### 4.5 Monitor Role

**Назначение**: собирать полную историю orderbook с бирж и батчить в ClickHouse.

**Поток**:
1. Подписывается на orderbook события через pub/sub
2. Получает обновления в ring buffer
3. Периодически (например, каждую секунду или по таймеру) батчит в ClickHouse:
   - Дельты: каждое изменение orderbook
   - Снимки: полный срез orderbook каждые N секунд

**ClickHouse схемы**:
```sql
-- Дельты (изменения)
CREATE TABLE orderbook_deltas (
    timestamp DateTime,
    exchange_id String,
    market_type String,
    pair String,
    side String,          -- 'bid' или 'ask'
    price Float64,
    amount Float64,
    action String         -- 'update' или 'delete'
) ENGINE = MergeTree()
ORDER BY (timestamp, exchange_id, pair);

-- Снимки (snapshots)
CREATE TABLE orderbook_snapshots (
    timestamp DateTime,
    exchange_id String,
    market_type String,
    pair String,
    bids Array(Tuple(Float64, Float64)),  -- [price, amount]
    asks Array(Tuple(Float64, Float64)),
    sequence_num Int64
) ENGINE = MergeTree()
ORDER BY (timestamp, exchange_id, pair);
```

### 4.6 Trader Role

**Назначение**: торговать, следить за портфелем, исполнять стратегии.

**Компоненты**:
- **Portfolio**: текущие позиции, остаток
- **Strategy Engine**: вычисляет торговые действия
- **Order Executor**: отправляет ордера через REST API
- **Private WS Listener**: получает исполнения, обновления позиций
- **Trade History**: логирует все операции в БД

**Поток**:
1. Получает обновление orderbook (на актуальном состоянии, без истории)
2. Применяет стратегию к текущему orderbook + portfolio
3. Если стратегия вернула TradeAction:
   - Вычисляет параметры ордера
   - Отправляет REST запрос на создание ордера
4. Слушает приватный WS на исполнения
5. Обновляет портфель, логирует в БД

---

## 5. Интеграция компонентов

### 5.1 Жизненный цикл демона

```
1. Load Config
   ├─ Check role (monitor/trader/both)
   └─ Load exchange endpoints
   
2. Init Database
   └─ Connect to MySQL
   
3. Init Logger
   └─ Setup logging
   
4. Init Manager
   ├─ Create Task Fetcher
   ├─ Create Exchange Factory
   ├─ Create WS Pool Manager
   ├─ Create OrderBook Manager
   ├─ Create Pub/Sub Router
   ├─ If monitor role:
   │  └─ Create Monitor instance
   └─ If trader role:
      └─ Create Trader instance
   
5. Start Manager
   ├─ Start WS Pool Manager (listen to messages)
   ├─ Start Task Fetcher (periodic)
   ├─ Start Subscription Manager (apply diffs)
   ├─ If monitor role:
   │  └─ Start Monitor
   └─ If trader role:
      ├─ Start Private WS Listener
      └─ Start Trader
   
6. API Server
   └─ Listen for commands
   
7. Graceful Shutdown
   ├─ Stop accepting new tasks
   ├─ Close all WS connections
   ├─ Flush all buffers
   ├─ Save state
   └─ Exit
```

### 5.2 Concurrency Model

```
Goroutines:
├─ Main thread
│  └─ API Server (blocking)
│
├─ Task Fetcher
│  └─ Periodic ticker (5-10 sec)
│     └─ Fetch from MySQL
│
├─ Subscription Manager
│  └─ Watch task changes
│     └─ Apply WS diffs
│
├─ WS Pool Manager
│  ├─ For each connection:
│  │  ├─ Message reader loop
│  │  ├─ Heartbeat ticker
│  │  └─ Reconnection handler
│  │
│  └─ Route messages to:
│     ├─ OrderBook Manager
│     └─ Message Router
│
├─ OrderBook Manager
│  └─ Update state + notify subscribers
│
├─ Monitor (if enabled)
│  ├─ Orderbook event handler
│  └─ Batch writer to ClickHouse
│
└─ Trader (if enabled)
   ├─ Orderbook event handler
   ├─ Strategy evaluator
   ├─ Order executor
   └─ Private WS listener
```

### 5.3 Error Handling & Recovery

```
WebSocket Connection Lost:
├─ Close connection
├─ Remove from pool
├─ Exponential backoff reconnection
│  ├─ 1s, 2s, 4s, 8s, 16s... up to 5min
│  └─ Unlimited retries
├─ On reconnect:
│  └─ Re-subscribe all pairs

Task Fetch Failed:
├─ Log error
├─ Use last known state
├─ Retry on next tick

Order Execution Failed:
├─ Log error
├─ Retry if transient
├─ Alert if critical

Database Connection Lost:
├─ Monitor: buffer data locally, retry flush
├─ Trader: stop executing new orders, hold position
```

---

## 6. Спецификация по глубине orderbook

### Варианты глубины:

1. **Depth 20** (быстрая, ~10-50ms задержка)
   - Использование: основная для большинства стратегий
   - Размер обновления: ~500B
   - Преимущества: низкая задержка, мало трафика

2. **Depth 50** (средняя, ~20-100ms задержка)
   - Использование: для анализа глубины
   - Размер обновления: ~1.2KB
   - Преимущества: хороший баланс

3. **Full Depth** (полная, +100-500ms)
   - Использование: только если нужна полная история
   - Размер обновления: ~5-10KB
   - Преимущества: полная информация

### BBO (Best Bid/Offer)

**BBO** - только лучшие bid/ask (1 уровень):
- Размер: ~100B
- Задержка: ~1-5ms (самый быстрый)
- Использование: только для BBO-зависимых стратегий (например, микротрейдинг)

**Рекомендация**:
- Monitor: Depth 50 или Full (для истории)
- Trader: Depth 20 по умолчанию, BBO для специальных стратегий

---

## 7. SQL Schema (заготовка)

### MySQL таблицы

```sql
-- Конфигурация мониторинга
CREATE TABLE MONITORING (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    EXCHANGE_ID VARCHAR(50),
    EXCHANGE_NAME VARCHAR(100),
    MARKET_TYPE VARCHAR(20),   -- 'spot', 'futures'
    TRADE_PAIR_ID INT,
    TRADE_PAIR VARCHAR(50),    -- e.g., 'BTC/USDT'
    DAEMON_PRIORITY INT,       -- Для load balancing между демонами
    ENABLED BOOLEAN,
    ORDERBOOK_DEPTH INT,       -- 20, 50, или full
    CREATED_AT TIMESTAMP,
    UPDATED_AT TIMESTAMP
);

-- Конфигурация торговли
CREATE TABLE TRADE (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    EXCHANGE_ID VARCHAR(50),
    EXCHANGE_NAME VARCHAR(100),
    MARKET_TYPE VARCHAR(20),
    TRADE_PAIR_ID INT,
    TRADE_PAIR VARCHAR(50),
    STRATEGY_ID VARCHAR(100),
    STRATEGY_PARAMS JSON,      -- e.g., {"grid_step": 0.5, "order_size": 1.0}
    DAEMON_PRIORITY INT,
    ENABLED BOOLEAN,
    CREATED_AT TIMESTAMP,
    UPDATED_AT TIMESTAMP
);

-- История торговли
CREATE TABLE TRADE_HISTORY (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    DAEMON_ID VARCHAR(100),
    EXCHANGE_ID VARCHAR(50),
    TRADE_PAIR VARCHAR(50),
    ORDER_ID VARCHAR(100),
    SIDE VARCHAR(10),          -- 'buy', 'sell'
    PRICE DECIMAL(20,8),
    AMOUNT DECIMAL(20,8),
    COMMISSION DECIMAL(20,8),
    STATUS VARCHAR(50),        -- 'open', 'filled', 'cancelled'
    EXECUTED_AT TIMESTAMP,
    CREATED_AT TIMESTAMP
);

-- Состояние демона (для восстановления)
CREATE TABLE DAEMON_STATE (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    DAEMON_ID VARCHAR(100) UNIQUE,
    ROLE VARCHAR(50),          -- 'monitor', 'trader', 'both'
    STATUS VARCHAR(50),        -- 'running', 'stopped', 'error'
    LAST_HEARTBEAT TIMESTAMP,
    CONFIG JSON,
    CREATED_AT TIMESTAMP,
    UPDATED_AT TIMESTAMP
);
```

---

## 8. Обработка глубины и требования к отправке subscribe сообщений

### Per-Exchange Subscribe Message Format

```go
// Binance
SubscribeMessage: {"method":"SUBSCRIBE","params":["btcusdt@depth@100ms"],"id":1}

// Bybit
SubscribeMessage: {"op":"subscribe","args":["orderbook.50.BTCUSDT"]}

// OKX
SubscribeMessage: {"op":"subscribe","args":[{"channel":"books","instId":"BTC-USDT"}]}

// Kucoin
SubscribeMessage: {"id":"1234","type":"subscribe","topic":"/market/level2:BTC-USDT","response":true}

// MEXC
SubscribeMessage: {"method":"SUBSCRIBE","params":["spot@public.limit.depth@symbol:BTCUSDT"]}
```

### Converter Interface

```go
type MessageConverter interface {
    // Convert exchange-specific message to unified format
    ToUnified(raw []byte) (*Message, error)
    
    // Convert unified subscribe request to exchange format
    CreateSubscribe(pairs []string, depth int) ([]byte, error)
    
    // Convert unified unsubscribe request to exchange format
    CreateUnsubscribe(pairs []string) ([]byte, error)
    
    // Parse exchange heartbeat/ping
    IsPing(data []byte) bool
    CreatePong(pingData []byte) []byte
}
```

---

## 9. Примечания по производительности

### Latency targets
- Orderbook update processing: < 100ms
- Monitor write to ClickHouse: < 1s (batch)
- Trader order execution: < 500ms (от OB update до order send)

### Throughput targets
- WS messages/sec: ~1000-5000 (зависит от количества пар)
- Orders/sec: variable (от стратегии)
- Disk I/O to ClickHouse: batch writes every 1-5s

### Resource limits
- RAM per daemon: ~500MB-2GB (зависит от depth + count)
- WS connections per daemon: ~10-20 (30-50 пар каждое)
- Max pairs per daemon: ~300-500

---

## 10. Расширяемость (DEX, новые биржи)

### Добавление новой биржи

1. Создать `internal/core/exchange/drivers/newexchange/driver.go`
2. Реализовать интерфейс `ExchangeDriver`
3. Создать `internal/core/messaging/converters/newexchange_converter.go`
4. Зарегистрировать в `ExchangeFactory`
5. Добавить конфиг endpoints

### Добавление DEX

1. Создать `internal/core/exchange/drivers/dex/driver.go`
2. Реализовать `ExchangeDriver` (может быть async REST вместо WS)
3. Создать соответствующий converter
4. Интегрировать в систему

