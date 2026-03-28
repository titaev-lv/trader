# WS Transport Skeleton Plan (CTS-Core <-> Trader)

Дата: 2026-03-28
Статус: draft для transport-first до бизнес-логики

## 1. Цель

Сначала довести надежный транспорт WS между `cts-core` и `trader`, затем поверх него включать бизнес-действия (`task.*`, `trade.result`, `monitor.result`).

## 2. Scope (только transport)

В рамках этого плана закрываем:

- mTLS подключение (клиентский сертификат обязателен).
- Удержание канала через transport ping/pong.
- Таймауты чтения/записи и корректный detect half-open.
- Безопасное закрытие соединений и cleanup без утечек.
- Reconnect у `trader` с мягким backoff.
- Sequence/ack контракт для направления сообщений (включая ping/pong).

Не закрываем в этом шаге:

- Полная бизнес-маршрутизация команд и результатов.
- Планировщик задач, routing policy и бизнес-idempotency.

## 3. Предлагаемый минимальный протокол транспорта

## 3.1 Envelope (для text/binary business frames)

```json
{
  "type": "request|event|response",
  "action": "...",
  "request_id": "...",
  "session_id": "...",
  "seq": 123,
  "ack": 120,
  "ts": 1760000000000,
  "payload": {}
}
```

Правила:

- `seq`: монотонный номер исходящего потока в рамках одного направления и соединения.
- `ack`: последний непрерывно принятый `seq` от противоположной стороны.
- На reconnect счетчики начинают новый цикл, `session_id` меняется.

## 3.2 Sequence для ping/pong

Для transport ping/pong использовать control frame payload (<=125 bytes):

- Ping payload: 16 bytes (`seq:uint64` + `ack:uint64`, big-endian).
- Pong должен вернуть тот же payload (стандарт gorilla это поддерживает).
- RTT оценивается по ping-seq.

Если прокси/инфраструктура портит payload control frames, fallback:

- держим transport ping/pong без seq,
- а sequence подтверждаем в обычных envelope полях (`seq/ack`).

## 4. Работы по CTS-Core

## 4.1 Endpoint и mTLS

- Поднять WS endpoint на TLS listener.
- В `tls.Config`:
  - `ClientAuth = RequireAndVerifyClientCert`.
  - Настроенный `ClientCAs`.
  - Проверка сертификата по SAN/CN/OU (allowlist) и mapping на `trader_id`.
- Отклонять подключение без валидного клиентского сертификата до upgrade.

## 4.2 Keepalive и таймауты

- `SetReadLimit` для payload safety.
- `SetReadDeadline(now + pong_timeout)` при connect.
- `SetPongHandler(...)` -> продлевать `ReadDeadline`.
- Периодический `WriteControl(PingMessage, payload, deadline)`.
- `SetWriteDeadline` на каждый write.

## 4.3 Lifecycle и cleanup

- На disconnect:
  - remove из всех connection maps,
  - завершить ping goroutine,
  - финализировать сессию,
  - освободить dedup/seq state.
- Корректный close handshake:
  - отправка close frame,
  - ожидание краткого grace,
  - затем force close.

## 4.4 Sequence state

- Пер-connection state:
  - `nextOutSeq`, `lastInSeq`, `lastAckedByPeer`.
- Reject/replay policy:
  - если `seq <= lastInSeq` -> duplicate/stale,
  - если gap большой -> лог + опциональный protocol error.

## 4.5 Логирование и метрики

- Логи `ws_access/ws_out` дополнить:
  - `seq`, `ack`, `ping_seq`, `rtt_ms`, `close_code`, `close_reason`, `cert_subject`, `cert_fingerprint`.
- Метрики:
  - active connections,
  - reconnects,
  - ping RTT,
  - timeout/disconnect count,
  - duplicate/stale frames.

## 5. Работы по Trader

## 5.1 Connect + mTLS

- В dialer TLS config:
  - клиентский cert/key,
  - trusted CA,
  - `ServerName`, `MinVersion`.
- Fail-fast при ошибках сертификата.

## 5.2 Keepalive и таймауты

- `SetReadDeadline` + `SetPongHandler`.
- Пинг-цикл через `WriteControl(PingMessage, payload, deadline)`.
- `SetWriteDeadline` для register/heartbeat/прочих write.

## 5.3 Reconnect policy

- Мягкий backoff: например 1s -> 2s -> 3s ... -> cap 10s (+ jitter 10-20%).
- Сброс backoff после стабильного соединения (например 60s uptime).

## 5.4 Shutdown disconnect

- На shutdown:
  - отправить close frame (`normal closure`),
  - дождаться server close/timeout,
  - закрыть socket,
  - завершить goroutines.

## 5.5 Sequence state

- Аналогично серверу: per-direction `seq/ack`.
- Sequence в register/heartbeat и во всех последующих action.

## 6. Что еще добавить (важно)

- Версионирование transport-контракта (`transport_version`) отдельно от бизнес-протокола.
- Ограничение frame size и защита от flood на обеих сторонах.
- Correlation: сквозной `request_id` + `session_id` + `seq`.
- Chaos-тесты:
  - packet loss,
  - delayed pong,
  - cert rotation,
  - abrupt close (RST),
  - duplicate frame replay.
- Наблюдаемость в health:
  - last_ping_ts,
  - last_pong_ts,
  - last_rtt_ms,
  - ws_state.

## 7. Этапы внедрения

1. `CTS-Core`: mTLS endpoint + cert validation + baseline connect/reject tests.
2. `Trader`: mTLS dial + connect/shutdown close-handshake.
3. `CTS-Core/Trader`: transport ping/pong + deadlines + timeout handling.
4. `Trader`: reconnect backoff + jitter + reset policy.
5. `CTS-Core/Trader`: sequence/ack в envelope + ping payload sequence.
6. Наблюдаемость: логи/метрики/health поля.
7. Integration tests (docker compose + cert matrix + fault injection).

## 8. Критерии готовности (DoD)

- Соединение возможно только с валидным клиентским сертификатом.
- Half-open и silent drop детектятся не позже `pong_timeout`.
- После обрыва `trader` восстанавливает соединение автоматически.
- На shutdown обе стороны завершают соединение без висящих goroutine/утечек.
- `seq/ack` присутствует во всех transport envelope и валидируется.
- Ping/pong sequence логируется и наблюдаем в метриках.
- Тесты transport слоя проходят стабильно в CI.

## 9. Рекомендуемые стартовые значения

- `ping_interval`: 10s
- `pong_timeout`: 30s
- `write_timeout`: 5s
- `heartbeat_interval` (business): 10s
- `reconnect_backoff`: 1..10s, jitter 10-20%
