# Структура кода trader

Документ описывает фактическую структуру кода и степень готовности модулей.

## Версия

- Версия приложения в коде: `2.0.2` (`cmd/trader/main.go`)

## Точка входа

### `cmd/trader/main.go`

Назначение:

- загрузка конфигурации
- инициализация логирования
- запуск/остановка manager
- обработка `SIGINT/SIGTERM`

Статус: реализовано и используется в runtime.

## Системные модули

### `internal/config/config.go`

Назначение:

- чтение `conf/config.yaml`
- дефолтные значения
- env overrides

Статус: реализовано.

### `internal/logger/logger.go`

Назначение:

- `slog` логирование
- ротация файлов через `lumberjack`
- отдельные потоки: `error`, `out_request`, `ws_in`, `ws_out`, `audit`

Статус: реализовано.

### `internal/manager/manager.go`

Назначение:

- lifecycle (`Start/Stop/Status`)
- shutdown timeout
- координация остановки через context

Статус: реализовано как каркас управления.

### `internal/state/state.go`

Назначение:

- сохранение признака running/stopped

Статус: реализовано.

## Core-модули

### `internal/core/exchange/types.go`

Назначение:

- типы домена: биржи, рынки, orderbook, monitoring/trading tasks

Статус: реализовано.

### `internal/core/messaging/message.go`

Назначение:

- общий формат сообщений/метаданных

Статус: реализовано.

### `internal/core/ws/ws.go`

Назначение:

- API для `Subscribe/Unsubscribe`
- логирование исходящих/входящих WS событий
- корреляция `event_id <-> request_id` с TTL cleanup

Статус: частично реализовано.

Комментарий:

- Модуль полезен для orchestration/logging слоя.
- Полноценный production-level websocket transport с реальными биржевыми подключениями еще не завершен.

## Task-модули

### `internal/task/types.go`

Назначение:

- общие структуры входных задач (`TasksData`) для orchestration-пайплайна

Статус: реализовано.

### `internal/task/subscription_manager.go`

Назначение:

- merge/diff старого и нового состояния задач
- вычисление `subscribe/unsubscribe`
- применение через `ws.Pool`

Статус: реализовано.

## Trader/операционные модули

На текущем этапе отдельные runtime-модули трейдинга находятся в стадии доработки и собираются через roadmap-фазы.

## Что не стоит считать завершенным

На текущий момент в репозитории отсутствует подтвержденная полнофункциональная реализация следующих блоков:

- production-драйверы бирж в составе полного runtime
- end-to-end обработка orderbook потока
- полный execution layer торговых стратегий
- полноценно собранный monitor pipeline с записью в ClickHouse

Эти части следует считать roadmap-задачами, даже если они упоминаются в исторических документах.
