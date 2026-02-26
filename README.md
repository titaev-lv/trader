# trader - Crypto Trading System v2

**Высокопроизводительный демон для автоматического мониторинга и торговли криптовалютами на децентрализованных и централизованных биржах.**

---

## 📋 Описание

`trader` — это Go приложение для:
- **Мониторинга** стакана ордеров (orderbook) с разных крипто-бирж
- **Торговли** по автоматизированным стратегиям (grid, DCA, scalping)
- **Управления позициями** с контролем рисков
- **Хранения исторических данных** в ClickHouse
- **Outbound взаимодействия** с CTS-Core и биржами по WS/REST

Архитектура поддерживает работу в трёх режимах:
- **monitor** — только сбор данных с бирж в ClickHouse
- **trader** — только исполнение торговых стратегий
- **both** — одновременный мониторинг и торговля

---

## 🚀 Быстрый старт

### Требования
- **Go** 1.25.4 или выше
- **CTS-Core** (доступный по WS/REST)
- **ClickHouse** 23+ (для хранения исторических данных)
- Доступ к крипто-биржам API ключи (Binance, Bybit, OKX, Kucoin и т.д.)

### Установка
```bash
# Клонирование репозитория
git clone https://github.com/titaev-lv/daemon2.git
cd daemon2

# Установка зависимостей
go mod download

# Компиляция
go build -o trader cmd/trader/main.go

# Запуск
./trader
```

### Конфигурация
Скопируйте и отредактируйте файл конфигурации:
```bash
cp conf/config.example.yaml conf/config.yaml
# Отредактируйте conf/config.yaml с вашими параметрами
```

Основные параметры в `conf/config.yaml`:
```yaml
logging:
    level: debug             # debug, info, warn, error
    format: json             # json, text
    error_path: "/var/log/trader/error.log"
    out_request_path: "/var/log/trader/out_request.log"
    ws_in_path: "/var/log/trader/ws_in.log"
    ws_out_path: "/var/log/trader/ws_out.log"
    audit_path: "/var/log/trader/audit.log"
    out_request_to_stdout: true
    ws_in_to_stdout: true
    ws_out_to_stdout: true
    audit_to_stdout: true

role: both                 # monitor, trader или both

monitor:
    orderbook_depth: 20      # Глубина стакана: 20, 50 или 0 (full)
    batch_size: 500
    batch_interval: 5
    ring_buffer_size: 10000

trader:
    max_open_orders: 10
    max_position_size: 1000.0  # USDT
    default_strategy: grid      # grid, dca, scalp

databases:
    system:
        engine: ""
    audit:
        engine: ""
    quotes:
        engine: clickhouse
        clickhouse:
            host: localhost
            port: 8123
            database: crypto
            username: default
            password: default
```

ENV-переопределения для `databases.quotes.clickhouse`:
```bash
TRADER_DATABASES_QUOTES_ENGINE=clickhouse
TRADER_DATABASES_QUOTES_CLICKHOUSE_HOST=clickhouse
TRADER_DATABASES_QUOTES_CLICKHOUSE_PORT=8123
TRADER_DATABASES_QUOTES_CLICKHOUSE_DATABASE=crypto
TRADER_DATABASES_QUOTES_CLICKHOUSE_USERNAME=default
TRADER_DATABASES_QUOTES_CLICKHOUSE_PASSWORD=default
TRADER_DATABASES_QUOTES_CLICKHOUSE_TLS_ENABLED=false
TRADER_DATABASES_QUOTES_CLICKHOUSE_TLS_SKIP_VERIFY=false
TRADER_DATABASES_QUOTES_CLICKHOUSE_TLS_CA_PATH=
TRADER_DATABASES_QUOTES_CLICKHOUSE_TLS_CERT_PATH=
TRADER_DATABASES_QUOTES_CLICKHOUSE_TLS_KEY_PATH=
TRADER_DATABASES_QUOTES_CLICKHOUSE_POOL_CONNECT_TIMEOUT=10
TRADER_DATABASES_QUOTES_CLICKHOUSE_POOL_MAX_BATCH_SIZE=10000
TRADER_DATABASES_QUOTES_CLICKHOUSE_POOL_REPLICATION_FACTOR=1
TRADER_DATABASES_QUOTES_CLICKHOUSE_RETRY_MAX_ATTEMPTS=3
TRADER_DATABASES_QUOTES_CLICKHOUSE_RETRY_INITIAL_DELAY=1s
TRADER_DATABASES_QUOTES_CLICKHOUSE_RETRY_MAX_DELAY=5s
TRADER_DATABASES_QUOTES_CLICKHOUSE_RETRY_MULTIPLIER=2.0
```

---

## 📁 Структура проекта

```
daemon2/
├── cmd/
│   └── trader/
│       ├── main.go              # Точка входа, инициализация
│       └── trader               # Скомпилированный бинарник
├── conf/
│   ├── config.example.yaml      # Пример конфигурации (полный)
│   ├── config.yaml              # Рабочая конфигурация (НЕ коммитить!)
│   └── ssl/                     # TLS сертификаты (опционально)
├── internal/
│   ├── config/
│   │   └── config.go            # Загрузка конфигурации
│   ├── logger/
│   │   └── logger.go            # Структурированное логирование
│   ├── manager/
│   │   └── manager.go           # Управление жизненным циклом
│   ├── state/
│   │   └── state.go             # Сохранение состояния демона
│   └── core/
│       ├── exchange/
│       │   ├── types.go          # Типы данных (Order, Trade и т.д.)
│       │   └── drivers/          # Драйверы для 8 бирж
│       └── messaging/
│           └── message.go        # Унифицированный формат сообщений
├── logs/                        # Логи приложения
├── state/
│   └── trader.state             # Состояние процесса (JSON)
├── go.mod                       # Go модули
├── go.sum                       # Хешибьем зависимостей
├── README.md                    # Этот файл
├── CODE_STRUCTURE.md            # Подробное описание структуры
├── PHASE_1_2_TYPES_REFERENCE.md # Справочник типов Phase 1.2
└── PHASE_1_3_DETAILED.md        # Описание Phase 1.3 (конфигурация)
```

---

## 🏗️ Архитектура

### Инициализация
При запуске сервис выполняет следующие этапы (в порядке):
1. **Load Config** — загрузка конфигурации из `config.yaml`
2. **Init Logger** — инициализация логирования со своим форматом
3. **Init Manager** — создание менеджера жизненного цикла
4. **Start Manager** — запуск monitor/trader компонентов
5. **Setup Signals** — обработка SIGINT/SIGTERM для graceful shutdown

### Graceful Shutdown
При получении сигнала на завершение (Ctrl+C, SIGTERM):
- Закрывает открытые позиции/ордера
- Сохраняет состояние в `state/trader.state`
- **Timeout:** максимум 30 секунд на завершение

### Компоненты

#### Config (`internal/config/config.go`)
- Парсит конфигурацию из YAML файла
- Поддерживает режимы: monitor, trader, both
- Конфигурация для логирования, monitor/trader режимов и ClickHouse
- Параметры мониторинга и торговли с defaults

#### Logger (`internal/logger/logger.go`)
- Структурированное логирование через `slog`
- JSON формат
- Ротация логов по размеру
- Отдельные файлы: `error.log`, `out_request.log`, `audit.log`

#### Manager (`internal/manager/manager.go`)
- Управление жизненным циклом сервиса
- Start/Stop методы
- Context-based synchronization
- Graceful shutdown с таймаутом

#### State (`internal/state/state.go`)
- Сохранение состояния в JSON файл (`state/trader.state`)
- Singleton pattern с sync.Once
- Фиксация последнего состояния lifecycle для диагностики

## 🔐 Безопасность

### Transport Security
- Для исходящих WS/REST интеграций используйте TLS и проверку сертификатов
- Не храните ключи и токены в репозитории
- Логируйте только безопасные метаданные запросов (без секретов)

### Конфиденциальность
- API ключи бирж в переменные окружения или secure vault
- Логи содержат sensitive data (обрезать для production)

---

## ⚡ Производительность

### Оптимизация
- **Ring Buffer:** кэширование последних данных в памяти
- **Batch Processing:** отправка обновлений батчами в ClickHouse
- **Async Processing:** горутины для параллельной работы

### Масштабируемость
- **Timestamp:** микросекунды (1 μs = 10^-6 sec) для точности HFT
- **ClickHouse:** оптимизирована для аналитики больших объемов данных
- **Multiple Exchanges:** независимые горутины для каждой биржи

---

## 📊 Типы данных

### Унифицированный формат (Phase 1.2)
Все данные от разных бирж преобразуются в унифицированный формат:

```go
// Orderbook (стакан ордеров)
type Level struct {
    Price     float64
    Amount    float64
    Timestamp int64    // microseconds
}

// Message (унифицированное сообщение)
type Message struct {
    Type      string    // "orderbook", "trade", "order", "position"
    ExchangeID string   // "binance", "bybit", "okx"
    Pair      string    // "BTC/USDT"
    Timestamp int64     // microseconds (UTC)
    Data      []byte    // JSON
}
```

---

## 🔌 Поддерживаемые биржи (Phase 1.5+)

Планируется поддержка:
- ✅ **Binance** (spot + futures)
- ✅ **Bybit** (spot + futures)
- ✅ **OKX** (spot + futures)
- ✅ **Kucoin** (spot + futures)
- ✅ **Coinex** (spot)
- ✅ **HTX** (spot + futures, бывший Huobi)
- ✅ **MEXC** (spot + futures)
- ⏳ **DEX** (Uniswap V3, 1inch и т.д.)

---

## 📈 Развитие (Roadmap)

### Phase 1 (текущая) - Фундамент
- ✅ Phase 1.1: Структура проекта
- ✅ Phase 1.2: Типы данных (Order, Trade и т.д.)
- ✅ Phase 1.3: Конфигурация с поддержкой режимов
- ⏳ Phase 1.4: Полная интеграция task flow через CTS-Core (WS протокол)
- ⏳ Phase 1.5: Exchange driver interface + Binance реализация
- ⏳ Phase 1.6-1.8: Остальные биржи (Bybit, OKX, Kucoin и т.д.)

### Phase 2 - Стратегии
- Grid Trading
- Dollar Cost Averaging (DCA)
- Scalping с микро-ордерами

### Phase 3 - Анализ
- Риск-менеджмент
- Backtesting
- Performance метрики

### Phase 4+ - Расширение
- WebSocket live updates
- Mobile app для управления
- Machine learning стратегии

---

## 🛠️ Разработка

### Требования для разработки
```bash
go version              # Go 1.25.4+
golangci-lint --version # Линтер
go test ./...           # Тесты
```

### Построение
```bash
# Debug версия
go build -o trader cmd/trader/main.go

# Release версия
go build -ldflags="-s -w" -o trader cmd/trader/main.go
```

### Тестирование
```bash
# Запуск всех тестов
go test ./...

# С покрытием
go test -cover ./...
```

### Форматирование кода
```bash
# Форматирование
gofmt -w .

# Лinting
golangci-lint run ./internal/...
```

---

## 📚 Документация

### Основные файлы документации
- **`CODE_STRUCTURE.md`** — полное описание структуры проекта и всех компонентов
- **`PHASE_1_2_TYPES_REFERENCE.md`** — справочник всех типов данных Phase 1.2
- **`PHASE_1_3_DETAILED.md`** — подробное описание конфигурации Phase 1.3
- **`ARCHITECTURE.md`** (TODO) — архитектурные диаграммы и data flow
- **`DEVELOPMENT_PLAN.md`** — единый актуальный план развития Trader

### Комментарии в коде
- ✅ Все файлы задокументированы на русском
- ✅ Функции содержат полное описание параметров и возвращаемых значений
- ✅ Критичные параметры отмечены (ОПАСНО!, РЕКОМЕНДУЕТСЯ!)

---

## 🐛 Известные проблемы

- Нет поддержки WebSocket (только REST)
- ClickHouse интеграция в разработке
- Backtesting только симуляция (реальная торговля в Phase 2+)

---

## 📝 Лицензия

Proprietary. Все права защищены.

---

## 👥 Автор

**Лев Титаев** (@titaev-lv)
- GitHub: https://github.com/titaev-lv

---

## 📞 Контакты

- Issues: https://github.com/titaev-lv/daemon2/issues
- Discussions: https://github.com/titaev-lv/daemon2/discussions

---

## 🔗 Ссылки

- [Go Documentation](https://golang.org/doc/)
- [ClickHouse Go Client](https://github.com/ClickHouse/clickhouse-go)

---

**Последнее обновление:** 11 декабря 2024 г. | **Version:** 2.0.1
