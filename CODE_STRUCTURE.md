# Структура кода trader - Подробное описание

Этот документ описывает архитектуру и назначение каждого основного файла проекта.

---

## 📁 Точка входа приложения

### `cmd/trader/main.go` - Главная функция приложения

**Назначение**: Точка входа в приложение. Инициализирует все компоненты в правильном порядке.

**Порядок инициализации (критичен)**:
1. **Загрузка конфигурации** - читает `conf/config.yaml`
2. **Инициализация логирования** - создает систему логов в `./logs`
3. **Инициализация Manager** - создает менеджер для управления компонентами
4. **Запуск Manager** - стартует monitor/trader компоненты
5. **Обработка сигналов ОС** - перехватывает SIGINT и SIGTERM для graceful shutdown

**Ключевые переменные**:
- `Version = "2.0.1"` - текущая версия приложения
- `configFile` - флаг для указания пути к конфигу

**Graceful shutdown**:
```
Ctrl+C (SIGINT) или kill -15
  ↓
mgr.Stop() - останавливает все компоненты
  ↓
logger.Close() - закрывает логи (через defer)
  ↓
Выход
```

---

## 🔧 Компоненты приложения

### `internal/config/config.go` - Конфигурация

**Назначение**: Загрузка и парсинг конфигурации из YAML файла.

**Структуры**:
```go
  Databases DatabasesConfig // Параметры хранилищ (system/audit/quotes)
  Logging   LogConfig       // Параметры логирования
  Trade     TradeConfig     // Параметры торговли
  OrderBook OrderBookConfig // Параметры книги ордеров
}
  - Рекомендуется ≥ 5

**LogConfig - система логирования**:
- `Level` - "debug", "info", "warn", "error"
- `Dir` - папка для логов (по умолчанию ./logs)
- `MaxFileSizeMB` - максимальный размер файла перед ротацией

**Загрузка**:
```go
cfg, _ := config.Load("conf/config.yaml")
// cfg.Log.Level == "info"
```

---

### `internal/logger/logger.go` - Система логирования

**Назначение**: Структурированное логирование во все компоненты приложения с ротацией файлов.

**Особенности**:
- Использует встроенный Go `slog` (structured logging)
- JSON формат для унифицированной обработки
- Автоматическая ротация файлов через `lumberjack`
- Разные логгеры для системных, outbound-request и audit событий

**Глобальные переменные**:
- `Log` - основной логгер для ошибок
- `OutRequestLog` - логгер исходящих REST/WS request-событий
- `WSInLog` - логгер входящих сообщений WebSocket
- `WSOutLog` - логгер исходящих сообщений WebSocket
- `AuditLog` - логгер audit-событий
- `Trade` - специальный логгер для торговых операций
- `logFiles` - map всех открытых файлов логов

**Типы логов (по уровню важности)**:
```
DEBUG   - детальная отладка разработчика (отключается в production)
INFO    - основные события (запуск, подключение) - рекомендуется в production
WARN    - неожиданные но некритичные события (потеря соединения, retry)
ERROR   - критичные ошибки (crash, некорректные данные)
```

**Инициализация**:
```go
// Инициализация выполняется из cmd/trader/main.go на основе conf/config.yaml
// stdout дублирование потоков управляется флагами:
// out_request_to_stdout, ws_in_to_stdout, ws_out_to_stdout, audit_to_stdout

log := logger.Get("main")
log.Info("Starting", "version", "2.0.1")
log.Error("Connection failed", "error", err)

logger.TradeInfo("Opened position", "pair", "BTC/USDT")
logger.TradeError("Margin low", "level", "critical")
```

**Файлы логов**:
- `error.log` - системные/ошибочные события
- `out_request.log` - исходящие запросы/вызовы к внешним сервисам
- `ws_in.log` - входящие WS-сообщения/события
- `ws_out.log` - исходящие WS-команды/подписки
- `audit.log` - audit события для критичных действий трейдера
- При ротации: `*.log` с suffix от `lumberjack`

**Функции**:
- `Init(level, dir, maxMB)` - инициализация
- `Get(module)` - получить логгер для модуля
- `GetOutRequest(module)` - получить логгер исходящих запросов
- `GetWSIn(module)` - получить логгер входящих WS-сообщений
- `GetWSOut(module)` - получить логгер исходящих WS-сообщений
- `GetAudit(module)` - получить audit логгер
- `Debug/Info/Warn/Error(msg, args...)` - логирование с разными уровнями
- `TradeInfo/TradeWarn/TradeError()` - специальные функции для торговли
- `Close()` - закрыть все файлы логов (вызывается при выходе)

---

### `internal/manager/manager.go` - Менеджер приложения

**Назначение**: Управляет lifecycle всех компонентов и координирует их работу.

**Ответственность Manager**:
1. **Управление состоянием** - запуск/остановка системы
2. **Синхронизация** - через context дает сигнал всем goroutine о завершении
3. **Graceful shutdown** - корректное завершение с таймаутом
4. **Отслеживание времени** - знает когда запустился, как долго работает

**Структура**:
```go
Manager {
    cfg            *config.Config    // Конфигурация
    ctx, cancel    context.Context   // Для graceful shutdown
    wg             sync.WaitGroup    // Отслеживание goroutine
    shutdownOnce   sync.Once         // Guarantee shutdown happens once
    isRunning      bool              // Текущее состояние
    startTime      time.Time         // Когда запустился
    shutdownTime   time.Time         // Когда остановился
    shutdownError  error             // Ошибка если была при shutdown
}
```

**Жизненный цикл**:
```
1. New(cfg) → создается менеджер (еще не запущен)

2. Start() → 
   - Проверяет что не запущен уже
   - Устанавливает isRunning = true
  - Сохраняет state на диск
   - Запускает все компоненты в порядке зависимостей
   
3. Stop() → 
   - Проверяет что запущен
   - Использует shutdownOnce для гарантии однократного выполнения
   - Отправляет cancel() в контекст (сигнал для всех goroutine)
   - Ждет завершения с таймаутом (GracefulShutdownTimeout = 30s)
   - Если таймаут истек - принудительно убивает goroutine
```

**Методы**:
- `Start()` - запустить систему
- `Stop()` - остановить систему (graceful)
- `GetStatus()` - получить информацию (running, uptime, start_time и т.д.)
- `IsRunning()` - простая проверка статуса
- `GetContext()` - получить контекст для компонентов

**Пример использования**:
```go
mgr := manager.New(cfg)

// Запуск
if err := mgr.Start(); err != nil {
    log.Fatal(err)
}

// Позже...
if err := mgr.Stop(); err != nil {
    log.Fatal(err)
}

// Информация
status := mgr.GetStatus()
// status["uptime"] = "2h 30m 45s"
// status["running"] = true
```

---

### `internal/state/state.go` - Управление состоянием

**Назначение**: Сохраняет и восстанавливает состояние процесса между запусками.

**Зачем нужно**:
- Фиксирует последнее lifecycle-состояние процесса
- Упрощает диагностику при рестартах и авариях
- Обеспечивает восстановление контекста работы (без inbound auto-start логики)

**Структура**:
```go
State {
    IsRunning bool  // Был ли запущен при последнем выключении
}

Manager {
    filePath string      // Путь к файлу (state/trader.state)
    state    *State      // Текущее состояние в памяти
    mu       sync.RWMutex // Для потокобезопасности
}
```

**Singleton паттерн**:
```go
// Получить единственный экземпляр во всем приложении
mgr := state.GetInstance()
```

**Файл состояния** - `state/trader.state`:
```json
{
  "is_running": true
}
```

**Методы**:
- `GetInstance()` - получить singleton Manager
- `Load()` - загрузить состояние с диска (вызывается при init)
- `Save()` - сохранить состояние на диск
- `SetRunning(bool)` - установить флаг и сохранить
- `IsRunning()` - получить текущий флаг

**Использование**:
```go
// В manager.Start():
state.GetInstance().SetRunning(true)

// В manager.Stop():
state.GetInstance().SetRunning(false)
```

---

## 📊 Микросекундная точность времени

**Все timestamp в проекте используют МИКРОСЕКУНДЫ (microseconds)**:

```
Примеры значений:
- 1702274400000000 = 2023-12-11 12:00:00 UTC (в микросекундах)
- 1702274400000 = 2023-12-11 12:00:00 UTC (в миллисекундах)

Конверсия:
- Из миллисекунд: ms × 1000 = μs
- Из наносекунд: ns ÷ 1000 = μs
```

**Используется в**:
- `exchange.OrderBook.Timestamp` - когда биржа выслала update
- `messaging.Message.Timestamp` - когда произошло событие
- Логирование и БД

**Преимущества микросекунд**:
- ✓ Достаточная точность для всех бирж
- ✓ Легкая конверсия из других форматов
- ✓ Стандарт для HFT систем
- ✓ Встроенная поддержка в Go: `time.UnixMicro()`

---

## 🔄 Поток данных

```
OS Signal (SIGINT/SIGTERM)
  ↓
main() перехватывает → вызывает mgr.Stop()
  ↓
Manager.Stop()
  ↓
Manager.doStop() - graceful shutdown
  ├─ cancel() - отправляет сигнал в контекст
  ├─ Все goroutine видят контекст отменен
  ├─ Ждет завершения с таймаутом 30s
  └─ Если не завершились - force cancel
  ↓
state.SetRunning(false) - сохраняет состояние
  ↓
db.Close() - закрывает БД
  ↓
logger.Close() - закрывает логи
  ↓
Приложение завершается
```

---

## 📋 Контрольный список компонентов

| Файл | Назначение | Инициализация | Завершение |
|------|-----------|--------------|-----------|
| main.go | Точка входа | Порядок критичен | graceful shutdown |
| config.go | Конфигурация | Первым (нужна для всех) | Н/А |
| logger.go | Логирование | Вторым (для отладки) | Close() в defer |
| manager.go | Менеджер | Четвертым | Stop() → graceful |
| state.go | Состояние | При init Manager | SetRunning(false) |

---

## 🚀 Как запустить

```bash
# Помощь
./trader -h

# С конфигом по умолчанию
./trader

# С кастомным конфигом
./trader -c /path/to/config.yaml
```

## 📝 Логирование

```bash
# Просмотр основных логов
tail -f /var/log/trader/error.log

# Просмотр торговых логов
tail -f /var/log/trader/out_request.log

# Последних 100 строк
tail -100 /var/log/trader/error.log

# С фильтром
grep "ERROR" /var/log/trader/error.log
grep "Connection" /var/log/trader/error.log
```

---

## 🔐 Безопасность

**TLS/SSL поддержка**:
```yaml
databases:
  quotes:
    engine: clickhouse
    clickhouse:
      tls:
        enabled: true
        skip_verify: false
      pool:
        connect_timeout: 10
      retry:
        max_attempts: 3
```

**Таймауты**:
- Подключение БД: `clickhouse.pool.connect_timeout` (по умолчанию 10s)
- Graceful shutdown: 30 секунд (GracefulShutdownTimeout)
- Retry logic: количество попыток = `clickhouse.retry.max_attempts`

**ENV overrides (quotes.clickhouse)**:
- `TRADER_DATABASES_QUOTES_CLICKHOUSE_TLS_ENABLED`
- `TRADER_DATABASES_QUOTES_CLICKHOUSE_TLS_SKIP_VERIFY`
- `TRADER_DATABASES_QUOTES_CLICKHOUSE_POOL_CONNECT_TIMEOUT`
- `TRADER_DATABASES_QUOTES_CLICKHOUSE_POOL_MAX_BATCH_SIZE`
- `TRADER_DATABASES_QUOTES_CLICKHOUSE_POOL_REPLICATION_FACTOR`
- `TRADER_DATABASES_QUOTES_CLICKHOUSE_RETRY_MAX_ATTEMPTS`

