# План разработки ct-system daemon2

## Структура плана

- **Phase 1**: Фундамент и инфраструктура (недели 1-2)
- **Phase 2**: Обмен и WebSocket (недели 3-4)
- **Phase 3**: Order book и Pub/Sub (неделя 5)
- **Phase 4**: Task & Subscription management (неделя 6)
- **Phase 5**: Monitor role (неделя 7)
- **Phase 6**: Trader role (недели 8-9)
- **Phase 7**: Интеграция и тестирование (неделя 10)
- **Phase 8**: Production hardening (неделя 11+)

---

# PHASE 1: Фундамент и инфраструктура

## 1.1 Подготовка структуры проекта

**Цель**: создать папки и базовые типы данных

**Задачи**:
- [ ] Создать папки: `internal/core/`, `internal/task/`, `internal/monitor/`, `internal/trader/`
- [ ] Создать подпапки в `internal/core/`:
  - `exchange/` и `exchange/drivers/`
  - `orderbook/`
  - `messaging/` и `messaging/converters/`
  - `ws/`
  - `pubsub/`
- [ ] Создать `internal/exchange/drivers/` с подпапками для каждой биржи:
  - `binance/`, `bybit/`, `okx/`, `kucoin/`, `coinex/`, `htx/`, `mexc/`, `dex/`

**Проверка результата**:
```bash
$ find internal/core -type d | head -20
$ find internal/core/exchange/drivers -type d | wc -l  # должно быть 8+
```

---

## 1.2 Определение базовых типов

**Файл**: `internal/core/exchange/types.go`

**Цель**: все общие типы в одном месте

**Содержание**:
```go
package exchange

// ExchangeID constants
const (
    Binance  = "binance"
    Bybit    = "bybit"
    OKX      = "okx"
    Kucoin   = "kucoin"
    Coinex   = "coinex"
    HTX      = "htx"
    MEXC     = "mexc"
    DEX      = "dex"
)

// MarketType constants
const (
    MarketSpot    = "spot"
    MarketFutures = "futures"
)

// OrderBook related
type Level struct {
    Price  float64
    Amount float64
}

type OrderBook struct {
    ExchangeID string
    Pair       string
    MarketType string
    
    Bids       []Level
    Asks       []Level
    Depth      int
    
    Timestamp  int64  // Unix milliseconds
    SeqNum     int64  // Exchange sequence number
}

// Taskа структуры
type MonitoringTask struct {
    ExchangeID   string
    ExchangeName string
    MarketType   string
    TradePairID  int
    TradePair    string
}

type TradingTask struct {
    ExchangeID      string
    ExchangeName    string
    MarketType      string
    TradePairID     int
    TradePair       string
    StrategyID      string
    StrategyParams  map[string]interface{}
}
```

**Проверка результата**:
```bash
$ go test ./internal/core/exchange
```

---

## 1.3 Unified Message Format

**Файл**: `internal/core/messaging/message.go`

**Цель**: единый формат сообщений от всех бирж

**Содержание**:
```go
package messaging

const (
    // Message types
    TypeOrderBook = "orderbook"
    TypeTrade     = "trade"
    TypePosition  = "position"
    TypeOrder     = "order"
)

type Message struct {
    Timestamp  int64       // Unix milliseconds
    ExchangeID string
    MarketType string
    Type       string
    
    Pair       string
    SeqNum     int64       // Exchange sequence number
    
    // Type-specific data
    OrderBook  *OrderBookData
    Trade      *TradeData
    Position   *PositionData
    Order      *OrderData
}

type OrderBookData struct {
    Bids   []Level
    Asks   []Level
    Depth  int    // 20, 50, full (0)
}

type TradeData struct {
    Price  float64
    Amount float64
    Side   string  // "buy", "sell"
}

type PositionData struct {
    // Для трейдера
    Side   string
    Amount float64
    Price  float64
    PnL    float64
}

type OrderData struct {
    OrderID    string
    Side       string
    Price      float64
    Amount     float64
    Status     string
    Filled     float64
    Commission float64
}
```

**Проверка результата**:
```bash
$ go test ./internal/core/messaging
```

---

## 1.4 Обновление Config

**Файл**: `internal/config/config.go`

**Цель**: добавить параметры для monitor/trader ролей

**Изменения**:
- Добавить поле `Role string` (значения: "monitor", "trader", "both")
- Добавить структуру `MonitorConfig` с параметрами мониторинга
- Добавить структуру `TraderConfig` с параметрами торговца
- Добавить `ClickHouseConfig`

**Пример**:
```go
type Config struct {
    Role      string
    Database  DatabaseConfig
    ClickHouse ClickHouseConfig
    Monitor   MonitorConfig
    Trader    TraderConfig
    // ...остальное
}

type MonitorConfig struct {
    OrderBookDepth  int    // 20, 50, или 0 (full)
    BatchSize       int    // Сколько событий батчить
    BatchInterval   int    // В секундах
}

type TraderConfig struct {
    MaxOpenOrders   int
    DefaultStrategy string
}

type ClickHouseConfig struct {
    Host     string
    Port     int
    Database string
    Username string
    Password string
}
```

**Проверка результата**:
```bash
$ grep -n "type Config struct" internal/config/config.go
$ go build ./cmd/daemon/
```

---

## 1.5 Database Schema для MySQL

**Файл**: `internal/db/schema.sql`

**Цель**: создать таблицы в MySQL

**Содержание**:
```sql
-- Таблица MONITORING для мониторинга
CREATE TABLE IF NOT EXISTS MONITORING (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    EXCHANGE_ID VARCHAR(50) NOT NULL,
    EXCHANGE_NAME VARCHAR(100),
    MARKET_TYPE VARCHAR(20),
    TRADE_PAIR_ID INT,
    TRADE_PAIR VARCHAR(50) NOT NULL,
    DAEMON_PRIORITY INT DEFAULT 0,
    ENABLED BOOLEAN DEFAULT TRUE,
    ORDERBOOK_DEPTH INT DEFAULT 20,
    CREATED_AT TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UPDATED_AT TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY (EXCHANGE_ID, MARKET_TYPE, ENABLED),
    KEY (DAEMON_PRIORITY)
);

-- Таблица TRADE для торговли
CREATE TABLE IF NOT EXISTS TRADE (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    EXCHANGE_ID VARCHAR(50) NOT NULL,
    EXCHANGE_NAME VARCHAR(100),
    MARKET_TYPE VARCHAR(20),
    TRADE_PAIR_ID INT,
    TRADE_PAIR VARCHAR(50) NOT NULL,
    STRATEGY_ID VARCHAR(100),
    STRATEGY_PARAMS JSON,
    DAEMON_PRIORITY INT DEFAULT 0,
    ENABLED BOOLEAN DEFAULT TRUE,
    CREATED_AT TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UPDATED_AT TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY (EXCHANGE_ID, MARKET_TYPE, ENABLED),
    KEY (DAEMON_PRIORITY)
);

-- Таблица истории торговли
CREATE TABLE IF NOT EXISTS TRADE_HISTORY (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    DAEMON_ID VARCHAR(100),
    EXCHANGE_ID VARCHAR(50),
    TRADE_PAIR VARCHAR(50),
    ORDER_ID VARCHAR(100),
    SIDE VARCHAR(10),
    PRICE DECIMAL(20,8),
    AMOUNT DECIMAL(20,8),
    COMMISSION DECIMAL(20,8),
    STATUS VARCHAR(50),
    EXECUTED_AT TIMESTAMP,
    CREATED_AT TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    KEY (EXCHANGE_ID, TRADE_PAIR, EXECUTED_AT)
);
```

**Проверка результата**:
```bash
$ mysql -u root -p ct_system < internal/db/schema.sql
$ mysql -u root -p -e "DESC ct_system.MONITORING;"
```

---

## 1.6 Exchange Driver Interface

**Файл**: `internal/core/exchange/driver.go`

**Цель**: определить интерфейс для драйвера биржи

**Содержание**:
```go
package exchange

type Driver interface {
    // Identification
    GetExchangeID() string
    GetName() string
    
    // WebSocket endpoints
    GetSpotWSEndpoint() string
    GetFuturesWSEndpoint() string
    
    // REST endpoints (опционально)
    GetOrderBookEndpoint() string
    
    // Subscribe/Unsubscribe messages
    CreateSubscribeMessage(pairs []string, marketType string, depth int) ([]byte, error)
    CreateUnsubscribeMessage(pairs []string, marketType string) ([]byte, error)
    
    // Heartbeat
    IsPing(data []byte) bool
    CreatePong(pingData []byte) []byte
    
    // Message conversion
    ParseMessage(data []byte) (*messaging.Message, error)
}
```

**Проверка результата**:
```bash
$ go test ./internal/core/exchange -v
```

---

# PHASE 2: Обмен и WebSocket

## 2.1 Binance Driver

**Файл**: `internal/core/exchange/drivers/binance/driver.go`

**Цель**: реализовать драйвер для Binance

**Ключевые моменты**:
- Spot: `wss://stream.binance.com:9443/ws`
- Futures: `wss://fstream.binance.com/ws`
- Heartbeat: мы отправляем ping, ждем pong
- Message format: WebSocket events в JSON

**Содержание**:
```go
package binance

type Driver struct {
    exchangeID string
    name       string
}

func (d *Driver) GetExchangeID() string {
    return "binance"
}

func (d *Driver) GetSpotWSEndpoint() string {
    return "wss://stream.binance.com:9443/ws"
}

func (d *Driver) GetFuturesWSEndpoint() string {
    return "wss://fstream.binance.com/ws"
}

func (d *Driver) CreateSubscribeMessage(pairs []string, marketType string, depth int) ([]byte, error) {
    // Convert pairs to Binance format: BTC/USDT -> btcusdt
    // depth: 20 -> @depth20, 50 -> @depth50, 0 -> @depth
    // Return: {"method":"SUBSCRIBE","params":["..."],"id":1}
}

func (d *Driver) IsPing(data []byte) bool {
    // Check if message is {"ping":"some_value"}
}

func (d *Driver) CreatePong(pingData []byte) []byte {
    // Create {"pong":"same_value"}
}

func (d *Driver) ParseMessage(data []byte) (*messaging.Message, error) {
    // Parse Binance message and convert to unified format
}
```

**Проверка результата**:
```bash
$ go test ./internal/core/exchange/drivers/binance -v
```

---

## 2.2 Остальные драйверы (Bybit, OKX, Kucoin, etc.)

**Файлы**: `internal/core/exchange/drivers/{bybit,okx,kucoin,coinex,htx,mexc,dex}/driver.go`

**Цель**: реализовать для каждой биржи

**План**:
1. Изучить документацию биржи (endpoints, message format, heartbeat)
2. Реализовать интерфейс `Driver`
3. Написать юнит-тесты с примерами реальных сообщений

**Проверка результата**:
```bash
$ go build ./internal/core/exchange/drivers/...
```

---

## 2.3 Exchange Factory

**Файл**: `internal/core/exchange/factory.go`

**Цель**: создавать драйверы нужной биржи по ID

**Содержание**:
```go
package exchange

func NewDriver(exchangeID string) (Driver, error) {
    switch exchangeID {
    case Binance:
        return binance.New()
    case Bybit:
        return bybit.New()
    case OKX:
        return okx.New()
    // ...
    default:
        return nil, fmt.Errorf("unknown exchange: %s", exchangeID)
    }
}
```

**Проверка результата**:
```bash
$ go test ./internal/core/exchange -v
```

---

## 2.4 WebSocket Connection

**Файл**: `internal/core/ws/connection.go`

**Цель**: управление одним WebSocket соединением

**Содержание**:
```go
package ws

type Connection struct {
    url        string
    exchangeID string
    marketType string
    driver     exchange.Driver
    
    conn       *websocket.Conn
    ctx        context.Context
    cancel     context.CancelFunc
    
    msgChan    chan *messaging.Message
    errChan    chan error
    
    subscriptions map[string]bool  // pair -> subscribed
    
    mu         sync.RWMutex
}

func (c *Connection) Connect() error {
    // Dial WebSocket
    // Start read loop
    // Start heartbeat loop
}

func (c *Connection) Subscribe(pairs []string, depth int) error {
    // Create subscribe message
    // Send to WS
    // Update subscriptions map
}

func (c *Connection) Unsubscribe(pairs []string) error {
    // Create unsubscribe message
    // Send to WS
    // Remove from subscriptions map
}

func (c *Connection) MessageChan() <-chan *messaging.Message {
    return c.msgChan
}

func (c *Connection) Close() error {
    // Cancel context
    // Close WebSocket
    // Close channels
}

// Private methods
func (c *Connection) readLoop() {
    // Read from WebSocket
    // Check for ping
    // Send pong if needed
    // Parse message
    // Send to msgChan
}

func (c *Connection) heartbeatLoop() {
    // Periodic ping (5-10 sec)
    // Detect timeout
    // Signal reconnect needed
}
```

**Проверка результата**:
```bash
$ go test ./internal/core/ws -v
```

---

## 2.5 WebSocket Pool Manager

**Файл**: `internal/core/ws/pool.go`

**Цель**: управлять пулом WebSocket соединений (30-50 пар на соединение)

**Содержание**:
```go
package ws

type Pool struct {
    connections map[string]*Connection  // key: "binance:spot", "binance:futures"
    
    driverFactory exchange.DriverFactory
    maxPairsPerConn int  // 30-50
    
    msgRouter   chan *messaging.Message
    
    mu          sync.RWMutex
    ctx         context.Context
    cancel      context.CancelFunc
    wg          sync.WaitGroup
}

func (p *Pool) Subscribe(exchangeID, marketType string, pairs []string) error {
    // Find or create connection for exchange+marketType
    // If existing connection is not full:
    //   subscribe pairs there
    // Else:
    //   create new connection
    //   subscribe pairs there
}

func (p *Pool) Unsubscribe(exchangeID, marketType string, pairs []string) error {
    // Find connection
    // Unsubscribe pairs
    // If connection has no more subscriptions:
    //   close connection
}

func (p *Pool) GetSubscriptions(exchangeID, marketType string) []string {
    // Return list of subscribed pairs
}

func (p *Pool) Start() error {
    // Start routing messages
    // For each connection:
    //   start reading
}

func (p *Pool) Stop() error {
    // Close all connections
    // Wait for goroutines
}
```

**Проверка результата**:
```bash
$ go test ./internal/core/ws -v
```

---

# PHASE 3: Order Book и Pub/Sub система

## 3.1 Order Book Manager

**Файл**: `internal/core/orderbook/manager.go`

**Цель**: управлять множественными order books

**Содержание**:
```go
package orderbook

type Manager struct {
    books    map[string]*OrderBook  // key: "binance:spot:BTC/USDT"
    
    subscribers map[string][]pubsub.Subscriber  // pair -> subscribers
    
    mu       sync.RWMutex
}

func (m *Manager) UpdateOrderBook(msg *messaging.Message) error {
    // Get or create OrderBook
    // Update with new data
    // Notify subscribers
}

func (m *Manager) GetOrderBook(exchangeID, pair, marketType string) *exchange.OrderBook {
    // Return current orderbook (copy)
}

func (m *Manager) Subscribe(subscriber pubsub.Subscriber, exchangeID, pair, marketType string) {
    // Add subscriber to list
}

func (m *Manager) Unsubscribe(subscriber pubsub.Subscriber, exchangeID, pair, marketType string) {
    // Remove subscriber from list
}

// Приватные методы
func (m *Manager) notifySubscribers(exchangeID, pair, marketType string, msg *messaging.Message) {
    // Iterate subscribers
    // Call OnMessage for each
}
```

**Проверка результата**:
```bash
$ go test ./internal/core/orderbook -v
```

---

## 3.2 Ring Buffer (для Monitor)

**Файл**: `internal/core/orderbook/ringbuffer.go`

**Цель**: циклический буфер для хранения истории обновлений

**Содержание**:
```go
package orderbook

type RingBuffer struct {
    entries    []*RingBufferEntry
    head       int
    size       int
    capacity   int
    mu         sync.RWMutex
}

type RingBufferEntry struct {
    Timestamp  int64
    ExchangeID string
    Pair       string
    OrderBook  *exchange.OrderBook
}

func NewRingBuffer(capacity int) *RingBuffer {
    return &RingBuffer{
        entries:  make([]*RingBufferEntry, capacity),
        capacity: capacity,
    }
}

func (rb *RingBuffer) Add(entry *RingBufferEntry) {
    rb.mu.Lock()
    defer rb.mu.Unlock()
    
    rb.entries[rb.head] = entry
    rb.head = (rb.head + 1) % rb.capacity
    if rb.size < rb.capacity {
        rb.size++
    }
}

func (rb *RingBuffer) GetAll() []*RingBufferEntry {
    rb.mu.RLock()
    defer rb.mu.RUnlock()
    
    result := make([]*RingBufferEntry, rb.size)
    for i := 0; i < rb.size; i++ {
        result[i] = rb.entries[(rb.head+i)%rb.capacity]
    }
    return result
}

func (rb *RingBuffer) Flush() []*RingBufferEntry {
    rb.mu.Lock()
    defer rb.mu.Unlock()
    
    result := rb.GetAll()
    rb.head = 0
    rb.size = 0
    return result
}
```

**Проверка результата**:
```bash
$ go test ./internal/core/orderbook -v
```

---

## 3.3 Pub/Sub Subscriber Interface

**Файл**: `internal/core/pubsub/subscriber.go`

**Цель**: интерфейс для подписчиков

**Содержание**:
```go
package pubsub

type Subscriber interface {
    GetID() string
    OnMessage(msg *messaging.Message)
    OnError(err error)
}
```

**Проверка результата**:
```bash
$ go test ./internal/core/pubsub -v
```

---

# PHASE 4: Task Management и Subscription

## 4.1 Task Fetcher из MySQL

**Файл**: `internal/task/fetcher.go`

**Цель**: периодически загружать задачи из MySQL

**Содержание**:
```go
package task

type Fetcher struct {
    db          *sql.DB
    interval    time.Duration
    
    lastMonitoring []exchange.MonitoringTask
    lastTrading    []exchange.TradingTask
    
    ctx         context.Context
    cancel      context.CancelFunc
    wg          sync.WaitGroup
    
    mu          sync.RWMutex
}

func (f *Fetcher) Start() error {
    // Spawn goroutine
    // Tick every `interval`
    // Call fetch()
}

func (f *Fetcher) Fetch() (*TasksData, error) {
    // Query MONITORING table
    // Query TRADE table
    // Update lastMonitoring, lastTrading
    // Return combined data
}

func (f *Fetcher) GetLast() *TasksData {
    // Return last fetched data
}

type TasksData struct {
    Timestamp      int64
    MonitoringTasks []exchange.MonitoringTask
    TradingTasks   []exchange.TradingTask
}
```

**SQL Queries**:
```sql
SELECT EXCHANGE_ID, EXCHANGE_NAME, MARKET_TYPE, TRADE_PAIR_ID, TRADE_PAIR, ORDERBOOK_DEPTH
FROM MONITORING
WHERE ENABLED = 1
ORDER BY DAEMON_PRIORITY DESC;

SELECT EXCHANGE_ID, EXCHANGE_NAME, MARKET_TYPE, TRADE_PAIR_ID, TRADE_PAIR, STRATEGY_ID, STRATEGY_PARAMS
FROM TRADE
WHERE ENABLED = 1
ORDER BY DAEMON_PRIORITY DESC;
```

**Проверка результата**:
```bash
$ go test ./internal/task -v
```

---

## 4.2 Subscription Manager

**Файл**: `internal/task/subscription_manager.go`

**Цель**: сравнить новые задачи с предыдущими, вычислить дельту

**Содержание**:
```go
package task

type SubscriptionManager struct {
    lastState *TasksData
    wsPool    ws.Pool
    
    mu        sync.RWMutex
}

type SubscriptionDiff struct {
    ToSubscribe   []Subscription
    Unsubscribe   []Subscription
}

type Subscription struct {
    ExchangeID string
    MarketType string
    Pairs      []string
    Depth      int
}

func (sm *SubscriptionManager) Merge(newTasks *TasksData) (*SubscriptionDiff, error) {
    // Build map of "exchange:markettype:pair" -> depth from newTasks
    // Build same map from lastState
    // Compare:
    //   - New pairs: add to ToSubscribe
    //   - Removed pairs: add to Unsubscribe
    //   - Changed depth: unsubscribe old, subscribe new
    // Return diff
}

func (sm *SubscriptionManager) ApplyDiff(diff *SubscriptionDiff) error {
    // For each subscription in ToSubscribe:
    //   wsPool.Subscribe(...)
    // For each subscription in Unsubscribe:
    //   wsPool.Unsubscribe(...)
}
```

**Проверка результата**:
```bash
$ go test ./internal/task -v
```

---

# PHASE 5: Monitor Role

## 5.1 Monitor Main Component

**Файл**: `internal/monitor/monitor.go`

**Цель**: главный контроллер мониторинга

**Содержание**:
```go
package monitor

type Monitor struct {
    id              string
    cfg             *config.MonitorConfig
    
    obManager       *orderbook.Manager
    ringBuffer      *orderbook.RingBuffer
    chClient        *clickhouse.Client
    
    ctx             context.Context
    cancel          context.CancelFunc
    wg              sync.WaitGroup
}

func (m *Monitor) Start(ctx context.Context) error {
    // Subscribe to orderbook updates
    // Start event handler loop
}

func (m *Monitor) OnMessage(msg *messaging.Message) {
    // Add to ring buffer
}

func (m *Monitor) Stop() error {
    // Unsubscribe from orderbook
    // Flush remaining data to ClickHouse
}

func (m *Monitor) GetID() string {
    return m.id
}
```

**Проверка результата**:
```bash
$ go test ./internal/monitor -v
```

---

## 5.2 ClickHouse Client

**Файл**: `internal/monitor/clickhouse/client.go`

**Цель**: писать данные в ClickHouse

**Содержание**:
```go
package clickhouse

type Client struct {
    conn    *sql.DB
    cfg     config.ClickHouseConfig
}

func (c *Client) WriteOrderBookDeltas(deltas []OrderBookDelta) error {
    // INSERT into orderbook_deltas
}

func (c *Client) WriteOrderBookSnapshot(snapshot OrderBookSnapshot) error {
    // INSERT into orderbook_snapshots
}

type OrderBookDelta struct {
    Timestamp   int64
    ExchangeID  string
    Pair        string
    Side        string  // "bid", "ask"
    Price       float64
    Amount      float64
    Action      string  // "update", "delete"
}

type OrderBookSnapshot struct {
    Timestamp   int64
    ExchangeID  string
    Pair        string
    Bids        [][2]float64
    Asks        [][2]float64
    SeqNum      int64
}
```

**Проверка результата**:
```bash
$ go test ./internal/monitor/clickhouse -v
```

---

## 5.3 ClickHouse Schema

**Файл**: `internal/monitor/clickhouse/schema.sql`

**Содержание**:
```sql
CREATE TABLE IF NOT EXISTS orderbook_deltas (
    timestamp DateTime,
    exchange_id String,
    pair String,
    market_type String,
    side String,
    price Float64,
    amount Float64,
    action String
) ENGINE = MergeTree()
ORDER BY (timestamp, exchange_id, pair);

CREATE TABLE IF NOT EXISTS orderbook_snapshots (
    timestamp DateTime,
    exchange_id String,
    pair String,
    market_type String,
    bids Array(Tuple(Float64, Float64)),
    asks Array(Tuple(Float64, Float64)),
    sequence_num Int64
) ENGINE = MergeTree()
ORDER BY (timestamp, exchange_id, pair);
```

**Проверка результата**:
```bash
$ clickhouse-client -q "SHOW TABLES FROM default LIKE 'orderbook%';"
```

---

# PHASE 6: Trader Role

## 6.1 Trader Main Component

**Файл**: `internal/trader/trader.go`

**Цель**: главный контроллер торговца

**Содержание**:
```go
package trader

type Trader struct {
    id              string
    cfg             *config.TraderConfig
    
    obManager       *orderbook.Manager
    portfolio       *Portfolio
    strategies      map[string]Strategy
    executor        *OrderExecutor
    
    ctx             context.Context
    cancel          context.CancelFunc
    wg              sync.WaitGroup
}

func (t *Trader) Start(ctx context.Context) error {
    // Load portfolios
    // Subscribe to orderbook updates
    // Start event handler loop
    // Start private WS listener
}

func (t *Trader) OnMessage(msg *messaging.Message) {
    // Check message type
    if msg.Type == messaging.TypeOrderBook {
        // Evaluate strategy
        // Execute if needed
    } else if msg.Type == messaging.TypeOrder {
        // Update portfolio
    }
}

func (t *Trader) Stop() error {
    // Close all positions (если нужно)
    // Save state
}

func (t *Trader) GetID() string {
    return t.id
}
```

**Проверка результата**:
```bash
$ go test ./internal/trader -v
```

---

## 6.2 Portfolio Management

**Файл**: `internal/trader/portfolio.go`

**Цель**: управление позициями и балансом

**Содержание**:
```go
package trader

type Portfolio struct {
    exchangeID string
    balances   map[string]float64  // asset -> amount
    positions  map[string]*Position  // pair -> position
    
    mu         sync.RWMutex
}

type Position struct {
    Pair      string
    Side      string  // "long", "short"
    Amount    float64
    EntryPrice float64
    CurrentPrice float64
    PnL       float64
}

func (p *Portfolio) GetBalance(asset string) float64 {
    p.mu.RLock()
    defer p.mu.RUnlock()
    return p.balances[asset]
}

func (p *Portfolio) UpdatePosition(pair string, pos *Position) {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.positions[pair] = pos
}

func (p *Portfolio) GetPosition(pair string) *Position {
    p.mu.RLock()
    defer p.mu.RUnlock()
    return p.positions[pair]
}
```

**Проверка результата**:
```bash
$ go test ./internal/trader -v
```

---

## 6.3 Strategy Interface

**Файл**: `internal/trader/strategies/strategy.go`

**Цель**: интерфейс для стратегий

**Содержание**:
```go
package strategies

type Strategy interface {
    GetID() string
    GetPair() string
    
    // Evaluate market and return action
    Evaluate(ob *exchange.OrderBook, portfolio *trader.Portfolio) *TradeAction
    
    // Called after order execution
    OnExecuted(order *Order)
}

type TradeAction struct {
    Type   string      // "buy", "sell", "close", "none"
    Price  float64
    Amount float64
    Reason string
}

type Order struct {
    OrderID    string
    Pair       string
    Side       string
    Price      float64
    Amount     float64
    Status     string
    ExecutedAt int64
}
```

**Проверка результата**:
```bash
$ go test ./internal/trader/strategies -v
```

---

## 6.4 Grid Strategy (пример)

**Файл**: `internal/trader/strategies/grid/grid.go`

**Цель**: реализовать grid стратегию как пример

**Ключевые параметры**:
- `grid_step`: размер сетки (%)
- `order_size`: размер одного ордера
- `layers`: количество слоев сверху и снизу

**Проверка результата**:
```bash
$ go test ./internal/trader/strategies/grid -v
```

---

## 6.5 Order Executor

**Файл**: `internal/trader/executor.go`

**Цель**: отправлять ордера на биржу через REST API

**Содержание**:
```go
package trader

type OrderExecutor struct {
    exchangeID string
    apiKey     string
    apiSecret  string
    
    // REST client
}

func (e *OrderExecutor) PlaceOrder(pair string, side string, price float64, amount float64) (*Order, error) {
    // Create order
    // Send REST request to exchange
    // Return order details
}

func (e *OrderExecutor) CancelOrder(orderID string) error {
    // Send cancel request
}

func (e *OrderExecutor) GetOpenOrders(pair string) ([]*Order, error) {
    // Query open orders
}
```

**Проверка результата**:
```bash
$ go test ./internal/trader -v
```

---

# PHASE 7: Интеграция и Manager

## 7.1 Main Manager Update

**Файл**: `internal/manager/manager.go`

**Цель**: обновить Manager для новой архитектуры

**Содержание**:
```go
type Manager struct {
    cfg         *config.Config
    role        string  // "monitor", "trader", "both"
    
    // Core components
    wsPool      *ws.Pool
    obManager   *orderbook.Manager
    msgRouter   *pubsub.Router
    
    // Tasks
    fetcher     *task.Fetcher
    subMgr      *task.SubscriptionManager
    
    // Role-specific
    monitor     *monitor.Monitor
    trader      *trader.Trader
    
    ctx         context.Context
    cancel      context.CancelFunc
    wg          sync.WaitGroup
}

func (m *Manager) Start() error {
    // Start WS Pool
    // Start Task Fetcher
    // Start Subscription Manager
    // If monitor role: start Monitor
    // If trader role: start Trader
}

func (m *Manager) Stop() error {
    // Stop Trader
    // Stop Monitor
    // Stop all WS connections
    // Save state
}
```

**Проверка результата**:
```bash
$ go build ./cmd/daemon/
```

---

## 7.2 API Updates

**Файл**: `internal/api/server.go`

**Цель**: добавить endpoints для управления ролями

**Endpoints**:
- `GET /status` - статус демона
- `POST /monitor/tasks` - загруженные задачи мониторинга
- `POST /trader/orders` - открытые ордера
- `GET /orderbook/:exchange/:pair` - текущий orderbook

**Проверка результата**:
```bash
$ curl http://localhost:8080/status
```

---

# PHASE 8: Тестирование и Production Hardening

## 8.1 Unit Tests

**Цель**: покрыть unit тестами все компоненты

**Минимум**:
- [ ] core/exchange/drivers - тесты парсинга сообщений
- [ ] core/orderbook - тесты обновления и sub/unsub
- [ ] core/ws - тесты подключения/переподключения
- [ ] task - тесты merge логики
- [ ] monitor - тесты буферизации
- [ ] trader - тесты стратегий

**Проверка результата**:
```bash
$ go test ./... -v -cover
```

---

## 8.2 Integration Tests

**Цель**: тесты взаимодействия компонентов

**Примеры**:
- Подписка → получение сообщения → обновление OB → уведомление подписчиков
- Загрузка задач → merge → подписка на WS
- Обновление OB → стратегия → выполнение ордера

**Проверка результата**:
```bash
$ go test ./... -tags=integration -v
```

---

## 8.3 Load Testing

**Цель**: проверить производительность

**Сценарии**:
- 1000 пар на разных биржах
- 100 обновлений orderbook в секунду
- Запись 10K событий в ClickHouse в секунду

**Проверка результата**:
```bash
$ go test -bench=. -benchmem
```

---

## 8.4 Stability & Recovery

**Цель**: проверить восстановление при сбоях

**Сценарии**:
- [ ] Обрыв WS соединения → автоматическое переподключение
- [ ] MySQL недоступна → буферизация, повторные попытки
- [ ] ClickHouse недоступна → буферизация
- [ ] OOM → graceful shutdown

**Проверка результата**:
```bash
$ # Manual testing с отключением сервисов
```

---

## 8.5 Documentation

**Файлы**:
- [ ] README.md - как запустить, конфигурация
- [ ] API.md - описание endpoints
- [ ] MONITORING.md - как настроить мониторинг
- [ ] TRADING.md - как создать свою стратегию

**Проверка результата**:
```bash
$ ls *.md
```

---

# Чеклист успешности разработки

## Функциональные требования
- [ ] Поддержка 7 CEX (Binance, Bybit, OKX, Kucoin, Coinex, HTX, MEXC)
- [ ] Поддержка Spot и Futures на каждой бирже
- [ ] Работа на множественных демонах с одной БД
- [ ] Monitor собирает полную историю в ClickHouse
- [ ] Trader торгует согласно стратегиям
- [ ] Автоматическое восстановление соединений
- [ ] Graceful shutdown

## Non-Functional Requirements
- [ ] Latency orderbook processing < 100ms
- [ ] Throughput 1000-5000 msg/sec
- [ ] Поддержка 300-500 пар на демон
- [ ] Max 20 WS соединений на демон
- [ ] Memory usage < 2GB
- [ ] 99.9% uptime (по возможности)

## Code Quality
- [ ] Go code follows best practices
- [ ] Error handling everywhere
- [ ] Proper logging levels
- [ ] Comments for complex logic
- [ ] Test coverage > 80%

---

# Timeline

- Week 1-2: Phase 1-2 (Foundation + Exchange)
- Week 3: Phase 3 (OrderBook + Pub/Sub)
- Week 4: Phase 4 (Task Management)
- Week 5: Phase 5 (Monitor)
- Week 6-7: Phase 6 (Trader)
- Week 8: Phase 7 (Integration)
- Week 9-10: Phase 8 (Testing + Hardening)

**Total: 10 weeks** для MVP версии

