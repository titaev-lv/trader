package ctscorews

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"trader/internal/config"
	"trader/internal/logger"
)

type HandlerFunc func(raw []byte) error

type Client struct {
	cfg     config.CoreWSConfig
	handler HandlerFunc
	log     *slog.Logger

	writeMu   sync.Mutex
	sessionMu sync.RWMutex
	sessionID string
}

func New(cfg config.CoreWSConfig, handler HandlerFunc) *Client {
	return &Client{
		cfg:     cfg,
		handler: handler,
		log:     logger.Get("ctscorews"),
	}
}

func (c *Client) Run(ctx context.Context) {
	reconnectDelay := time.Duration(c.cfg.ReconnectDelaySec) * time.Second
	if reconnectDelay <= 0 {
		reconnectDelay = 5 * time.Second
	}

	for {
		if ctx.Err() != nil {
			return
		}

		if err := c.runSession(ctx); err != nil {
			c.log.Warn("cts-core ws session finished", "error", err)
		}

		if ctx.Err() != nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}

func (c *Client) runSession(ctx context.Context) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.cfg.URL, nil)
	if err != nil {
		return fmt.Errorf("dial cts-core ws: %w", err)
	}
	defer conn.Close()

	c.log.Info("cts-core ws connected", "url", c.cfg.URL)
	c.setSessionID("")

	if err := c.sendRegister(conn); err != nil {
		return err
	}

	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	go c.heartbeatLoop(hbCtx, conn)

	for {
		if ctx.Err() != nil {
			return nil
		}

		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read ws message: %w", err)
		}

		c.captureSessionID(raw)

		if !isTaskEnvelope(raw) {
			continue
		}

		if c.handler == nil {
			continue
		}

		if err := c.handler(raw); err != nil {
			c.log.Warn("failed to apply task envelope", "error", err)
		} else {
			c.log.Info("task envelope applied", "source", "ctscorews")
		}
	}
}

func (c *Client) heartbeatLoop(ctx context.Context, conn *websocket.Conn) {
	interval := time.Duration(c.cfg.HeartbeatIntervalSec) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
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
		"trader_id": c.cfg.TraderID,
		"version":   c.cfg.Version,
		"region":    c.cfg.Region,
	}

	env := map[string]any{
		"type":       "request",
		"action":     "trader.register",
		"request_id": fmt.Sprintf("reg-%d", time.Now().UTC().UnixNano()),
		"payload":    payload,
	}

	return c.writeJSON(conn, env)
}

func (c *Client) sendHeartbeat(conn *websocket.Conn) error {
	payload := map[string]any{
		"trader_id": c.cfg.TraderID,
		"status":    "active",
	}
	if sid := c.getSessionID(); sid != "" {
		payload["session_id"] = sid
	}

	env := map[string]any{
		"type":    "event",
		"action":  "trader.heartbeat",
		"payload": payload,
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
	if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
		return err
	}

	if action, _ := payload["action"].(string); action != "" {
		c.log.Info("ws out", "action", action)
	}
	return nil
}

func (c *Client) captureSessionID(raw []byte) {
	var msg struct {
		Action  string `json:"action"`
		Payload struct {
			SessionID string `json:"session_id"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	if msg.Action != "trader.register_ack" || msg.Payload.SessionID == "" {
		return
	}
	c.setSessionID(msg.Payload.SessionID)
	c.log.Info("received register ack", "session_id", msg.Payload.SessionID)
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
