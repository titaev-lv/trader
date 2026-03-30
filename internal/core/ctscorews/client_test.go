package ctscorews

import (
	"context"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"trader/internal/config"
)

func TestNextOutboundSeqAck(t *testing.T) {
	c := New(config.CoreWSConfig{}, nil)

	seq, ack := c.nextOutboundSeqAck()
	if seq != 1 || ack != 0 {
		t.Fatalf("expected first outbound (seq=1 ack=0), got (seq=%d ack=%d)", seq, ack)
	}

	expected, ok, gap := c.observeInboundSeq(1)
	if !ok || gap {
		t.Fatalf("expected inbound seq=1 accepted, got expected=%d ok=%v gap=%v", expected, ok, gap)
	}

	seq, ack = c.nextOutboundSeqAck()
	if seq != 2 || ack != 1 {
		t.Fatalf("expected second outbound (seq=2 ack=1), got (seq=%d ack=%d)", seq, ack)
	}
}

func TestPingSeqDoesNotAdvanceOutboundEnvelopeSeq(t *testing.T) {
	c := New(config.CoreWSConfig{}, nil)

	seq, ack := c.nextOutboundSeqAck()
	if seq != 1 || ack != 0 {
		t.Fatalf("expected first outbound (seq=1 ack=0), got (seq=%d ack=%d)", seq, ack)
	}

	pingSeq, pingAck := c.nextPingSeqAck()
	if pingSeq != 1 || pingAck != 0 {
		t.Fatalf("expected first ping payload (seq=1 ack=0), got (seq=%d ack=%d)", pingSeq, pingAck)
	}

	seq, ack = c.nextOutboundSeqAck()
	if seq != 2 || ack != 0 {
		t.Fatalf("expected second outbound to remain seq=2 after ping, got (seq=%d ack=%d)", seq, ack)
	}
}

func TestClassifyReconnectReasonClose4009(t *testing.T) {
	reason := classifyReconnectReason(&websocket.CloseError{Code: 4009, Text: "seq gap"})
	if reason != reconnectReasonClose4009 {
		t.Fatalf("expected %q, got %q", reconnectReasonClose4009, reason)
	}
}

func TestClassifyReconnectReasonFromWrappedError(t *testing.T) {
	err := fmt.Errorf("read ws message: %w", &websocket.CloseError{Code: 1006, Text: "abnormal closure"})
	reason := classifyReconnectReason(err)
	if reason != "ws_close_1006" {
		t.Fatalf("expected ws_close_1006, got %q", reason)
	}
}

func TestClassifyReconnectReasonContextCanceled(t *testing.T) {
	reason := classifyReconnectReason(context.Canceled)
	if reason != "context_canceled" {
		t.Fatalf("expected context_canceled, got %q", reason)
	}
}

func TestReconnectLogLevelEscalation(t *testing.T) {
	c := New(config.CoreWSConfig{}, nil)

	if lvl := c.reconnectLogLevel("context_canceled", 1, 1); lvl != slog.LevelInfo {
		t.Fatalf("expected info for context_canceled, got %v", lvl)
	}

	if lvl := c.reconnectLogLevel(reconnectReasonClose4009, 1, 1); lvl != slog.LevelError {
		t.Fatalf("expected error for close_4009, got %v", lvl)
	}

	if lvl := c.reconnectLogLevel("dial_error", 1, 1); lvl != slog.LevelWarn {
		t.Fatalf("expected warn for dial_error baseline, got %v", lvl)
	}

	if lvl := c.reconnectLogLevel("dial_error", reconnectErrorTotalThreshold, 1); lvl != slog.LevelError {
		t.Fatalf("expected error when total exceeds threshold, got %v", lvl)
	}

	if lvl := c.reconnectLogLevel("dial_error", 1, reconnectErrorReasonThreshold); lvl != slog.LevelError {
		t.Fatalf("expected error when reason exceeds threshold, got %v", lvl)
	}
}

func TestBuildTLSConfigUsesURLServerName(t *testing.T) {
	caPath, certPath, keyPath := writeTLSMaterialFiles(t)

	cfg := config.CoreWSConfig{
		URL: "wss://core.example/ws",
		TLS: config.CoreWSTLSConfig{
			CAPath:   caPath,
			CertPath: certPath,
			KeyPath:  keyPath,
		},
	}

	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg == nil {
		t.Fatalf("expected tls config, got nil")
	}
	if tlsCfg.ServerName != "core.example" {
		t.Fatalf("expected server name core.example, got %q", tlsCfg.ServerName)
	}
	if tlsCfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected min version tls1.3, got %v", tlsCfg.MinVersion)
	}
	if tlsCfg.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify=false")
	}
}

func TestBuildTLSConfigRejectsPlainWS(t *testing.T) {
	caPath, certPath, keyPath := writeTLSMaterialFiles(t)

	cfg := config.CoreWSConfig{
		URL: "ws://core.example/ws",
		TLS: config.CoreWSTLSConfig{
			CAPath:   caPath,
			CertPath: certPath,
			KeyPath:  keyPath,
		},
	}

	if _, err := buildTLSConfig(cfg); err == nil {
		t.Fatalf("expected error for ws scheme")
	}
}

func TestBuildTLSConfigMissingKeyPair(t *testing.T) {
	caPath, certPath, _ := writeTLSMaterialFiles(t)

	cfg := config.CoreWSConfig{
		URL: "wss://core.example/ws",
		TLS: config.CoreWSTLSConfig{CAPath: caPath, CertPath: certPath},
	}

	if _, err := buildTLSConfig(cfg); err == nil {
		t.Fatalf("expected error for missing key_path")
	}
}

func TestPingStatsRTT(t *testing.T) {
	c := New(config.CoreWSConfig{}, nil)

	// simulate ping just happened
	c.pingMu.Lock()
	c.lastPingAt = time.Now().Add(-5 * time.Millisecond)
	c.pingMu.Unlock()

	c.touchPong("")

	stats := c.PingStats()
	if stats.LastRTT <= 0 {
		t.Fatalf("expected RTT > 0, got %v", stats.LastRTT)
	}
	if stats.LastPongAt.IsZero() {
		t.Fatalf("expected lastPongAt set")
	}
}

func TestWSWriteTimeoutUsesConfiguredValue(t *testing.T) {
	c := New(config.CoreWSConfig{WriteTimeout: 17 * time.Second}, nil)

	if got := c.wsWriteTimeout(); got != 17*time.Second {
		t.Fatalf("expected configured write timeout 17s, got %s", got)
	}
}

func TestWSWriteTimeoutFallsBackToDefault(t *testing.T) {
	tests := []time.Duration{0, -1 * time.Second}
	for _, in := range tests {
		c := New(config.CoreWSConfig{WriteTimeout: in}, nil)
		if got := c.wsWriteTimeout(); got != defaultWriteTimeout {
			t.Fatalf("expected default write timeout %s for input %s, got %s", defaultWriteTimeout, in, got)
		}
	}
}

func TestCloseConnWaitsForPeerCloseFrame(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	peerCloseSeen := make(chan struct{}, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, _, readErr := conn.ReadMessage()
			if readErr != nil {
				if _, ok := readErr.(*websocket.CloseError); ok {
					peerCloseSeen <- struct{}{}
				}
				return
			}
		}
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	c := New(config.CoreWSConfig{WriteTimeout: 200 * time.Millisecond}, nil)
	if err := c.closeConn(conn, websocket.CloseNormalClosure, "shutdown"); err != nil {
		t.Fatalf("closeConn: %v", err)
	}

	select {
	case <-peerCloseSeen:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected peer to observe close frame")
	}
}

func TestCloseConnTimeoutWhenPeerDoesNotRespond(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	releasePeer := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		<-releasePeer
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()
	defer close(releasePeer)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	c := New(config.CoreWSConfig{WriteTimeout: 60 * time.Millisecond}, nil)
	err = c.closeConn(conn, websocket.CloseNormalClosure, "shutdown")
	if err == nil {
		t.Fatalf("expected close handshake timeout error")
	}
	if !strings.Contains(err.Error(), "close handshake timeout") {
		t.Fatalf("expected close handshake timeout error, got %v", err)
	}
}

func TestReconnectBackoffWithJitterAndReset(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	b := newReconnectBackoff(config.CoreWSConfig{ReconnectDelaySec: 1}, r)

	d1 := b.next(5 * time.Second)
	if d1 < time.Duration(float64(time.Second)*1.1) || d1 > time.Duration(float64(time.Second)*1.2) {
		t.Fatalf("expected first backoff ~1.1-1.2s, got %v", d1)
	}

	d2 := b.next(5 * time.Second)
	if d2 < time.Duration(float64(2*time.Second)*1.1) || d2 > time.Duration(float64(2*time.Second)*1.2) {
		t.Fatalf("expected second backoff ~2.2-2.4s, got %v", d2)
	}

	d3 := b.next(5 * time.Second)
	d3Base := 3 * time.Second
	if d3 < time.Duration(float64(d3Base)*1.1) || d3 > time.Duration(float64(d3Base)*1.2) {
		t.Fatalf("expected third backoff ~3.3-3.6s, got %v", d3)
	}

	d4 := b.next(65 * time.Second) // stable session uptime triggers reset
	if d4 < time.Duration(float64(time.Second)*1.1) || d4 > time.Duration(float64(time.Second)*1.2) {
		t.Fatalf("expected reset backoff ~1.1-1.2s after long uptime, got %v", d4)
	}
}

func TestReconnectMetricsCounter4009(t *testing.T) {
	c := New(config.CoreWSConfig{}, nil)

	total, reasonCount, close4009 := c.incrementReconnectReason(reconnectReasonClose4009)
	if total != 1 || reasonCount != 1 || close4009 != 1 {
		t.Fatalf("unexpected counters after first 4009: total=%d reason=%d close4009=%d", total, reasonCount, close4009)
	}

	total, reasonCount, close4009 = c.incrementReconnectReason(reconnectReasonClose4009)
	if total != 2 || reasonCount != 2 || close4009 != 2 {
		t.Fatalf("unexpected counters after second 4009: total=%d reason=%d close4009=%d", total, reasonCount, close4009)
	}

	metrics := c.ReconnectMetrics()
	if metrics.Total != 2 {
		t.Fatalf("expected metrics total=2, got %d", metrics.Total)
	}
	if metrics.Close4009SeqGap != 2 {
		t.Fatalf("expected close_4009_seq_gap=2, got %d", metrics.Close4009SeqGap)
	}
	if metrics.ByReason[reconnectReasonClose4009] != 2 {
		t.Fatalf("expected reason counter=2, got %d", metrics.ByReason[reconnectReasonClose4009])
	}
}

func TestObserveInboundSeqGap(t *testing.T) {
	c := New(config.CoreWSConfig{}, nil)

	if _, ok, gap := c.observeInboundSeq(1); !ok || gap {
		t.Fatalf("expected first seq=1 to pass")
	}

	expected, ok, gap := c.observeInboundSeq(3)
	if ok || !gap {
		t.Fatalf("expected gap for seq=3, got expected=%d ok=%v gap=%v", expected, ok, gap)
	}
	if expected != 2 {
		t.Fatalf("expected expected_seq=2, got %d", expected)
	}
}

func TestObserveInboundEnvelopeGapError(t *testing.T) {
	c := New(config.CoreWSConfig{}, nil)

	if shouldProcess, err := c.observeInboundEnvelope(envelope{Seq: 1}); err != nil {
		t.Fatalf("expected seq=1 to pass, got %v", err)
	} else if !shouldProcess {
		t.Fatalf("expected seq=1 to be processed")
	}

	_, err := c.observeInboundEnvelope(envelope{Seq: 3})
	if err == nil {
		t.Fatalf("expected sequence gap error")
	}
	if !strings.Contains(err.Error(), errSequenceGap.Error()) {
		t.Fatalf("expected errSequenceGap in error, got %v", err)
	}
}

func TestObserveInboundEnvelopeDuplicateIgnored(t *testing.T) {
	c := New(config.CoreWSConfig{}, nil)

	if shouldProcess, err := c.observeInboundEnvelope(envelope{Seq: 1}); err != nil {
		t.Fatalf("expected seq=1 to pass, got %v", err)
	} else if !shouldProcess {
		t.Fatalf("expected seq=1 to be processed")
	}

	shouldProcess, err := c.observeInboundEnvelope(envelope{Seq: 1})
	if err != nil {
		t.Fatalf("expected duplicate seq to be ignored without error, got %v", err)
	}
	if shouldProcess {
		t.Fatalf("expected duplicate seq to be ignored")
	}
}

func TestSendRegisterIncludesSeq(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	received := make(chan envelope, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return
		}
		received <- env
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	c := New(config.CoreWSConfig{
		Version: "2.0.2",
		Region:  "local",
	}, nil)

	if err := c.sendRegister(conn); err != nil {
		t.Fatalf("sendRegister: %v", err)
	}

	select {
	case env := <-received:
		if env.Action != "trader.register" {
			t.Fatalf("expected action trader.register, got %q", env.Action)
		}
		if env.Seq != 1 {
			t.Fatalf("expected seq=1, got %d", env.Seq)
		}
		if env.Ack != 0 {
			t.Fatalf("expected ack=0 for first register, got %d", env.Ack)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for register envelope")
	}
}

func writeTLSMaterialFiles(t *testing.T) (caPath, certPath, keyPath string) {
	t.Helper()

	key, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "trader-test-client"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	der, err := x509.CreateCertificate(crand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	dir := t.TempDir()
	caPath = dir + "/ca.crt"
	certPath = dir + "/client.crt"
	keyPath = dir + "/client.key"

	if err := os.WriteFile(caPath, certPEM, 0o600); err != nil {
		t.Fatalf("write ca file: %v", err)
	}
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert file: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	return caPath, certPath, keyPath
}
