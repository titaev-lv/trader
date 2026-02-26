# 🚀 Trader — План разработки

> **Версия**: 1.4  
> **Дата актуализации**: 2026-02-20  
> **Статус**: Canonical plan (заменяет `DEVELOPMENT_PLAN_1.md`)

---

## 📋 Актуальный статус (2026-02-20)

### ✅ Выполнено

- ✅ Phase 1 фундамент: структура, типы, конфигурация
- ✅ Консолидация документации: один canonical `DEVELOPMENT_PLAN.md`
- ✅ Logging unification в коде: `error.log` + `out_request.log` + `ws_in.log` + `ws_out.log` + `audit.log`
- ✅ `stdout + file` для всех stream'ов через `io.MultiWriter`
- ✅ JSON logging + rotation на `lumberjack`
- ✅ Trader работает в outbound-only модели (локальный HTTP API сервер удален)
- ✅ WS correlation layer: `event_id` + `request_id` mapping между `ws_out` и `ws_in`
- ✅ Добавлен TTL 24h и периодическая очистка map корреляций `event_id -> request_id`

### ✅ Закрыто / актуальный статус

- ✅ Logging migration в CT-SYSTEM завершена: runtime-валидация в compose пройдена (docker logs + файловые потоки)
- ✅ Integration wiring для контейнерного запуска Trader в составе CT-SYSTEM финализирован
- ✅ Integration test-процедуры Trader синхронизированы с root `TESTING.md`

### ℹ️ Что подтверждено в коде и runtime

- Унификация логирования реализована в коде (`slog`, `lumberjack`, `stdout + file`, `out_request`, `ws_in`, `ws_out`, `audit`)
- End-to-end smoke проверка в составе CT-SYSTEM выполнена: startup JSON logs и file streams подтверждены

### ℹ️ Примечание по документам

- Этот файл является **единственным актуальным планом развития Trader**.
- Исторический файл `DEVELOPMENT_PLAN_1.md` выведен из использования.

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

**Статус**: ✅ ВЫПОЛНЕНО

**Задачи**:
- [x] Создать папки: `internal/core/`, `internal/task/`, `internal/monitor/`, `internal/trader/`
- [x] Создать подпапки в `internal/core/`:
  - `exchange/` и `exchange/drivers/`
  - `orderbook/`
  - `messaging/` и `messaging/converters/`
  - `ws/`
  - `pubsub/`
- [x] Создать `internal/exchange/drivers/` с подпапками для каждой биржи:
  - `binance/`, `bybit/`, `okx/`, `kucoin/`, `coinex/`, `htx/`, `mexc/`, `dex/`

**Проверка результата**:
```bash
$ find internal/core -type d | head -20
internal
internal/core
internal/core/exchange
internal/core/exchange/drivers
internal/core/exchange/drivers/binance
internal/core/exchange/drivers/bybit
internal/core/exchange/drivers/coinex
internal/core/exchange/drivers/dex
internal/core/exchange/drivers/htx
internal/core/exchange/drivers/kucoin
internal/core/exchange/drivers/mexc
internal/core/exchange/drivers/okx
internal/core/messaging
internal/core/messaging/converters
internal/core/orderbook
internal/core/pubsub
internal/core/ws

$ find internal/core/exchange/drivers -type d | wc -l  # должно быть 8+
9  # ✅ 8 бирж + 1 root папка = 9
```

**Дополнительно созданы папки**:
- `internal/task/` - для управления задачами
- `internal/monitor/` - для Monitor роли
- `internal/trader/` - для Trader роли

---

## 1.2 Определение базовых типов

**Файл**: `internal/core/exchange/types.go`

**Цель**: все общие типы в одном месте

**Статус**: ✅ ВЫПОЛНЕНО

**Содержание**:

### Константы:
- **Exchange IDs**: Binance, Bybit, OKX, Kucoin, Coinex, HTX, MEXC, DEX
- **Market Types**: MarketSpot, MarketFutures

### Основные типы:

#### Level
```go
type Level struct {
    Price  float64  // Цена за единицу (например, 45123.56 USDT)
    Amount float64  // Объем на этой цене (0 = уровень удален)
}
```
**Использование**: Один уровень в книге ордеров

#### OrderBook
```go
type OrderBook struct {
    ExchangeID string  // Какая биржа (binance, bybit и т.д.)
    Pair       string  // Торговая пара (BTC/USDT)
    MarketType string  // Тип рынка (spot или futures)
    Bids       []Level // Уровни покупателей (отсортированы по цене вниз)
    Asks       []Level // Уровни продавцов (отсортированы по цене вверх)
    Depth      int     // Глубина: 20, 50 или 0 (full)
    Timestamp  int64   // Unix миллисекунды
    SeqNum     int64   // Последовательный номер от биржи
}
```
**Использование**: Хранит текущую книгу ордеров для пары на бирже

#### MonitoringTask
```go
type MonitoringTask struct {
    ExchangeID   string // Какую биржу мониторить
    ExchangeName string // Человеческое название
    MarketType   string // spot или futures
    TradePairID  int    // ID в нашей БД
    TradePair    string // BTC/USDT и т.д.
}
```
**Использование**: Описывает что мониторить (получается из CTS-Core по WS task flow)

#### TradingTask
```go
type TradingTask struct {
    ExchangeID     string                 // Какую биржу торговать
    ExchangeName   string                 // Человеческое название
    MarketType     string                 // spot или futures
    TradePairID    int                    // ID в нашей БД
    TradePair      string                 // BTC/USDT и т.д.
    StrategyID     string                 // grid, dca, momentum и т.д.
    StrategyParams map[string]interface{} // Параметры стратегии в JSON формате
}
```
**Использование**: Описывает что торговать и какой стратегией (получается из CTS-Core по WS task flow)

#### TasksData
```go
type TasksData struct {
    Timestamp       int64
    MonitoringTasks []MonitoringTask // Пары для мониторинга
    TradingTasks    []TradingTask    // Пары для торговли
}
```
**Использование**: Объединение всех задач из потока CTS-Core (каждые 5-10 сек)

### Вспомогательные функции:
- `GetOrderBookKey(exchangeID, pair, marketType)` - уникальный ключ для orderbook
- `GetMonitoringTaskKey(task)` - уникальный ключ для мониторинга
- `GetTradingTaskKey(task)` - уникальный ключ для торговли

**Проверка результата**:
```bash
$ go build ./internal/core/exchange
✓ Успешная компиляция
```

---

## 1.3 Unified Message Format

**Файл**: `internal/core/messaging/message.go`

**Цель**: единый формат сообщений от всех бирж

**Статус**: ✅ ВЫПОЛНЕНО

**Содержание**:

### Константы типов сообщений:
```go
const (
    TypeOrderBook = "orderbook" // Обновление книги ордеров
    TypeTrade     = "trade"     // Новая сделка на бирже
    TypePosition  = "position"  // Обновление моей позиции (приватное)
    TypeOrder     = "order"     // Обновление статуса моего ордера (приватное)
)
```

### Основные типы:

#### Message (главная структура)
```go
type Message struct {
    Timestamp  int64              // Unix миллисекунды (стандартизовано)
    ExchangeID string             // binance, bybit, okx и т.д.
    MarketType string             // spot или futures
    Type       string             // orderbook, trade, position, order
    Pair       string             // BTC/USDT
    SeqNum     int64              // Порядковый номер от биржи
    
    // Только одно из полей ниже заполнено (зависит от Type)
    OrderBook  *OrderBookData
    Trade      *TradeData
    Position   *PositionData
    Order      *OrderData
}
```
**Использование**: Универсальный формат для всех сообщений от бирж

#### OrderBookData
```go
type OrderBookData struct {
    Bids  []Level // Best Bid первый
    Asks  []Level // Best Ask первый
    Depth int     // 20, 50 или 0 (full)
}
```
**Использование**: Когда Type == TypeOrderBook

#### TradeData
```go
type TradeData struct {
    Price  float64 // Цена сделки
    Amount float64 // Объем сделки
    Side   string  // "buy" или "sell" (инициатор сделки)
}
```
**Использование**: Когда Type == TypeTrade (реальные сделки на бирже)

#### PositionData
```go
type PositionData struct {
    Side         string  // "long" или "short"
    Amount       float64 // Объем позиции
    EntryPrice   float64 // Цена входа
    CurrentPrice float64 // Текущая цена
    PnL          float64 // Прибыль/убыток
}
```
**Использование**: Когда Type == TypePosition (приватные данные трейдера)

#### OrderData
```go
type OrderData struct {
    OrderID    string  // ID ордера на бирже
    Side       string  // "buy" или "sell"
    Price      float64 // Цена
    Amount     float64 // Всего объем
    Filled     float64 // Исполнено объема
    Status     string  // open, filled, partially_filled, cancelled, rejected
    Commission float64 // Комиссия биржи
}
```
**Использование**: Когда Type == TypeOrder (исполнение моих ордеров)

### Вспомогательная функция:
- `GetMessageKey(msg)` - уникальный ключ для логирования/дедупликации

**Преимущества единого формата**:
- ✅ Одинаковый код для обработки данных со всех бирж
- ✅ Легко добавлять новые биржи (только converter нужен)
- ✅ Простая маршрутизация в Monitor/Trader
- ✅ Типобезопасность и валидация

**Проверка результата**:
```bash
$ go build ./internal/core/messaging
✓ Успешная компиляция
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
$ go build ./cmd/trader/
```

---

## 1.5 CTS-Core Task Mapping

**Статус**: ✅ ВЫПОЛНЕНО (task flow согласован с CTS-Core)

**Файл**: `services/cts-core/API_SPECIFICATION.md`

**Цель**: Зафиксировать mapping payload'ов задач CTS-Core на Go типы

**Ключевые таблицы** (уже созданы):

| Таблица | Назначение | Records |
|---------|-----------|---------|
| **ARBITRAGE_TRANS** | PRIMARY: транзакции арбитража (основная функция) | 79 |
| **TRADE** | Конфигурация торговых стратегий | 8 |
| **MONITORING** | Конфигурация мониторинга стаканов | 7 |
| **TRADE_PAIR** | Каталог торговых пар (SPOT + FUTURES) | 1.3M+ |
| **TRADE_PAIRS** | Junction: TRADE → TRADE_PAIR → EXCHANGE_ACCOUNTS | ? |
| **MONITORING_TRADE_PAIRS** | Junction: MONITORING → TRADE_PAIR | ? |
| **TRADE_HISTORY** | История выполнения ордеров (Phase 1.4 новая) | 0 |
| **DAEMON_STATE** | Состояние демона (Phase 1.4 новая) | 0 |
| **USER, EXCHANGE, COIN, CHAIN** | Справочники | 300+ |

**Что уже присутствует в TRADE:**
```sql
-- Все эти колонки УЖЕ ЕСТЬ в production БД:
MAX_AMOUNT_TRADE DECIMAL(30,12)
MAX_OPEN_ORDERS INT DEFAULT 10
MAX_POSITION_SIZE DECIMAL(30,12)
STRATEGY_UPDATE_INTERVAL_SEC INT DEFAULT 300
SLIPPAGE_PERCENT DECIMAL(10,6) DEFAULT 0.1
ENABLE_BACKTEST TINYINT(1) DEFAULT 0
FIN_PROTECTION TINYINT(1) DEFAULT 0
BBO_ONLY TINYINT(1) DEFAULT 1
```

**Что уже присутствует в MONITORING:**
```sql
-- Все эти колонки УЖЕ ЕСТЬ в production БД:
ORDERBOOK_DEPTH INT DEFAULT 50
BATCH_SIZE INT DEFAULT 1000
BATCH_INTERVAL_SEC INT DEFAULT 300
RING_BUFFER_SIZE INT DEFAULT 10000
SAVE_INTERVAL_SEC INT DEFAULT 600
ACTIVE TINYINT(1) DEFAULT 1
```

**Что требует реализации**:
1. **Task Fetcher** (`internal/task/fetcher.go`):
   - Загружать MONITORING конфиги (7 записей)
   - Загружать TRADE конфиги (8 записей)
   - Преобразовать в MonitoringTask и TradingTask

2. **Subscription Manager** (`internal/task/subscription_manager.go`):
   - Сравнивать загруженные конфиги с текущими subscriptions
   - Выполнять подписку/отписку пар через WS Pool

3. **ARBITRAGE_TRANS Handler** (`internal/trader/arbitrage.go`):
   - Монитор на новые ARBITRAGE_TRANS записи
   - Преобразование в orders через Order Executor
   - Обновление STATUS (New → In Progress → Complete/Error)

4. **TRADE_HISTORY Logger** (`internal/trader/executor.go`):
   - Логирование каждого ордера в TRADE_HISTORY
   - Заполнение: EXECUTED_AT (microseconds), COMMISSION, STATUS

5. **DAEMON_STATE Tracker** (`internal/manager/daemon_state.go`):
   - Запись heartbeat каждые 5 сек (LAST_HEARTBEAT)
   - Мониторинг для recovery логики
   - Graceful shutdown: STATUS → STOPPING/STOPPED

**Проверка результата**:
```bash
$ # В production уже есть 27 таблиц, проверяем только наши типы Go:
$ go build ./internal/task
$ go build ./internal/trader
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

## 4.1 Task Fetcher из CTS-Core

**Файл**: `internal/task/fetcher.go`

**Цель**: периодически получать задачи из CTS-Core (WS events)

**Содержание**:
```go
package task

type Fetcher struct {
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
    // Read task events from CTS-Core stream
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
$ go build ./cmd/trader/
```

---

## 7.2 API Updates

**Статус**: ❌ Не применяется в актуальной архитектуре

Trader работает в **outbound-only** модели и не поднимает локальный HTTP API.
Управление задачами и контроль состояния выполняются через WS/REST взаимодействие с CTS-Core.

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
- [ ] CTS-Core недоступен → буферизация, повторные попытки
- [ ] ClickHouse недоступна → буферизация
- [ ] OOM → graceful shutdown

**Проверка результата**:
```bash
$ # Manual testing с отключением сервисов
```

---

## 8.5 Documentation

**Файлы**:
- [x] README.md - как запустить, конфигурация
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
- [ ] Работа на множественных trader-инстансах с оркестрацией через CTS-Core
- [ ] Monitor собирает полную историю в ClickHouse
- [ ] Trader торгует согласно стратегиям
- [ ] Автоматическое восстановление соединений
- [ ] Graceful shutdown

## Non-Functional Requirements
- [ ] Latency orderbook processing < 100ms
- [ ] Throughput 1000-5000 msg/sec
- [ ] Поддержка 300-500 пар на trader-инстанс
- [ ] Max 20 WS соединений на trader-инстанс
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

