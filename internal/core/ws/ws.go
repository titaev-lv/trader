package ws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"trader/internal/logger"
)

const (
	correlationTTL             = 24 * time.Hour
	correlationCleanupInterval = 1 * time.Hour
)

type correlationEntry struct {
	requestID string
	createdAt time.Time
}

// Pool управляет пулом WebSocket соединений
type Pool struct {
	mu               sync.RWMutex
	eventToRequestID map[string]correlationEntry
	outReqLog        *slog.Logger
	wsExchangesLog   *slog.Logger
}

// NewPool создает новый WS pool с единым логгером канала бирж ws_exchanges
func NewPool() *Pool {
	exchangeLog := logger.GetWSExchanges("ws_exchanges")
	pool := &Pool{
		eventToRequestID: make(map[string]correlationEntry),
		outReqLog:        logger.GetOutRequest("ws"),
		wsExchangesLog:   exchangeLog,
	}

	go pool.correlationCleanupLoop()
	return pool
}

// Subscribe подписывает на пары
func (p *Pool) Subscribe(exchangeID, marketType string, pairs []string, depth int) error {
	_, err := p.SubscribeWithRequestID(exchangeID, marketType, pairs, depth, "")
	return err
}

// SubscribeWithRequestID подписывает на пары и прокидывает request_id в ws_exchanges
// Возвращает event_id для корреляции входящих WS событий.
func (p *Pool) SubscribeWithRequestID(exchangeID, marketType string, pairs []string, depth int, requestID string) (string, error) {
	start := time.Now()
	url := fmt.Sprintf("ws://%s/%s", exchangeID, marketType)
	if len(pairs) == 0 {
		err := fmt.Errorf("pairs list is empty")
		p.logOutRequest("WS_SUBSCRIBE", "/subscribe", url, 400, time.Since(start), requestID, err)
		return "", err
	}

	eventID := newEventID("ws-sub")
	p.rememberCorrelation(eventID, requestID)
	latencyMS := float64(time.Since(start).Microseconds()) / 1000.0
	latencyField := p.buildWSLatencyField(p.wsExchangesLog, latencyMS, nil)

	p.logExchangeEvent(exchangeID, "ws subscribe",
		"event_id", eventID,
		"request_id", requestID,
		"exchange_id", exchangeID,
		"market_type", marketType,
		"pairs", strings.Join(pairs, ","),
		"depth", depth,
		"latency_ms", latencyField,
	)
	p.logOutRequest("WS_SUBSCRIBE", "/subscribe", url, 200, time.Since(start), requestID, nil)

	return eventID, nil
}

// Unsubscribe отписывает от пар
func (p *Pool) Unsubscribe(exchangeID, marketType string, pairs []string) error {
	_, err := p.UnsubscribeWithRequestID(exchangeID, marketType, pairs, "")
	return err
}

// UnsubscribeWithRequestID отписывает пары и прокидывает request_id в ws_exchanges
// Возвращает event_id для корреляции входящих WS событий.
func (p *Pool) UnsubscribeWithRequestID(exchangeID, marketType string, pairs []string, requestID string) (string, error) {
	start := time.Now()
	url := fmt.Sprintf("ws://%s/%s", exchangeID, marketType)
	if len(pairs) == 0 {
		err := fmt.Errorf("pairs list is empty")
		p.logOutRequest("WS_UNSUBSCRIBE", "/unsubscribe", url, 400, time.Since(start), requestID, err)
		return "", err
	}

	eventID := newEventID("ws-unsub")
	p.rememberCorrelation(eventID, requestID)
	latencyMS := float64(time.Since(start).Microseconds()) / 1000.0
	latencyField := p.buildWSLatencyField(p.wsExchangesLog, latencyMS, nil)

	p.logExchangeEvent(exchangeID, "ws unsubscribe",
		"event_id", eventID,
		"request_id", requestID,
		"exchange_id", exchangeID,
		"market_type", marketType,
		"pairs", strings.Join(pairs, ","),
		"latency_ms", latencyField,
	)
	p.logOutRequest("WS_UNSUBSCRIBE", "/unsubscribe", url, 200, time.Since(start), requestID, nil)

	return eventID, nil
}

func (p *Pool) logOutRequest(method, path, url string, status int, latency time.Duration, requestID string, err error) {
	if p.outReqLog == nil {
		return
	}

	includeDetailedLatency := p.outReqLog.Enabled(context.Background(), slog.LevelDebug)
	totalLatencyMS := float64(latency.Microseconds()) / 1000.0

	latencyField := any(totalLatencyMS)
	if includeDetailedLatency {
		latencyField = map[string]float64{"total": totalLatencyMS}
	}

	fields := []any{
		"method", method,
		"path", path,
		"url", url,
		"status", status,
		"latency_ms", latencyField,
		"request_id", requestID,
	}

	if err != nil {
		fields = append(fields, "error", err)
		p.outReqLog.Warn("WS request", fields...)
		return
	}

	p.outReqLog.Info("WS request", fields...)
}

// LogInboundMessage логирует входящее WS событие в ws_exchanges.
// Если request_id пустой, пытается восстановить его по event_id.
func (p *Pool) LogInboundMessage(exchangeID, marketType, messageType, eventID, requestID string, payloadSize int, status string) {
	inboundStart := time.Now()
	latencyBreakdown := map[string]float64{}
	if requestID == "" && eventID != "" {
		correlatedRequestID, correlationLatencyMS, correlated := p.requestIDByEvent(eventID)
		requestID = correlatedRequestID
		if correlated {
			latencyBreakdown["correlation"] = correlationLatencyMS
		}
	}

	inboundLatencyMS := float64(time.Since(inboundStart).Microseconds()) / 1000.0
	latencyField := p.buildWSLatencyField(p.wsExchangesLog, inboundLatencyMS, latencyBreakdown)

	p.logExchangeEvent(exchangeID, "ws inbound",
		"event_id", eventID,
		"request_id", requestID,
		"exchange_id", exchangeID,
		"market_type", marketType,
		"message_type", messageType,
		"payload_size", payloadSize,
		"status", status,
		"latency_ms", latencyField,
	)
}

func (p *Pool) logExchangeEvent(exchangeID string, msg string, fields ...any) {
	if p.wsExchangesLog != nil {
		p.wsExchangesLog.Info(msg, fields...)
	}

	if strings.TrimSpace(exchangeID) == "" {
		return
	}

	exchangeLog := logger.GetWSExchange(exchangeID, "ws_exchange")
	if exchangeLog != nil {
		exchangeLog.Info(msg, fields...)
	}
}

func (p *Pool) rememberCorrelation(eventID, requestID string) {
	if eventID == "" || requestID == "" {
		return
	}
	p.mu.Lock()
	p.eventToRequestID[eventID] = correlationEntry{
		requestID: requestID,
		createdAt: time.Now().UTC(),
	}
	p.mu.Unlock()
}

func (p *Pool) requestIDByEvent(eventID string) (string, float64, bool) {
	p.mu.Lock()
	entry, ok := p.eventToRequestID[eventID]
	if !ok {
		p.mu.Unlock()
		return "", 0, false
	}

	if time.Since(entry.createdAt) > correlationTTL {
		delete(p.eventToRequestID, eventID)
		p.mu.Unlock()
		return "", 0, false
	}

	latencyMS := float64(time.Since(entry.createdAt).Microseconds()) / 1000.0
	delete(p.eventToRequestID, eventID)
	p.mu.Unlock()
	return entry.requestID, latencyMS, true
}

func (p *Pool) buildWSLatencyField(log *slog.Logger, totalMS float64, breakdown map[string]float64) any {
	if log != nil && log.Enabled(context.Background(), slog.LevelDebug) {
		payload := map[string]float64{"total": totalMS}
		for key, value := range breakdown {
			payload[key] = value
		}
		return payload
	}

	return totalMS
}

func (p *Pool) correlationCleanupLoop() {
	ticker := time.NewTicker(correlationCleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		p.cleanupExpiredCorrelations(time.Now().UTC())
	}
}

func (p *Pool) cleanupExpiredCorrelations(now time.Time) {
	cutoff := now.Add(-correlationTTL)

	p.mu.Lock()
	for eventID, entry := range p.eventToRequestID {
		if entry.createdAt.Before(cutoff) {
			delete(p.eventToRequestID, eventID)
		}
	}
	p.mu.Unlock()
}

func newEventID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err == nil {
		return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}
