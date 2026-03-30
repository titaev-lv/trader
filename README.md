# trader

Go-сервис для outbound-интеграции с CTS-Core и биржевыми потоками.

## Текущий статус

Документация ниже описывает фактическое состояние кода на текущий момент.

- Startup-версия бинарника берется из release metadata сборки:
	- release:
		- если HEAD ровно на git tag, используется tag (release build)
		- если после последнего tag есть коммиты, используется формат `${last_tag}-dev.${commits_since_tag}+${utc_timestamp}.${short_sha}`
	- commit: short SHA
	- build_time: UTC timestamp сборки
	- если в репозитории нет ни одного tag, сборка завершается с ошибкой
- Сервис работает в outbound-first модели взаимодействия
- Реализованы базовые модули: конфиг, логирование, lifecycle manager, state, базовые типы, WS-пул (логический слой), merge/apply подписок
- Часть модулей присутствует как каркас или изолированные компоненты и еще не собрана в полноценный runtime-пайплайн

## Что действительно реализовано

- `cmd/trader/main.go`: инициализация `config -> logger -> manager`, запуск, graceful shutdown
- `internal/config/config.go`: загрузка YAML + env overrides
- `internal/logger/logger.go`: `slog` + `lumberjack`, потоки `error/out_request/ws_in/ws_out/audit`
- `internal/manager/manager.go`: lifecycle-каркас (`Start/Stop/Status`) c таймаутом shutdown
- `internal/state/state.go`: сохранение runtime-state
- `internal/core/exchange/types.go`: доменные типы и константы бирж/рынков
- `internal/core/messaging/message.go`: общие структуры сообщений
- `internal/core/ws/ws.go`: уровень логирования/корреляции WS-событий (`event_id`/`request_id`), TTL-cleanup
- `internal/task/types.go`: структуры задач для orchestration слоя
- `internal/task/source.go`: источник задач и событийные уведомления (`GetTasks`/`Watch`/`SetTasks`)
- `internal/task/subscription_manager.go`: diff и применение подписок через WS pool

## Что пока не реализовано полностью

- Полный execution pipeline торговых стратегий
- Реальные коннекторы/драйверы бирж (в текущем состоянии нет полноценных production-драйверов в `internal/core/exchange/drivers`)
- Полноценный monitor-поток с записью рыночных данных в ClickHouse
- Интеграция всех вспомогательных модулей в единый рабочий контур manager

## Быстрый старт

Требования:

- Go `1.25.4+`
- Доступная конфигурация `conf/config.yaml`

Сборка и запуск:

```bash
go mod download
go build -o trader cmd/trader/main.go
./trader -c conf/config.yaml
```

## Контракт взаимодействия с CTS-Core

- Основной runtime-канал `trader <-> cts-core`: WebSocket.
- По WS передаются: `trader.register`, `trader.heartbeat`, `metrics.report`, `task.*`, `trade.result`, `monitor.result`, диагностика.
- `trader` не пишет runtime-данные напрямую в MySQL; запись идет через `cts-core`.
- REST в экосистеме сохраняется для control-plane и recovery/replay сценариев, но не как основной hot path для runtime доставки результатов от `trader`.

Identity policy (trader WS):

- `trader_id` определяется на стороне CTS-Core из CN клиентского сертификата.
- `payload.trader_id` не используется для идентификации.
- Для подключения в trader channel нужен клиентский сертификат с `OU=Trading`.

## WS Write Timeout

- Параметр: `core_connections.ws.write_timeout`.
- Значение по умолчанию: `5s`.
- ENV override: `TRADER_CORE_CONNECTIONS_WS_WRITE_TIMEOUT`.
- Допустимый диапазон: `> 0` и `<= 24h`.

## Структура проекта (актуальная)

```text
cmd/trader/main.go
internal/config/config.go
internal/logger/logger.go
internal/state/state.go
internal/manager/manager.go
internal/core/exchange/types.go
internal/core/messaging/message.go
internal/core/ws/ws.go
internal/task/types.go
internal/task/source.go
internal/task/subscription_manager.go
```

## Документация

- `ARCHITECTURE.md`: фактическая архитектура и зоны развития
- `DEVELOPMENT_PLAN.md`: обновленный план с честными статусами

## Примечание

Если какой-то раздел старой документации противоречит коду, приоритет у кода в `cmd/` и `internal/`.
