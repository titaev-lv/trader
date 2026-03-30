# Changelog

## v0.0.1 - 2026-03-31

### Features
- Introduce outbound CTS-Core WebSocket client baseline (`trader.register`, `trader.heartbeat`) and deterministic task ingress for `task.assign` / `task.update` / `task.remove` envelopes.
- Add runtime manager loop with event-driven updates and periodic reconciliation path.
- Unify runtime configuration around YAML + env overrides.
- Improve structured logging consistency across streams (`error`, `out_request`, `ws_in`, `ws_out`, `audit`) with per-stream stdout controls.

### Security
- Enforce strict CTS-Core WS TLS mode (TLS 1.3, CA + client certificate/key for mTLS path).
- Remove insecure Core WS `skip_verify` option from Trader config surface.
- Harden WS transport sequencing: duplicate inbound `seq` is handled idempotently, sequence gaps trigger reconnect flow.

### Reliability
- Implement bounded graceful close handshake for WS sessions.
- Align reconnect strategy to linear backoff with jitter.
- Align WS transport defaults used by Trader runtime (including write timeout baseline).
- Stop tracking runtime state artifacts in git.

### Build & Release
- Add startup build metadata fields in logs: `release`, `commit`, `build_time`.
- Switch release identity to git-tag-first policy:
  - exact tag on `HEAD` => release build,
  - commits after last tag => `${last_tag}-dev.${commits_since_tag}+${utc_timestamp}.${short_sha}`,
  - no tags in repository => build fails.
- Remove `VERSION` fallback and delete `VERSION` file.
- Publish first tagged Trader release: `v0.0.1`.

### Tests
- Extend WS/client/config test coverage for transport hardening paths (sequence handling, reconnect behavior, TLS strictness, deprecated config guards).
- `go test ./...` passes for release state.

### Documentation
- Refresh architecture/plan docs to reflect WS-first runtime direction and current integration baseline.
- Update README release metadata description to match tag-driven build behavior.
