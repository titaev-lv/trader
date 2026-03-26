# trader

Go-сервис для outbound-интеграции с CTS-Core и биржевыми потоками.

## Текущий статус

Документация ниже описывает фактическое состояние кода на текущий момент.

- Версия бинарника: `2.0.2` (`cmd/trader/main.go`)
- Локальный HTTP API не поднимается, сервис работает в outbound-only модели
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
internal/task/subscription_manager.go
```

## Документация

- `ARCHITECTURE.md`: фактическая архитектура и зоны развития
- `CODE_STRUCTURE.md`: назначение модулей и степень готовности
- `DEVELOPMENT_PLAN.md`: обновленный план с честными статусами

## Примечание

Если какой-то раздел старой документации противоречит коду, приоритет у кода в `cmd/` и `internal/`.
