# Changelog

## v0.0.1 - 2026-03-31

### Новые возможности
- Добавлен базовый outbound WS-клиент к CTS-Core (`trader.register`, `trader.heartbeat`) и детерминированный ingress задач для `task.assign` / `task.update` / `task.remove`.
- Добавлен runtime loop менеджера с event-driven обновлениями и периодическим reconcile-контуром.
- Унифицирована runtime-конфигурация (YAML + env overrides).
- Выравнено структурированное логирование потоков (`error`, `out_request`, `ws_in`, `ws_out`, `audit`) с отдельными stdout-флагами.

### Исправления безопасности
- Для Core WS включен строгий TLS-режим (TLS 1.3, CA + клиентский cert/key для mTLS-пути).
- Из конфигурационной поверхности Trader удалена небезопасная опция Core WS `skip_verify`.
- Усилен WS transport sequencing: duplicate inbound `seq` обрабатывается идемпотентно, `seq` gap переводит клиент в reconnect flow.

### Надежность
- Реализован bounded graceful close handshake для WS-сессий.
- Стратегия reconnect переведена на linear backoff с jitter.
- Выравнены дефолты WS transport, используемые в runtime Trader (включая baseline write timeout).
- Runtime state-артефакты исключены из git-трекинга.

### Сборка и релизы
- В startup-лог добавлены метаданные сборки: `release`, `commit`, `build_time`.
- Идентификация релиза переведена на git-tag-first политику:
  - точный tag на `HEAD` => release build,
  - коммиты после последнего tag => `${last_tag}-dev.${commits_since_tag}+${utc_timestamp}.${short_sha}`,
  - отсутствие tag в репозитории => ошибка сборки.
- Удален fallback на `VERSION` и удален файл `VERSION`.
- Опубликован первый тегированный релиз Trader: `v0.0.1`.

### Тестирование
- Расширено покрытие WS/client/config для transport hardening сценариев (sequence handling, reconnect behavior, TLS strictness, deprecated config guards).
- На состоянии релиза проходит `go test ./...`.

### Документация
- Обновлены архитектурные/плановые документы под WS-first направление runtime и текущий integration baseline.
- README синхронизирован с tag-driven политикой release metadata.
