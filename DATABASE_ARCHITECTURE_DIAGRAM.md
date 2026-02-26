# PHASE 1.4: Диаграмма связей и архитектура БД

## 📊 Полная диаграмма отношений БД (Entity-Relationship Diagram)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          DATABASE ARCHITECTURE                              │
│                              (ct_system)                                    │
└─────────────────────────────────────────────────────────────────────────────┘


                                  ┌──────────┐
                                  │  USER    │
                                  │(users)   │
                                  └────┬─────┘
                    ┌───────────────────┼───────────────────┐
                    │                   │                   │
                    │                   │                   │
         ┌──────────▼─────────┐  ┌──────▼──────┐   ┌────────▼──────┐
         │  MONITORING        │  │  TRADE      │   │  GROUP        │
         │  (мониторинг)      │  │  (торговля) │   │  (группы)     │
         │                    │  │             │   │               │
         │ ✨ Phase 1.4 New:  │  │ ✨Phase 1.4 │   │               │
         │ - ORDERBOOK_DEPTH  │  │   New:      │   │               │
         │ - BATCH_SIZE       │  │ - MAX_OPEN  │   │               │
         │ - BATCH_INTERVAL   │  │   _ORDERS   │   │               │
         │ - RING_BUFFER_SZ   │  │ - MAX_POS   │   │               │
         │ - SAVE_INTERVAL    │  │   _SIZE     │   │               │
         │                    │  │ - STRATEGY  │   │               │
         │ (6 + 5 new cols)   │  │ - SLIPPAGE  │   │               │
         │ ✅ 7 records       │  │ - BACKTEST  │   │ (2 records)   │
         │                    │  │             │   │               │
         │ 🔗 PK: ID          │  │ 🔗 PK: ID   │   │ 🔗 PK: ID     │
         │ 🔗 FK: UID→USER    │  │ 🔗 FK: UID→ │   │ 🔗 FK:        │
         │                    │  │   USER      │   │ CREATED→USER  │
         └─────────┬────-─────┘  │ 🔗 FK:      │   │ MODIFY→USER   │
                   │             │   TYPE→     │   └───────────────┘
                   │             │   TRADE_TYP │
                   │             │   E         │
                   │             └──────┬──────┘
                   │                    │
        ┌──────────▼──────────┐  ┌───-──▼──────────────┐
        │MONITORING_SPOT_     │  │  TRADE_SPOT_        │
        │ARRAYS               │  │  ARRAYS  (⭐ NEW)   │
        │                     │  │                     │
        │ 🔗 FK: MONITOR_ID→  │  │ 🔗 FK: TRADE_ID→    │
        │       MONITORING    │  │       TRADE         │
        │ 🔗 FK: PAIR_ID→     │  │ 🔗 FK: PAIR_ID→     │
        │       SPOT_TRADE_   │  │       SPOT_TRADE    │
        │       PAIR          │  │ 🔗 FK: EAID→        │
        │                     │  │       EXCHANGE_     │
        │ (junction table)    │  │       ACCOUNTS      │
        └─────────┬───────────┘  │ (8 records)         │
                  │              │                     │
                  │              │ START_AMOUNT_BASE   │
                  │              │ START_AMOUNT_QUOTE  │
                  │              │ MIN_DELTA_PROFIT    │
                  │              └──────────┬────────-─┘
                  │                         │
                  └──────────────┬──────────┘
                                 │
                    ┌────────────▼───────────┐
                    │  SPOT_TRADE_PAIR       │
                    │  (торговые пары)       │
                    │                        │
                    │ 🔗 FK: (COIN_ID,       │
                    │        COIN_ID_QUOTE)  │
                    │ 🔗 FK: CHAIN_ID        │
                    │                        │
                    │ (1000+ pairs)          │
                    │ PK: ID                 │
                    └─────────┬──────────────┘
                              │
                   ┌──────────┴───────────┐
                   │                      │
         ┌─────────▼────────┐   ┌────────▼──────────┐
         │  COIN            │   │  SPOT_TRADE_PAIR_ │
         │  (криптовалюты)  │   │  FEE              │
         │                  │   │  (комиссии)       │
         │ (3000+ coins)    │   │                   │
         │                  │   │ 🔗 FK: PAIR_ID→   │
         │ PK: ID           │   │       SPOT_TRADE  │
         │ INDEX: SYMBOL    │   │       _PAIR       │
         └──────────────────┘   └───────────────────┘
                   │
                   │
         ┌─────────▼────────┐
         │  CHAIN           │
         │  (блокчейн сети) │
         │                  │
         │ ETH, BNB, SOL... │
         │ (330+ chains)    │
         │                  │
         │ PK: ID           │
         │ UNIQUE: NAME     │
         └──────────────────┘


┌───────────────────────────────────────────────────────────────────┐
│           EXCHANGE & ACCOUNTS (Справочные данные)                 │
└───────────────────────────────────────────────────────────────────┘

         ┌──────────────────┐
         │  EXCHANGE        │
         │  (биржи)         │
         │                  │
         │ Binance, Bybit   │
         │ OKX, Kucoin...   │
         │ (8 exchanges)    │
         │                  │
         │ PK: ID           │
         │ NAME             │
         │ BASE_URL         │
         │ WEBSOCKET_URL    │
         │ CLASS_TO_FACTORY │
         └────────┬─────────┘
                  │
         ┌────────▼──────────────┐
         │  EXCHANGE_ACCOUNTS    │
         │  (API ключи)          │
         │                       │
         │ 🔗 FK: EXID→EXCHANGE  │
         │ 🔗 FK: UID→USER       │
         │                       │
         │ (9 records)           │
         │                       │
         │ API_KEY               │
         │ SECRET_KEY            │
         │ ADD_KEY               │
         │ PRIORITY              │
         │ ACTIVE                │
         └───────────────────────┘


┌───────────────────────────────────────────────────────────────────┐
│    ✨ NEW PHASE 1.4 TABLES                                        │
└───────────────────────────────────────────────────────────────────┘

         ┌─────────────────────────┐
         │  TRADE_HISTORY          │  🆕 NEW TABLE
         │  (история сделок)       │
         │                         │
         │ PK: ID (bigint)         │
         │ 🔗 FK: TRADE_ID         │
         │ 🔗 FK: PAIR_ID          │
         │ 🔗 FK: EAID             │
         │                         │
         │ ORDER_ID (от биржи)     │
         │ SIDE (BUY/SELL)         │
         │ PRICE, QUANTITY         │
         │ COMMISSION              │
         │ EXECUTED_TIME (микросек)│
         │ STATUS                  │
         │ PROFIT_LOSS             │
         │                         │
         │ 📌 Ключевой индекс:     │
         │ (TRADE_ID, EXEC_TIME)   │
         │                         │
         │ ⏳ Растущая таблица!    │
         └────────────┬────────────┘
                      │
         ┌────────────▼──────────┐
         │  DAEMON_STATE         │  🆕 NEW TABLE
         │  (состояние демонов)  │
         │                       │
         │ PK: ID                │
         │ UNIQUE: DAEMON_NAME   │
         │                       │
         │ STATUS                │
         │ (STARTING/RUNNING/...)│
         │                       │
         │ ROLE                  │
         │ (MONITOR/TRADER/BOTH) │
         │                       │
         │ LAST_HEARTBEAT        │
         │ (микросекунды)        │
         │                       │
         │ 🔗 FK: ACTIVE_MON_ID→ │
         │        MONITORING     │
         │ 🔗 FK: ACTIVE_TRADE_ID│
         │        TRADE          │
         │                       │
         │ 📌 Для graceful       │
         │    shutdown & recovery│
         └───────────────────────┘


┌───────────────────────────────────────────────────────────────────┐
│              POSITION & TRANSACTION TABLES                        │
└───────────────────────────────────────────────────────────────────┘

         ┌───────────────────┐
         │  POS_POSITIONS    │
         │  (открытые позиции)
         │                   │
         │ 🔗 FK: PAIR_ID    │
         │ ENTRY_PRICE       │
         │ ENTRY_SIZE        │
         └───────────────────┘
                  │
         ┌────────▼─────────────┐
         │  POS_TRANSACTIONS    │
         │  (транзакции позиций)|
         │                      │
         │ 🔗 FK: POS_ID        │
         │ SIDE                 │
         │ AMOUNT               │
         └──────────────────────┘


┌───────────────────────────────────────────────────────────────────┐
│          REFERENCE & ANALYTICS TABLES                             │
└───────────────────────────────────────────────────────────────────┘

┌────────────────────┐  ┌──────────────────┐  ┌─────────────────────┐
│  ARBITRAGE_TRANS   │  │  PRICE_SPOT_LOG  │  │  TRADE_TYPE         │
│  (арбитраж)        │  │  (истории цен)   │  │  (типы торговли)    │
│                    │  │                  │  │                     │
│ 🔗 FK: TRADE_ID    │  │ 🔗 FK: PAIR_ID   │  │ Limit, Market, etc  │
│ 🔗 FK: STATUS_ID   │  │ PRICE            │  │                     │
│ AMOUNT             │  │ VOLUME_24H       │  │ PK: ID              │
│ CALC_PROFIT        │  │                  │  │ NAME                │
│                    │  │ ⏳ Растущая!     │  └─────────────────────┘
│ (80 records)       │  │ (1000+ в день)   │
└────────────────────┘  └──────────────────┘

┌─────────────────────┐  ┌──────────────────┐  ┌─────────────────────┐
│  ARBITRAGE_TRANS_   │  │  UPDATE_STATUS   │  │  VERSION_DB         │
│  STATUS             │  │  (статусы обн.)  │  │  (версия БД)        │
│  (статусы арб.)     │  │                  │  │                     │
│                     │  │ Pending, Waiting │  │ version_number      │
│ Profitable, Failed..│  │ Done, etc        │  │ install_date        │
│                     │  │                  │  │                     │
│ (9 records)         │  │ (4 records)      │  │ (управление)        │
└─────────────────────┘  └──────────────────┘  └─────────────────────┘

┌─────────────────────┐
│  WITHDRAWAL         │
│  (выводы средств)   │
│                     │
│ 🔗 FK: UID          │
│ 🔗 FK: EXID         │
│ CHAIN_ID            │
│ ADDRESS             │
│ AMOUNT              │
└─────────────────────┘


┌───────────────────────────────────────────────────────────────────┐
│  USER MANAGEMENT (Пользователи и группы)                          │
└───────────────────────────────────────────────────────────────────┘

         ┌──────────────┐
         │  USER        │
         │  (пользователи)
         │              │
         │ ID           │
         │ NAME         │
         │ EMAIL        │
         └────┬─────────┘
              │
    ┌─────────▼──────────┐
    │  USERS_GROUP       │
    │  (связь пользователь-группа)
    │                    │
    │ 🔗 FK: UID→USER    │
    │ 🔗 FK: GID→GROUP   │
    └────────────────────┘


┌───────────────────────────────────────────────────────────────────┐
│                  INDEXES & PERFORMANCE                            │
└───────────────────────────────────────────────────────────────────┘

✨ Ключевые индексы Phase 1.4:

MONITORING:
  - PRIMARY KEY (ID)
  - INDEX (UID, ACTIVE)  ← для быстрого поиска активных конфигов
  - INDEX (ACTIVE, ID)

TRADE:
  - PRIMARY KEY (ID)
  - INDEX (UID, ACTIVE)  ← для быстрого поиска активных конфигов
  - INDEX (ACTIVE, ID)

TRADE_HISTORY:
  - PRIMARY KEY (ID)
  - INDEX (TRADE_ID)             ← для аналитики по конфигу
  - INDEX (EXECUTED_TIME)        ← для временных срезов
  - INDEX (ORDER_ID)             ← для поиска конкретной сделки
  - INDEX (TRADE_ID, EXEC_TIME)  ← составной индекс!

DAEMON_STATE:
  - PRIMARY KEY (ID)
  - UNIQUE KEY (DAEMON_NAME)     ← гарантирует одного демона
  - INDEX (STATUS)               ← для поиска мертвых демонов
  - INDEX (LAST_HEARTBEAT)       ← для cleanup старых записей


┌───────────────────────────────────────────────────────────────────┐
│              DATA FLOW & RELATIONSHIPS                            │
└───────────────────────────────────────────────────────────────────┘

USER создает MONITORING конфигурацию:
  USER(ID=1) → MONITORING(ID=1, UID=1)
    ↓
  Мониторит торговые пары через MONITORING_SPOT_ARRAYS:
    MONITORING(1) → [SPOT_TRADE_PAIR(1), SPOT_TRADE_PAIR(2)...]
    ↓
  Каждая пара связана с COIN и CHAIN:
    SPOT_TRADE_PAIR(1) → COIN(BTC), COIN(USDT), CHAIN(mainnet)


USER создает TRADE конфигурацию:
  USER(ID=1) → TRADE(ID=1, UID=1)
    ↓
  Торговля по парам через TRADE_SPOT_ARRAYS:
    TRADE(1) → [
      {PAIR_ID=1, EAID=1},
      {PAIR_ID=2, EAID=2}
    ]
    ↓
  Каждый ордер записывается в TRADE_HISTORY:
    TRADE(1) → TRADE_HISTORY (множество сделок)
    ↓
  Отслеживается в DAEMON_STATE:
    DAEMON_STATE(ACTIVE_TRADE_ID=1) ↔ TRADE(1)


DAEMON_STATE отслеживает жизненный цикл:
  [STARTING] → [RUNNING] → [STOPPING] → [STOPPED]
    ↓
  Хранит ссылку на активную конфигурацию:
    DAEMON_STATE.ACTIVE_MONITORING_ID = MONITORING.ID
    DAEMON_STATE.ACTIVE_TRADE_ID = TRADE.ID
    ↓
  Отправляет heartbeat каждые N секунд (микросекунды):
    UPDATE DAEMON_STATE SET LAST_HEARTBEAT = now_microseconds()


┌───────────────────────────────────────────────────────────────────┐
│              STATISTICS & CAPACITY                                │
└───────────────────────────────────────────────────────────────────┘

Таблица           Текущие   Ожидаемые    Растущая?  Примечание
──────────────────────────────────────────────────────────────
COIN              3000+     5000+        Да         CoinMarketCap
CHAIN             330+      500+         Да         New chains
SPOT_TRADE_PAIR   1000+     5000+        Да         Все пары бирж
DEPOSIT           458K+     1M+          Да         Все депозиты
MONITORING        7         100          Медленно    Пользовательские конфиги
TRADE             8         100          Медленно    Пользовательские конфиги
TRADE_HISTORY     0         1000000+     ДА! 🔥     РАСТЕТ БЫСТРО!
PRICE_SPOT_LOG    ?         1000+/день   ДА! 🔥     РАСТЕТ БЫСТРО!
DAEMON_STATE      0         100          Медленно    Демоны-процессы


📊 РАЗМЕРЫ ТАБЛИЦ (примерные):

Таблица           Размер одной строки  Текущий объем  На год
──────────────────────────────────────────────────────────────
TRADE_HISTORY     ~200 bytes           ~1 MB          ~365 GB 🔥
PRICE_SPOT_LOG    ~150 bytes           ~1 MB          ~150 GB 🔥

💡 ВЫВОД: TRADE_HISTORY и PRICE_SPOT_LOG требуют архивирования!
          Рассмотреть партиционирование по дате!

```

---

## 🔄 Жизненный цикл данных

```
1️⃣ СОЗДАНИЕ КОНФИГУРАЦИИ:
   
   Пользователь → UI
        ↓
   INSERT INTO MONITORING (UID, ORDERBOOK_DEPTH, ...)
        ↓
   INSERT INTO MONITORING_SPOT_ARRAYS (MONITOR_ID, PAIR_ID)
        ↓
   ✅ MONITORING готовой к запуску

2️⃣ ЗАПУСК ДЕМОНА:

     go run main.go --config config.yaml
        ↓
     Демон парсит config.yaml → MonitorConfig struktura
        ↓
   SELECT * FROM MONITORING WHERE ID = config_id AND ACTIVE = 1
        ↓
   INSERT INTO DAEMON_STATE (DAEMON_NAME, STATUS=STARTING, ...)
        ↓
   [Инициализация]
        ↓
   UPDATE DAEMON_STATE SET STATUS = 'RUNNING', LAST_HEARTBEAT = now()
        ↓
   ✅ Демон работает

3️⃣ ДАННЫЕ МОНИТОРИНГА:

   while daemon.running {
       TickerOrder book data from birzha
            ↓
       Cache в ring buffer (RingBufferSize)
            ↓
       if batch_size_reached || timeout_reached {
           INSERT INTO PRICE_SPOT_LOG (pair_id, price, volume, ...)
           INSERT INTO MONITORING_SPOT_ARRAYS updates
       }
            ↓
       Heartbeat UPDATE DAEMON_STATE
   }

4️⃣ ТОРГОВЛЯ И СДЕЛКИ:

   while trader.running {
       Анализ рынка
            ↓
       CREATE ORDER на бирже
            ↓
       Get ORDER_ID от биржи
            ↓
       if order.filled {
           INSERT INTO TRADE_HISTORY (ORDER_ID, SIDE, PRICE, QTY, ...)
           if matched_with_previous_order {
               UPDATE PROFIT_LOSS
           }
       }
            ↓
       EXEC happens on EXCHANGE_ACCOUNTS API
   }

5️⃣ ОСТАНОВКА ДЕМОНА:

   signal: SIGTERM/SIGINT
        ↓
   UPDATE DAEMON_STATE SET STATUS = 'STOPPING'
        ↓
   Graceful shutdown (30 sec timeout)
        ↓
   Flush all pending data to TRADE_HISTORY
        ↓
   UPDATE DAEMON_STATE SET STATUS = 'STOPPED'
        ↓
   Close DB connections
        ↓
   Exit process
```

---

## 💾 Параметры подключения (из config.yaml)

```yaml
databases:
     system:
          engine: mysql
          mysql:
               host: localhost
               port: 3306
               user: ct_system_user
               password: *****
               database: ct_system
     audit:
          engine: ""
     quotes:
          engine: clickhouse
          clickhouse:
               host: clickhouse.internal
               port: 9000
               database: analytics
               username: default
               password: *****
               tls:
                    enabled: true
                    skip_verify: false
               pool:
                    connect_timeout: 10
                    max_batch_size: 10000
                    replication_factor: 2      # Для репликации в кластере
               retry:
                    max_attempts: 3

role:
     type: both                     # monitor, trader, или both
     monitoring_config_id: 1        # ID из MONITORING таблицы
     trade_config_id: 1             # ID из TRADE таблицы
     daemon_name: prod-daemon-1     # Для DAEMON_STATE

```

---

## 🎯 Запросы для мониторинга (Phase 1.4)

```sql
-- 1. Посмотреть текущее состояние всех демонов
SELECT daemon_name, status, role, last_heartbeat, 
       CAST(active_monitoring_id AS CHAR) as mon_id,
       CAST(active_trade_id AS CHAR) as trade_id
FROM DAEMON_STATE
ORDER BY last_heartbeat DESC;

-- 2. Найти мертвые демоны (не обновлялись > 5 минут)
SELECT daemon_name, last_heartbeat, status
FROM DAEMON_STATE
WHERE last_heartbeat < (UNIX_TIMESTAMP() * 1000000 - 300000000)
  AND status = 'RUNNING';

-- 3. Статистика сделок за день
SELECT DATE(FROM_UNIXTIME(executed_time / 1000000)) as trade_date,
       trade_id, side, COUNT(*) as count,
       SUM(quantity) as total_qty, AVG(price) as avg_price,
       SUM(CAST(profit_loss AS DECIMAL)) as total_pnl
FROM TRADE_HISTORY
WHERE executed_time > (UNIX_TIMESTAMP(CURDATE()) * 1000000)
GROUP BY DATE(FROM_UNIXTIME(executed_time / 1000000)), trade_id, side;

-- 4. Конфигурация мониторинга с парами
SELECT m.id, m.orderbook_depth, m.batch_size, m.batch_interval_sec,
       m.ring_buffer_size, m.save_interval_sec,
       GROUP_CONCAT(stp.pair_id) as pair_ids
FROM MONITORING m
LEFT JOIN MONITORING_SPOT_ARRAYS msa ON m.id = msa.monitor_id
LEFT JOIN SPOT_TRADE_PAIR stp ON msa.pair_id = stp.id
WHERE m.active = 1
GROUP BY m.id;

-- 5. Конфигурация торговли с параметрами
SELECT t.id, t.uid, t.max_open_orders, t.max_position_size,
       t.default_strategy, t.strategy_update_interval_sec,
       t.slippage_percent, t.enable_backtest,
       GROUP_CONCAT(CONCAT(stp.id, ':', ea.account_name)) as accounts_pairs
FROM TRADE t
LEFT JOIN TRADE_SPOT_ARRAYS tsa ON t.id = tsa.trade_id
LEFT JOIN SPOT_TRADE_PAIR stp ON tsa.pair_id = stp.id
LEFT JOIN EXCHANGE_ACCOUNTS ea ON tsa.eaid = ea.id
WHERE t.active = 1
GROUP BY t.id;
```

