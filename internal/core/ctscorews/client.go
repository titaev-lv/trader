package ctscorews

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"trader/internal/config"
	"trader/internal/logger"
)

type HandlerFunc func(raw []byte) error

type Client struct {
	cfg      config.CoreWSConfig
	handler  HandlerFunc
	log      *slog.Logger
	wsInLog  *slog.Logger
	wsOutLog *slog.Logger
	backoff  *reconnectBackoff

	pingMu     sync.Mutex
	lastPingAt time.Time
	lastPongAt time.Time
	lastRTT    time.Duration

	writeMu   sync.Mutex
	sessionMu sync.RWMutex
	sessionID string

	seqMu       sync.Mutex
	inboundSeq  uint64
	outboundSeq uint64
	peerAck     uint64
	pingSeq     uint64

	metricsMu             sync.Mutex
	reconnectTotal        uint64
	reconnectByReason     map[string]uint64
	reconnectSeqGapClose4 uint64
}

var errSequenceGap = errors.New("ctscore ws inbound sequence gap")

const (
	reconnectReasonClose4009             = "ws_close_4009_seq_gap"
	reconnectErrorTotalThreshold  uint64 = 5
	reconnectErrorReasonThreshold uint64 = 3

	defaultPingInterval = 10 * time.Second
	defaultPongTimeout  = 30 * time.Second
	defaultWriteTimeout = 5 * time.Second
)

type reconnectBackoff struct {
	min        time.Duration
	max        time.Duration
	step       time.Duration
	current    time.Duration
	resetAfter time.Duration
	rand       *rand.Rand
}

func newReconnectBackoff(cfg config.CoreWSConfig, r *rand.Rand) *reconnectBackoff {
	min := time.Duration(cfg.ReconnectDelaySec) * time.Second
	if min <= 0 {
		min = 1 * time.Second
	}
	max := 10 * time.Second
	if min > max {
		max = min
	}
	step := 1 * time.Second
	if r == nil {
		r = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	return &reconnectBackoff{
		min:        min,
		max:        max,
		step:       step,
		resetAfter: 60 * time.Second,
		rand:       r,
	}
}

func (b *reconnectBackoff) next(uptime time.Duration) time.Duration {
	if b == nil {
		return 1 * time.Second
	}

	if uptime >= b.resetAfter {
		b.current = b.min
		return b.withJitter(b.current)
	}

	if b.current == 0 {
		b.current = b.min
	} else {
		b.current += b.step
		if b.current > b.max {
			b.current = b.max
		}
	}

	return b.withJitter(b.current)
}

func (b *reconnectBackoff) withJitter(d time.Duration) time.Duration {
	if b == nil || b.rand == nil {
		return d
	}
	jitterPct := 0.1 + b.rand.Float64()*0.1 // 10-20% jitter
	jitter := time.Duration(float64(d) * jitterPct)
	return d + jitter
}

type ReconnectMetrics struct {
	Total           uint64            `json:"total"`
	ByReason        map[string]uint64 `json:"by_reason"`
	Close4009SeqGap uint64            `json:"close_4009_seq_gap"`
}

type PingStats struct {
	LastPingAt time.Time     `json:"last_ping_at"`
	LastPongAt time.Time     `json:"last_pong_at"`
	LastRTT    time.Duration `json:"last_rtt"`
}

type envelope struct {
	Type      string          `json:"type"`
	Action    string          `json:"action"`
	Seq       uint64          `json:"seq,omitempty"`
	Ack       uint64          `json:"ack,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

func New(cfg config.CoreWSConfig, handler HandlerFunc) *Client {
	coreLog := logger.GetWSCore("ctscorews")
	return &Client{
		cfg:               cfg,
		handler:           handler,
		log:               logger.Get("ctscorews"),
		wsInLog:           coreLog,
		wsOutLog:          coreLog,
		reconnectByReason: map[string]uint64{},
		backoff:           newReconnectBackoff(cfg, nil),
	}
}

func (c *Client) Run(ctx context.Context) {
	reconnectDelay := time.Duration(c.cfg.ReconnectDelaySec) * time.Second
	if reconnectDelay <= 0 {
		reconnectDelay = 1 * time.Second
	}

	for {
		if ctx.Err() != nil {
			return
		}

		sessionStart := time.Now()
		if err := c.runSession(ctx); err != nil {
			reason := classifyReconnectReason(err)
			total, reasonCount, gap4009Count := c.incrementReconnectReason(reason)
			c.logSessionFinished(err, reason, total, reasonCount, gap4009Count)

			delay := reconnectDelay
			if c.backoff != nil {
				delay = c.backoff.next(time.Since(sessionStart))
			}

			if ctx.Err() != nil {
				return
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}

		if ctx.Err() != nil {
			return
		}
	}
}

func (c *Client) runSession(ctx context.Context) error {
	dialer, err := c.buildDialer()
	if err != nil {
		return err
	}

	conn, _, err := dialer.DialContext(ctx, c.cfg.URL, nil)
	if err != nil {
		return fmt.Errorf("dial cts-core ws: %w", err)
	}
	defer func() {
		if err := c.closeConn(conn, websocket.CloseNormalClosure, "shutdown"); err != nil {
			c.log.Debug("ws close handshake incomplete", "error", err)
		}
		conn.Close()
	}()

	c.log.Info("cts-core ws connected", "url", c.cfg.URL)
	c.setSessionID("")
	c.resetSequenceState()
	c.prepareConn(conn)

	if err := c.sendRegister(conn); err != nil {
		return err
	}

	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	go c.heartbeatLoop(hbCtx, conn)
	go c.pingLoop(hbCtx, conn)

	for {
		if ctx.Err() != nil {
			return nil
		}

		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read ws message: %w", err)
		}

		env, ok := decodeEnvelope(raw)
		if ok {
			if shouldLogBusinessActionInfo(env.Action) {
				c.wsInLog.Info(string(raw), "direction", "in", "action", env.Action, "seq", env.Seq, "ack", env.Ack)
			} else {
				c.wsInLog.Debug(string(raw), "direction", "in", "action", env.Action, "seq", env.Seq, "ack", env.Ack)
			}
			shouldProcess, gapErr := c.observeInboundEnvelope(*env)
			if gapErr != nil {
				c.log.Warn("cts-core ws sequence gap detected, reconnecting", "error", gapErr)
				return gapErr
			}
			if !shouldProcess {
				continue
			}
			if env.Action == "trader.register_ack" {
				c.captureSessionID(*env)
			}

			if !strings.HasPrefix(env.Action, "task.") {
				continue
			}
		} else if !isTaskEnvelope(raw) {
			continue
		} else {
			c.wsInLog.Debug(string(raw), "direction", "in")
		}

		if c.handler == nil {
			continue
		}

		if err := c.handler(raw); err != nil {
			c.log.Warn("failed to apply task envelope", "error", err)
		} else {
			c.log.Debug("task envelope applied", "source", "ctscorews")
		}
	}
}

func (c *Client) heartbeatLoop(ctx context.Context, conn *websocket.Conn) {
	interval := time.Duration(c.cfg.HeartbeatIntervalSec) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(conn); err != nil {
				c.log.Warn("heartbeat send failed", "error", err)
				return
			}
		}
	}
}

func (c *Client) sendRegister(conn *websocket.Conn) error {
	payload := map[string]any{
		"region": c.cfg.Region,
	}
	if release := strings.TrimSpace(c.cfg.Release); release != "" {
		payload["release"] = release
	}

	seq, ack := c.nextOutboundSeqAck()
	env := map[string]any{
		"type":             "request",
		"action":           "trader.register",
		"protocol_version": c.cfg.ProtocolVersion,
		"seq":              seq,
		"request_id":       fmt.Sprintf("reg-%d", time.Now().UTC().UnixNano()),
		"payload":          payload,
	}
	if ack > 0 {
		env["ack"] = ack
	}

	return c.writeJSON(conn, env)
}

func (c *Client) sendHeartbeat(conn *websocket.Conn) error {
	payload := map[string]any{
		"status": "active",
	}
	if sid := c.getSessionID(); sid != "" {
		payload["session_id"] = sid
	}

	seq, ack := c.nextOutboundSeqAck()
	env := map[string]any{
		"type":    "event",
		"action":  "trader.heartbeat",
		"seq":     seq,
		"payload": payload,
	}
	if ack > 0 {
		env["ack"] = ack
	}

	return c.writeJSON(conn, env)
}

func (c *Client) writeJSON(conn *websocket.Conn, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(c.wsWriteTimeout()))
	if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
		return err
	}

	if action, _ := payload["action"].(string); action != "" {
		if shouldLogBusinessActionInfo(action) {
			c.wsOutLog.Info(string(raw), "direction", "out", "action", action, "seq", payload["seq"], "ack", payload["ack"])
		} else {
			c.wsOutLog.Debug(string(raw), "direction", "out", "action", action, "seq", payload["seq"], "ack", payload["ack"])
		}
	}
	return nil
}

func shouldLogBusinessActionInfo(action string) bool {
	action = strings.TrimSpace(action)
	if action == "" {
		return false
	}

	if action == "trader.heartbeat_ack" {
		return false
	}

	if strings.HasPrefix(action, "task.") {
		return true
	}

	switch action {
	case "trader.register", "trader.register_ack", "trader.heartbeat", "latency.test", "latency.test_result", "latency.test_result_ack", "error":
		return true
	default:
		return false
	}
}

func (c *Client) captureSessionID(env envelope) {
	if env.Action != "trader.register_ack" || len(env.Payload) == 0 {
		return
	}

	var payload struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return
	}
	if payload.SessionID == "" {
		return
	}
	c.setSessionID(payload.SessionID)
	c.log.Info("received register ack", "session_id", payload.SessionID)
}

func (c *Client) setSessionID(sessionID string) {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	c.sessionID = sessionID
}

func (c *Client) getSessionID() string {
	c.sessionMu.RLock()
	defer c.sessionMu.RUnlock()
	return c.sessionID
}

func isTaskEnvelope(raw []byte) bool {
	var msg struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return false
	}
	return strings.HasPrefix(msg.Action, "task.")
}

func decodeEnvelope(raw []byte) (*envelope, bool) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, false
	}
	if env.Action == "" {
		return nil, false
	}
	return &env, true
}

func (c *Client) observeInboundEnvelope(env envelope) (bool, error) {
	if env.Ack > 0 {
		c.observePeerAck(env.Ack)
	}

	if env.Seq == 0 {
		return true, nil
	}

	expected, ok, gap := c.observeInboundSeq(env.Seq)
	if ok {
		return true, nil
	}
	if gap {
		return false, fmt.Errorf("%w: expected=%d got=%d", errSequenceGap, expected, env.Seq)
	}

	c.log.Debug("cts-core ws duplicate inbound envelope ignored", "expected_seq", expected, "received_seq", env.Seq)
	return false, nil
}

func (c *Client) resetSequenceState() {
	c.seqMu.Lock()
	c.inboundSeq = 0
	c.outboundSeq = 0
	c.peerAck = 0
	c.pingSeq = 0
	c.seqMu.Unlock()
}

func (c *Client) nextOutboundSeqAck() (seq uint64, ack uint64) {
	c.seqMu.Lock()
	c.outboundSeq++
	seq = c.outboundSeq
	ack = c.inboundSeq
	c.seqMu.Unlock()
	return seq, ack
}

func (c *Client) nextPingSeqAck() (seq uint64, ack uint64) {
	c.seqMu.Lock()
	c.pingSeq++
	seq = c.pingSeq
	ack = c.inboundSeq
	c.seqMu.Unlock()
	return seq, ack
}

func (c *Client) observeInboundSeq(seq uint64) (expected uint64, ok bool, gap bool) {
	c.seqMu.Lock()
	defer c.seqMu.Unlock()

	if c.inboundSeq == 0 {
		expected = 1
		if seq == expected {
			c.inboundSeq = seq
			return expected, true, false
		}
		if seq > expected {
			return expected, false, true
		}
		return expected, false, false
	}

	expected = c.inboundSeq + 1
	if seq == expected {
		c.inboundSeq = seq
		return expected, true, false
	}
	if seq > expected {
		return expected, false, true
	}

	return expected, false, false
}

func (c *Client) observePeerAck(ack uint64) {
	c.seqMu.Lock()
	if ack > c.peerAck {
		c.peerAck = ack
	}
	c.seqMu.Unlock()
}

func (c *Client) closeConn(conn *websocket.Conn, code int, reason string) error {
	if conn == nil {
		return nil
	}
	if reason == "" {
		reason = "shutdown"
	}

	c.writeMu.Lock()
	deadline := time.Now().Add(c.wsWriteTimeout())
	_ = conn.SetWriteDeadline(deadline)
	msg := websocket.FormatCloseMessage(code, reason)
	writeErr := conn.WriteControl(websocket.CloseMessage, msg, deadline)
	c.writeMu.Unlock()
	if writeErr != nil {
		return writeErr
	}

	return c.awaitCloseFrame(conn)
}

func (c *Client) awaitCloseFrame(conn *websocket.Conn) error {
	if conn == nil {
		return nil
	}

	deadline := time.Now().Add(c.wsCloseWaitTimeout())
	if err := conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	defer func() {
		_ = conn.SetReadDeadline(time.Time{})
	}()

	for {
		msgType, _, err := conn.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				return nil
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return fmt.Errorf("close handshake timeout: %w", err)
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		if msgType == websocket.CloseMessage {
			return nil
		}
	}
}

func (c *Client) wsCloseWaitTimeout() time.Duration {
	wait := c.wsWriteTimeout()
	if wait <= 0 {
		wait = defaultWriteTimeout
	}
	if wait > 2*time.Second {
		return 2 * time.Second
	}
	return wait
}

func (c *Client) buildDialer() (*websocket.Dialer, error) {
	dialer := *websocket.DefaultDialer

	tlsCfg, err := buildTLSConfig(c.cfg)
	if err != nil {
		return nil, err
	}
	if tlsCfg != nil {
		dialer.TLSClientConfig = tlsCfg
	}

	return &dialer, nil
}

func buildTLSConfig(cfg config.CoreWSConfig) (*tls.Config, error) {
	parsedURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse ws url: %w", err)
	}
	if parsedURL.Scheme != "wss" {
		return nil, fmt.Errorf("core ws url must use wss scheme")
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: false,
		MinVersion:         tls.VersionTLS13,
	}

	serverName := cfg.TLS.ServerName
	if serverName == "" {
		serverName = parsedURL.Hostname()
	}
	if serverName != "" {
		tlsCfg.ServerName = serverName
	}

	if strings.TrimSpace(cfg.TLS.CAPath) == "" {
		return nil, fmt.Errorf("tls ca_path is required")
	}

	caPEM, err := os.ReadFile(cfg.TLS.CAPath)
	if err != nil {
		return nil, fmt.Errorf("read tls ca file: %w", err)
	}
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(caPEM); !ok {
		return nil, fmt.Errorf("parse tls ca file: %s", cfg.TLS.CAPath)
	}
	tlsCfg.RootCAs = pool

	if strings.TrimSpace(cfg.TLS.CertPath) == "" || strings.TrimSpace(cfg.TLS.KeyPath) == "" {
		return nil, fmt.Errorf("tls client cert_path and key_path are required")
	}
	cert, err := tls.LoadX509KeyPair(cfg.TLS.CertPath, cfg.TLS.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("load tls client cert/key: %w", err)
	}
	tlsCfg.Certificates = []tls.Certificate{cert}

	return tlsCfg, nil
}

func (c *Client) prepareConn(conn *websocket.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(defaultPongTimeout))
	conn.SetPongHandler(func(appData string) error {
		c.touchPong(appData)
		return conn.SetReadDeadline(time.Now().Add(defaultPongTimeout))
	})
}

func (c *Client) pingLoop(ctx context.Context, conn *websocket.Conn) {
	interval := defaultPingInterval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.sendPing(conn); err != nil {
				c.log.Warn("ws ping failed", "error", err)
				_ = conn.Close()
				return
			}
		}
	}
}

func (c *Client) sendPing(conn *websocket.Conn) error {
	seq, ack := c.nextPingSeqAck()
	buf := make([]byte, 16)
	binary.BigEndian.PutUint64(buf[0:8], seq)
	binary.BigEndian.PutUint64(buf[8:16], ack)

	c.pingMu.Lock()
	c.lastPingAt = time.Now()
	c.pingMu.Unlock()

	deadline := time.Now().Add(c.wsWriteTimeout())
	_ = conn.SetWriteDeadline(deadline)
	if err := conn.WriteControl(websocket.PingMessage, buf, deadline); err != nil {
		return err
	}

	c.wsOutLog.Debug(controlFrameMsg("ping", seq, ack), "direction", "out", "frame", "ping", "seq", seq, "ack", ack)
	return nil
}

func (c *Client) touchPong(appData string) {
	c.pingMu.Lock()
	if !c.lastPingAt.IsZero() {
		c.lastRTT = time.Since(c.lastPingAt)
	}
	c.lastPongAt = time.Now()
	c.pingMu.Unlock()

	raw := []byte(appData)
	if len(raw) >= 16 {
		seq := binary.BigEndian.Uint64(raw[0:8])
		ack := binary.BigEndian.Uint64(raw[8:16])
		c.wsInLog.Debug(controlFrameMsg("pong", seq, ack), "direction", "in", "frame", "pong", "seq", seq, "ack", ack)
		return
	}

	c.wsInLog.Debug(controlFrameMsg("pong", 0, 0), "direction", "in", "frame", "pong")
}

func controlFrameMsg(frame string, seq uint64, ack uint64) string {
	payload := map[string]any{
		"type":  "control",
		"frame": frame,
	}
	if seq > 0 {
		payload["seq"] = seq
	}
	if ack > 0 {
		payload["ack"] = ack
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("{\"type\":\"control\",\"frame\":\"%s\"}", frame)
	}
	return string(raw)
}

func (c *Client) PingStats() PingStats {
	c.pingMu.Lock()
	stats := PingStats{
		LastPingAt: c.lastPingAt,
		LastPongAt: c.lastPongAt,
		LastRTT:    c.lastRTT,
	}
	c.pingMu.Unlock()
	return stats
}

func (c *Client) wsWriteTimeout() time.Duration {
	if c.cfg.WriteTimeout <= 0 {
		return defaultWriteTimeout
	}
	return c.cfg.WriteTimeout
}

func classifyReconnectReason(err error) string {
	if err == nil {
		return "unknown"
	}

	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		if closeErr.Code == 4009 {
			return reconnectReasonClose4009
		}
		return fmt.Sprintf("ws_close_%d", closeErr.Code)
	}

	if errors.Is(err, errSequenceGap) {
		return "local_sequence_gap"
	}
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "i/o timeout") {
		return "read_timeout"
	}
	if strings.Contains(msg, "dial") {
		return "dial_error"
	}

	return "other_error"
}

func (c *Client) logSessionFinished(err error, reason string, total, reasonCount, gap4009Count uint64) {
	level := c.reconnectLogLevel(reason, total, reasonCount)
	log := c.log.With(
		"error", err,
		"reconnect_reason", reason,
		"reconnect_total", total,
		"reconnect_reason_count", reasonCount,
		"reconnect_close_4009_seq_gap", gap4009Count,
	)

	switch level {
	case slog.LevelDebug:
		log.Debug("cts-core ws session finished")
	case slog.LevelInfo:
		log.Info("cts-core ws session finished")
	case slog.LevelWarn:
		log.Warn("cts-core ws session finished")
	default:
		log.Error("cts-core ws session finished")
	}
}

func (c *Client) reconnectLogLevel(reason string, total, reasonCount uint64) slog.Level {
	switch reason {
	case "context_canceled", "context_done", "unknown":
		return slog.LevelInfo
	case reconnectReasonClose4009, "local_sequence_gap":
		return slog.LevelError
	}

	if reasonCount >= reconnectErrorReasonThreshold || total >= reconnectErrorTotalThreshold {
		return slog.LevelError
	}

	return slog.LevelWarn
}

func (c *Client) incrementReconnectReason(reason string) (total uint64, reasonCount uint64, close4009SeqGap uint64) {
	if reason == "" {
		reason = "unknown"
	}

	c.metricsMu.Lock()
	c.reconnectTotal++
	total = c.reconnectTotal
	c.reconnectByReason[reason]++
	reasonCount = c.reconnectByReason[reason]
	if reason == reconnectReasonClose4009 {
		c.reconnectSeqGapClose4++
	}
	close4009SeqGap = c.reconnectSeqGapClose4
	c.metricsMu.Unlock()
	return total, reasonCount, close4009SeqGap
}

func (c *Client) ReconnectMetrics() ReconnectMetrics {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()

	byReason := make(map[string]uint64, len(c.reconnectByReason))
	for k, v := range c.reconnectByReason {
		byReason[k] = v
	}

	return ReconnectMetrics{
		Total:           c.reconnectTotal,
		ByReason:        byReason,
		Close4009SeqGap: c.reconnectSeqGapClose4,
	}
}
