# Phase 1.3 - Расширенные конфигурационные структуры (Детальное описание)

## 📋 Обзор Phase 1.3

**Цель**: Расширить конфигурационную систему для поддержки двух ролей Trader и подключения к ClickHouse.

**Статус**: ✅ ЗАВЕРШЕНО

**Файлы**:
- `internal/config/config.go` - расширенная структура конфигурации
- `conf/config.example.yaml` - пример полной конфигурации с комментариями

---

## 🏗️ Архитектурные изменения

### Двойственность ролей

Trader теперь может работать в одной из трех ролей:

```
┌─────────────────────────────────────────────────────┐
│ trader (единое приложение)                          │
└────────────┬──────────────────────────────┬─────────┘
             │                              │
     ┌───────▼────────┐           ┌─────────▼────────┐
     │ MONITOR Role   │           │ TRADER Role      │
     │                │           │                  │
     │ - Слушает WS   │           │ - Выполняет      │
     │ - Собирает OB  │           │   стратегии      │
     │ - Сохраняет в  │           │ - Исполняет      │
     │   ClickHouse   │           │   ордера         │
     │ - Аналитика    │           │ - Риск менеджмент│
     └────────────────┘           └──────────────────┘
```

**Преимущества**:
- Один сервис может быть и монитором и трейдером
- Монитор и Трейдер могут запускаться независимо
- Масштабируемость: можно иметь несколько мониторов и один трейдер
- Модульность: легко отключить непотребную функцию

---

## 🔧 Новые структуры в Config

### 1. Основная структура Config (изменения)

```go
type Config struct {
    Databases  DatabasesConfig     // Подключения к БД/хранилищам
    Server     ServerConfig        // Было раньше
    Log        LogConfig           // Было раньше
    Trade      TradeConfig         // Было раньше
    OrderBook  OrderBookConfig     // Было раньше
    
    // НОВОЕ в Phase 1.3:
    Role       string              // "monitor", "trader" или "both"
    Monitor    MonitorConfig       // Конфиг мониторинга
    Trader     TraderConfig        // Конфиг торговли
    // ClickHouse теперь в Databases.Quotes.ClickHouse
}
```

**Назначение каждого поля**:
- `Role` - определяет что делать сервису (мониторить, торговать или оба)
- `Monitor` - параметры сбора данных (глубина OB, batch size и т.д.)
- `Trader` - параметры торговли (max orders, стратегия, риск)
- `Databases.Quotes.ClickHouse` - где хранить исторические данные

### 2. MonitorConfig - Конфигурация мониторинга

```go
type MonitorConfig struct {
    // Какую глубину книги ордеров мониторить
    OrderBookDepth int  // 20, 50 или 0 (full)
    
    // Как собираются данные в batch для отправки в ClickHouse
    BatchSize      int  // Сколько обновлений в batch (500-1000)
    BatchIntervalSec int // Максимальное время между отправками (5-10s)
    
    // Кэш для быстрого доступа к последним данным
    RingBufferSize int  // Размер ring buffer (10000-50000)
    
    // Как часто сохранять данные
    SaveInterval   int  // Интервал сохранения (5-10s)
}
```

**Примеры использования**:

```yaml
# Для быстрого сбора данных (меньше задержки, больше памяти)
monitor:
    orderbook_depth: 50      # Глубокая книга
    batch_size: 1000         # Большой batch
    batch_interval: 2        # Часто отправляем
    ring_buffer_size: 50000  # Большой кэш
    save_interval: 2

# Для экономного сбора (меньше памяти, больше задержка)
monitor:
    orderbook_depth: 20      # Мелкая книга
    batch_size: 100          # Маленький batch
    batch_interval: 10       # Редко отправляем
    ring_buffer_size: 1000   # Маленький кэш
    save_interval: 10
```

**Алгоритм сбора данных**:

```
1. Monitor получает OrderBook update от биржи
2. Преобразует в standardized Message формат
3. Добавляет в ring buffer (быстрый доступ)
4. Добавляет в batch для ClickHouse
5. Если batch полный ИЛИ прошло BatchIntervalSec секунд:
   - Отправляет batch в ClickHouse
   - Очищает batch
6. Каждые SaveInterval секунд - проверяет что все сохранено
```

### 3. TraderConfig - Конфигурация торговли

```go
type TraderConfig struct {
    // Ограничения на количество ордеров
    MaxOpenOrders  int     // Максимум одновременно открытых ордеров
    
    // Ограничения на размер позиции (риск менеджмент)
    MaxPositionSize float64 // Максимум в USDT на одну позицию
    
    // Выбор стратегии
    DefaultStrategy string  // "grid", "dca", "scalp" и т.д.
    StrategyUpdateInterval int // Как часто переоценивать стратегию
    
    // Контроль качества исполнения
    SlippagePercent float64 // Допустимое проскальзывание (%)
    
    // Режим тестирования
    EnableBacktest bool     // true = симуляция, false = реальная торговля
}
```

**Примеры использования**:

```yaml
# Консервативная торговля (мало ордеров, малый риск)
trader:
    max_open_orders: 5
    max_position_size: 500.0
    default_strategy: dca           # Dollar Cost Averaging (надежная)
    strategy_update_interval: 30    # Редко обновляем
    slippage_percent: 0.1           # Строгие требования
    enable_backtest: true           # Сначала тестируем!

# Агрессивная торговля (много ордеров, высокий риск)
trader:
    max_open_orders: 20
    max_position_size: 5000.0
    default_strategy: grid          # Grid Trading (высокочастотная)
    strategy_update_interval: 5     # Часто обновляем
    slippage_percent: 1.0           # Мягче к проскальзыванию
    enable_backtest: false          # Уже тестировали
```

**Риск менеджмент**:

```
Если открыто 5 ордеров из 5 допустимых:
  └─ Новые ордера не создаются (защита от перегруза)

Если позиция BTC = 1500$ из максимума 5000$:
  └─ Можно открыть еще 3500$ позиции (следит за лимитом)

Если ордер исполнился хуже чем на 1% (SlippagePercent):
  └─ Позиция не открывается, ордер отменяется и переставляется
```

### 4. ClickHouseConfig - Подключение к ClickHouse

```go
type ClickHouseConfig struct {
    // Адрес подключения
    Host     string  // Хост ClickHouse (обычно какой-то удаленный сервер)
    Port     int     // Порт HTTP API (8123)
    Database string  // База данных в ClickHouse
    
    // Аутентификация
    Username string
    Password string
    
    // Безопасность
    UseTLS       bool  // HTTPS для подключения
    TLSSkipVerify bool // Пропустить проверку сертификата (небезопасно!)
    
    // Надежность
    ConnectTimeoutSec int // Таймаут (10s)
    MaxRetries        int // Кол-во попыток подключения
    
    // Оптимизация
    Compression bool  // Сжатие трафика (LZ4)
    MaxBatchSize int // Макс размер batch для отправки
    
    // Надежность данных
    ReplicationFactor int // Сколько копий данных хранить (1, 2, 3...)
}
```

**ClickHouse vs MySQL**:

| Параметр | MySQL | ClickHouse |
|----------|-------|-----------|
| Назначение | Конфигурация, текущее состояние | Исторические данные, аналитика |
| Объем данных | Малый-средний | Очень большой (terabytes) |
| Запросы | UPDATE, DELETE | Только INSERT и SELECT |
| Скорость аналитики | Медленная | Очень быстрая |
| Репликация | Сложная | Встроенная |

**Примеры конфигурации**:

```yaml
# Для разработки (локальный ClickHouse)
databases:
    quotes:
        engine: clickhouse
        clickhouse:
            host: localhost
            port: 8123
            username: default
            password: ""
            compression: false
            pool:
                connect_timeout: 10
                max_batch_size: 10000
                replication_factor: 1
            retry:
                max_attempts: 3

# Для production (облачный ClickHouse)
databases:
    quotes:
        engine: clickhouse
        clickhouse:
            host: clickhouse-prod.company.com
            port: 8123
            username: crypto_service
            password: very_secure_password
            tls:
                enabled: true
                skip_verify: false
            compression: true           # Сжимаем трафик
            pool:
                connect_timeout: 10
                max_batch_size: 50000     # Большие batches
                replication_factor: 2     # Дублируем данные
            retry:
                max_attempts: 5
```

---

## 📊 Примеры конфигурации для разных сценариев

### Сценарий 1: Только мониторинг (сбор данных)

```yaml
role: monitor

monitor:
    orderbook_depth: 50
    batch_size: 1000
    batch_interval: 3
    ring_buffer_size: 30000
    save_interval: 5

databases:
    quotes:
        engine: clickhouse
        clickhouse:
            host: clickhouse-prod.company.com
            port: 8123
            username: monitor_user
            password: pass
            compression: true
            tls:
                enabled: true
                skip_verify: false
            pool:
                connect_timeout: 10
                max_batch_size: 10000
                replication_factor: 1
            retry:
                max_attempts: 3
```

**Что происходит**:
1. Демон подключается к Binance, Bybit и другим биржам
2. Получает обновления книги ордеров по WebSocket
3. Сохраняет каждое обновление в ClickHouse
4. Аналитики могут запускать SQL запросы на ClickHouse
5. Нет никакой торговли, только сбор данных

### Сценарий 2: Только торговля (исполнение стратегий)

```yaml
role: trader

trader:
    max_open_orders: 10
    max_position_size: 1000.0
    default_strategy: grid
    strategy_update_interval: 10
    enable_backtest: false

# monitor конфиг не используется
```

**Что происходит**:
1. Демон загружает текущие позиции с бирж
2. Применяет стратегии к каждой позиции
3. Создает и отменяет ордера
4. Управляет рисками (макс ордеров, макс позиция)
5. Логирует все сделки

### Сценарий 3: Оба режима одновременно (monitoring + trading)

```yaml
role: both

monitor:
    orderbook_depth: 20
    batch_size: 500
    batch_interval: 5
    ring_buffer_size: 10000
    save_interval: 5

trader:
    max_open_orders: 5
    max_position_size: 500.0
    default_strategy: dca
    strategy_update_interval: 20
    enable_backtest: false

clickhouse:
    host: clickhouse-prod.company.com
```

**Что происходит**:
1. Monitor параллельно собирает данные в ClickHouse
2. Trader параллельно выполняет стратегии и торгует
3. Обе части используют одно WebSocket соединение (оптимизация)
4. Trader может использовать данные Monitor для улучшения стратегии

---

## 🔄 Загрузка конфигурации

```go
cfg, err := config.Load("conf/config.yaml")

// После загрузки можно проверить роль:
switch cfg.Role {
case "monitor":
    // Запустить только Monitor
    monitor.Start(cfg.Monitor)
case "trader":
    // Запустить только Trader
    trader.Start(cfg.Trader)
case "both":
    // Запустить оба компонента
    monitor.Start(cfg.Monitor)
    trader.Start(cfg.Trader)
}
```

---

## 📊 Данные в ClickHouse

Структура таблиц которые создаст Monitor:

```sql
-- Таблица для обновлений книги ордеров
CREATE TABLE orderbook (
    timestamp UInt64,          -- Микросекунды
    exchange String,           -- "binance", "bybit" и т.д.
    pair String,              -- "BTC/USDT"
    market_type String,       -- "spot" или "futures"
    bids Array(Tuple(price Float64, amount Float64)),  -- Покупатели
    asks Array(Tuple(price Float64, amount Float64)),  -- Продавцы
    depth UInt32,             -- 20, 50 или 0
    seq_num UInt64            -- Номер обновления
)
ENGINE = ReplicatedMergeTree()
ORDER BY (timestamp, exchange, pair);

-- Таблица для исполненных сделок
CREATE TABLE trades (
    timestamp UInt64,
    exchange String,
    pair String,
    price Float64,
    amount Float64,
    side String,              -- "buy" или "sell"
    trade_id String           -- ID сделки на бирже
)
ENGINE = ReplicatedMergeTree()
ORDER BY (timestamp, exchange, pair);
```

---

## ✅ Контрольный список

- ✅ Config структура расширена (Role, Monitor, Trader, ClickHouse)
- ✅ Функция Load() парсит все новые параметры из YAML
- ✅ Значения по умолчанию для всех параметров
- ✅ Подробные комментарии в каждом поле
- ✅ Пример конфигурации (config.example.yaml)
- ✅ Все скомпилировалось без ошибок

---

## 🚀 Следующий шаг: Phase 1.4

**Phase 1.4 - Создание SQL схемы**:
- [ ] MONITORING таблица (список пар для мониторинга)
- [ ] TRADE таблица (список пар для торговли)
- [ ] TRADE_HISTORY таблица (история исполненных ордеров)
- [ ] DAEMON_STATE таблица (состояние демона)

**Phase 1.5 - Интерфейс Exchange Driver**:
- [ ] Интерфейс для всех exchange драйверов
- [ ] Методы: Connect, Disconnect, GetOrderBook, PlaceOrder, CancelOrder
- [ ] Обработка ошибок и переподключение

---

## 📚 Использованные концепции

1. **Configuration-driven development** - поведение определяется конфигом, не кодом
2. **Role-based architecture** - один демон может выполнять разные роли
3. **Batch processing** - собираем данные в batch для эффективности
4. **Ring buffer** - кэш для последних данных без обращения к БД
5. **Risk management** - контроль максимальных ордеров и позиций
6. **Exponential backoff** - умное переподключение при ошибках
7. **Time-series database** - ClickHouse оптимизирована для временных рядов

