# Trader - Development Plan

> Версия: 3.1
> Дата: 2026-03-27
> Формат: короткий рабочий план без сроков

## 1. Цель

Собрать `trader` в рабочий контур: получение задач от `cts-core`, обработка рыночных данных, исполнение задач и отправка результата обратно.

## 2. Базовый статус

Сделано:

- стартовый каркас (`config -> logger -> manager -> shutdown`)
- модели задач и diff/apply подписок
- WS orchestration/logging слой
- унифицированные логи (`error/out_request/ws_in/ws_out/audit`)

Не сделано:

- полноценный exchange transport
- execution pipeline
- monitor pipeline записи market data
- полная runtime-связка модулей внутри manager

## 3. Обязательные контракты

1. Контракт с `cts-core`:
   - `trader.register -> trader.register_ack`
   - `trader.heartbeat`
   - корректная обработка protocol/version/errors
2. Runtime-поведение:
   - `cts-core` выбирает исполнителя
   - решение `buy/sell` принимает `trader`
3. Логирование:
   - JSON + correlation (`request_id/event_id`)
4. Тестирование:
   - service-local -> minimal integration -> full E2E

## 4. План работ

### Step 1. Runtime Wiring

Сделать рабочий цикл `sync -> diff -> apply` и подключить его в manager.

Готово, когда:

- цикл стабильно виден в логах
- корректный старт/стоп без утечек goroutine

### Step 2. Protocol Compliance

Довести WS-клиент до совместимости с `cts-core`.

Готово, когда:

- проходит lifecycle smoke (`register`, `heartbeat`, reconnect)
- корректно обрабатываются `INVALID_PAYLOAD`, `UNSUPPORTED_VERSION`, `RATE_LIMITED`

### Step 3. Exchange Transport

Сделать реальное подключение минимум к одной бирже и восстановление после обрыва.

Готово, когда:

- стабильный прием market data в длительном прогоне
- после reconnect подписки восстанавливаются автоматически

### Step 4. Monitor Pipeline

Сделать буферизацию и запись market data в quotes storage.

Готово, когда:

- запись стабильна
- видны метрики/логи по throughput, lag, drop

### Step 5. Trade Execution

Сделать минимальный рабочий контур исполнения задач.

Готово, когда:

- выполняется цикл `task -> execute -> result`
- повторная доставка команды не вызывает двойное исполнение

### Step 6. Reliability

Закрыть эксплуатационные риски в multi-instance сценарии.

Готово, когда:

- full-system E2E проходит в compose
- документация и runtime не расходятся

## 5. Quality Gates

- `go test ./...` и сборка бинарника проходят
- WS lifecycle совместим с `cts-core`
- JSON/correlation логи присутствуют
- graceful shutdown корректный при штатном стопе и сбоях сети

## 6. Definition of Done

Ближайший релиз готов, если одновременно выполнено:

- рабочий runtime loop `sync -> diff -> apply`
- совместимый WS lifecycle с `cts-core`
- минимум один production-like exchange path
- рабочий контур `task -> execute -> result`
- тесты и логирование соответствуют root-контракту CT-System
