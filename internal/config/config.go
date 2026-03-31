// Package config отвечает за загрузку и парсинг конфигурации приложения.
// Конфигурация хранится в YAML формате.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config - главная структура конфигурации приложения
// Содержит все настройки для всех компонентов демона
type Config struct {
	// Logging - параметры логирования
	Logging LogConfig `yaml:"logging"`
	// OrderBook - параметры управления книгой ордеров
	OrderBook OrderBookConfig `yaml:"orderbook"`
	// Role - роль демона: "monitor" (сбор данных), "trader" (торговля) или "both" (оба)
	Role string `yaml:"role"`
	// Monitor - конфигурация для мониторинга (используется если Role = "monitor" или "both")
	Monitor MonitorConfig `yaml:"monitor"`
	// Trader - конфигурация для торговли (используется если Role = "trader" или "both")
	Trader TraderConfig `yaml:"trader"`
	// Databases - унифицированные подключения к хранилищам
	Databases DatabasesConfig `yaml:"databases"`
	// CoreConnections - подключения к CTS-Core (WS/REST)
	CoreConnections CoreConnectionsConfig `yaml:"core_connections"`
}

type CoreConnectionsConfig struct {
	WS   CoreWSConfig   `yaml:"ws"`
	REST CoreRESTConfig `yaml:"rest"`
}

type CoreWSConfig struct {
	Enabled              bool            `yaml:"enabled"`
	URL                  string          `yaml:"url"`
	ReconnectDelaySec    int             `yaml:"reconnect_delay_sec"`
	HeartbeatIntervalSec int             `yaml:"heartbeat_interval_sec"`
	WriteTimeout         time.Duration   `yaml:"write_timeout"`
	ProtocolVersion      string          `yaml:"protocol_version"`
	Region               string          `yaml:"region"`
	Release              string          `yaml:"-"`
	TLS                  CoreWSTLSConfig `yaml:"tls"`
}

type CoreWSTLSConfig struct {
	CertPath   string `yaml:"cert_path"`
	KeyPath    string `yaml:"key_path"`
	CAPath     string `yaml:"ca_path"`
	ServerName string `yaml:"server_name"`
}

type CoreRESTConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
}

type DatabasesConfig struct {
	System DatabaseTargetConfig `yaml:"system"`
	Audit  DatabaseTargetConfig `yaml:"audit"`
	Quotes DatabaseTargetConfig `yaml:"quotes"`
}

type DatabaseTargetConfig struct {
	Engine     string           `yaml:"engine"`
	ClickHouse ClickHouseConfig `yaml:"clickhouse"`
	PostgreSQL PostgreSQLConfig `yaml:"postgresql"`
}

type PostgreSQLConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

// OrderBookConfig - настройки для управления книгой ордеров
type OrderBookConfig struct {
	// DebugLogRaw - логировать ли сырые сообщения от бирж (много данных!)
	DebugLogRaw bool `yaml:"debug_log_raw"`
	// DebugLogMsg - логировать ли обработанные сообщения (также много данных!)
	DebugLogMsg bool `yaml:"debug_log_msg"`
}

// LogConfig - конфигурация системы логирования
type LogConfig struct {
	// Level - уровень логирования (debug, info, warn, error)
	Level string `yaml:"level"`
	// Format - формат логов (json или text)
	Format string `yaml:"format"`
	// Dir - папка куда писать логи (устарело; используйте *_path)
	Dir string `yaml:"dir"`
	// ErrorPath - путь к error.log
	ErrorPath string `yaml:"error_path"`
	// OutRequestPath - путь к out_request.log
	OutRequestPath string `yaml:"out_request_path"`
	// WSCorePath - путь к ws_core.log (канал Trader <-> CTS-Core)
	WSCorePath string `yaml:"ws_core_path"`
	// WSExchangesPath - путь к ws_exchanges.log (канал Trader <-> Exchanges)
	WSExchangesPath string `yaml:"ws_exchanges_path"`
	// AuditPath - путь к audit.log
	AuditPath string `yaml:"audit_path"`
	// MaxFileSizeMB - максимальный размер одного лог файла в мегабайтах
	// При достижении размера файл ротируется с добавлением timestamp
	MaxFileSizeMB int `yaml:"max_size_mb"`
	// MaxBackups - сколько файлов хранить после ротации
	MaxBackups int `yaml:"max_backups"`
	// MaxAgeDays - сколько дней хранить rotated логи
	MaxAgeDays int `yaml:"max_age_days"`
	// Compress - сжимать rotated логи
	Compress bool `yaml:"compress"`
	// OutRequestToStdout - дублировать out_request.log в stdout
	OutRequestToStdout bool `yaml:"out_request_to_stdout"`
	// WSCoreToStdout - дублировать ws_core.log в stdout
	WSCoreToStdout bool `yaml:"ws_core_to_stdout"`
	// WSExchangesToStdout - дублировать ws_exchanges.log в stdout
	WSExchangesToStdout bool `yaml:"ws_exchanges_to_stdout"`
	// WSExchangesLogEnable - включить логирование WS потоков бирж в агрегатор ws_exchanges.log
	WSExchangesLogEnable bool `yaml:"ws_exchanges_log_enable"`
	// WSExchangeSingleLogEnable - включить отдельные WS логи по каждой бирже (ws-<exchange>.log)
	WSExchangeSingleLogEnable bool `yaml:"ws_exchange_single_log_enable"`
	// AuditToStdout - дублировать audit.log в stdout
	AuditToStdout bool `yaml:"audit_to_stdout"`
}

// MonitorConfig - конфигурация для режима Monitor
// Monitor собирает данные с бирж и сохраняет их в ClickHouse для анализа
type MonitorConfig struct {
	// OrderBookDepth - глубина книги ордеров которую мониторить
	// Возможные значения: 20, 50, 0 (full depth)
	// 20 = быстро но меньше данных
	// 50 = компромисс между скоростью и полнотой
	// 0 = полная книга ордеров (медленно, много данных)
	OrderBookDepth int `yaml:"orderbook_depth"`

	// BatchSize - количество обновлений собираемых в batch перед отправкой в ClickHouse
	// Больший размер = меньше запросов к БД, но больше памяти
	// Рекомендуется 100-1000
	BatchSize int `yaml:"batch_size"`

	// BatchInterval - максимальное время в секундах между отправками batch в ClickHouse
	// Даже если не собрали полный BatchSize, отправим через это время
	// Гарантирует что данные не залеживаются более чем на N секунд
	BatchInterval int `yaml:"batch_interval"`

	// RingBufferSize - размер ring buffer для хранения исторических данных в памяти
	// Ring buffer хранит последние N обновлений для быстрого доступа без запроса к БД
	// Рекомендуется 5000-50000 в зависимости от памяти
	RingBufferSize int `yaml:"ring_buffer_size"`

	// SaveInterval - интервал сохранения данных в ClickHouse в секундах
	// Как часто Monitor запускает batch send в БД
	SaveInterval int `yaml:"save_interval"`
}

// TraderConfig - конфигурация для режима Trader
// Trader выполняет торговые стратегии на основе данных мониторинга
type TraderConfig struct {
	// MaxOpenOrders - максимальное количество открытых ордеров одновременно
	// Предотвращает излишнее накопление ордеров при сбое стратегии
	MaxOpenOrders int `yaml:"max_open_orders"`

	// MaxPositionSize - максимальный размер позиции в USDT
	// Ограничивает риск одной позиции
	MaxPositionSize float64 `yaml:"max_position_size"`

	// DefaultStrategy - стратегия по умолчанию для новых пар
	// Возможные значения: "grid", "dca", "scalp" и т.д.
	DefaultStrategy string `yaml:"default_strategy"`

	// StrategyUpdateInterval - интервал обновления стратегий в секундах
	// Как часто Trader переоценивает стратегию для каждой пары
	StrategyUpdateInterval int `yaml:"strategy_update_interval"`

	// SlippagePercent - допустимое проскальзывание в процентах при исполнении ордера
	// Если ордер исполнится хуже на больший процент - отменяется и переставляется
	SlippagePercent float64 `yaml:"slippage_percent"`

	// EnableBacktest - включить ли режим бэктестирования (тестирование без реального исполнения)
	EnableBacktest bool `yaml:"enable_backtest"`
}

// ClickHouseConfig - конфигурация для подключения к ClickHouse
// ClickHouse используется для хранения больших объемов исторических данных
// ClickHouse оптимизирована для аналитики и больших объемов данных
type ClickHouseConfig struct {
	// Host - адрес хоста ClickHouse
	Host string `yaml:"host"`

	// Port - порт ClickHouse HTTP API (обычно 8123)
	Port int `yaml:"port"`

	// Database - название базы данных в ClickHouse
	Database string `yaml:"database"`

	// Username - имя пользователя для подключения
	Username string `yaml:"username"`

	// Password - пароль для подключения
	Password string `yaml:"password"`

	// TLS - настройки TLS для подключения
	TLS TLSConfig `yaml:"tls"`

	// Pool - настройки пула/батчинга
	Pool ClickHousePoolConfig `yaml:"pool"`

	// Retry - настройки повторных попыток
	Retry RetryConfig `yaml:"retry"`

	// Compression - включить ли сжатие данных при отправке
	// Значительно снижает трафик для больших объемов данных
	Compression bool `yaml:"compression"`
}

type TLSConfig struct {
	Enabled    bool   `yaml:"enabled"`
	SkipVerify bool   `yaml:"skip_verify"`
	CertPath   string `yaml:"cert_path"`
	KeyPath    string `yaml:"key_path"`
	CAPath     string `yaml:"ca_path"`
}

type ClickHousePoolConfig struct {
	ConnectTimeout    int `yaml:"connect_timeout"`
	MaxBatchSize      int `yaml:"max_batch_size"`
	ReplicationFactor int `yaml:"replication_factor"`
}

type RetryConfig struct {
	MaxAttempts  int           `yaml:"max_attempts"`
	InitialDelay time.Duration `yaml:"initial_delay"`
	MaxDelay     time.Duration `yaml:"max_delay"`
	Multiplier   float64       `yaml:"multiplier"`
}

// Load загружает конфигурацию из YAML файла.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}
	if hasCoreWSTLSSkipVerify(data) {
		return nil, fmt.Errorf("core_connections.ws.tls.skip_verify is no longer supported; remove this key")
	}
	if hasLegacyCoreWSVersionKey(data) {
		return nil, fmt.Errorf("core_connections.ws.version is no longer supported; use core_connections.ws.protocol_version")
	}
	if hasLegacyTradeSection(data) {
		return nil, fmt.Errorf("trade section is no longer supported; remove trade.update_interval from config")
	}
	if hasLegacyWSLoggingKeys(data) {
		return nil, fmt.Errorf("logging ws_in/ws_out keys are no longer supported; use ws_core_path/ws_exchanges_path and ws_core_to_stdout/ws_exchanges_to_stdout")
	}

	c := defaultConfig()
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config: %w", err)
	}
	applyDefaults(c)
	applyEnvOverrides(c)
	if err := validateConfig(c); err != nil {
		return nil, err
	}

	return c, nil
}

func hasCoreWSTLSSkipVerify(data []byte) bool {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false
	}

	core := asStringMap(raw["core_connections"])
	if core == nil {
		return false
	}
	ws := asStringMap(core["ws"])
	if ws == nil {
		return false
	}
	tlsCfg := asStringMap(ws["tls"])
	if tlsCfg == nil {
		return false
	}

	_, exists := tlsCfg["skip_verify"]
	return exists
}

func hasLegacyCoreWSVersionKey(data []byte) bool {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false
	}

	core := asStringMap(raw["core_connections"])
	if core == nil {
		return false
	}
	ws := asStringMap(core["ws"])
	if ws == nil {
		return false
	}

	_, exists := ws["version"]
	return exists
}

func asStringMap(value interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]interface{}); ok {
		return m
	}
	if m, ok := value.(map[interface{}]interface{}); ok {
		out := make(map[string]interface{}, len(m))
		for k, v := range m {
			ks, ok := k.(string)
			if !ok {
				continue
			}
			out[ks] = v
		}
		return out
	}
	return nil
}

func hasLegacyTradeSection(data []byte) bool {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false
	}

	_, exists := raw["trade"]
	return exists
}

func hasLegacyWSLoggingKeys(data []byte) bool {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false
	}

	logging := asStringMap(raw["logging"])
	if logging == nil {
		return false
	}

	legacyKeys := []string{"ws_in_path", "ws_out_path", "ws_in_to_stdout", "ws_out_to_stdout"}
	for _, key := range legacyKeys {
		if _, exists := logging[key]; exists {
			return true
		}
	}

	return false
}

func validateConfig(c *Config) error {
	if raw, ok := os.LookupEnv("TRADER_CORE_CONNECTIONS_WS_TLS_SKIP_VERIFY"); ok && strings.TrimSpace(raw) != "" {
		return fmt.Errorf("TRADER_CORE_CONNECTIONS_WS_TLS_SKIP_VERIFY is no longer supported")
	}
	if raw, ok := os.LookupEnv("TRADER_CORE_CONNECTIONS_WS_VERSION"); ok && strings.TrimSpace(raw) != "" {
		return fmt.Errorf("TRADER_CORE_CONNECTIONS_WS_VERSION is no longer supported; use TRADER_CORE_CONNECTIONS_WS_PROTOCOL_VERSION")
	}

	legacyEnvToNew := map[string]string{
		"TRADER_LOG_WS_IN_PATH":       "TRADER_LOG_WS_CORE_PATH",
		"TRADER_LOG_WS_OUT_PATH":      "TRADER_LOG_WS_EXCHANGES_PATH",
		"TRADER_LOG_WS_IN_TO_STDOUT":  "TRADER_LOG_WS_CORE_TO_STDOUT",
		"TRADER_LOG_WS_OUT_TO_STDOUT": "TRADER_LOG_WS_EXCHANGES_TO_STDOUT",
	}
	for legacyKey, newKey := range legacyEnvToNew {
		if raw, ok := os.LookupEnv(legacyKey); ok && strings.TrimSpace(raw) != "" {
			return fmt.Errorf("%s is no longer supported; use %s", legacyKey, newKey)
		}
	}

	if c.CoreConnections.WS.WriteTimeout <= 0 || c.CoreConnections.WS.WriteTimeout > 24*time.Hour {
		return fmt.Errorf("invalid core_connections.ws.write_timeout: %s", c.CoreConnections.WS.WriteTimeout)
	}
	if c.CoreConnections.WS.Enabled {
		if strings.TrimSpace(c.CoreConnections.WS.URL) == "" {
			return fmt.Errorf("core_connections.ws.url is required when core_connections.ws.enabled=true")
		}

		parsedURL, err := url.Parse(c.CoreConnections.WS.URL)
		if err != nil || parsedURL.Scheme != "wss" {
			return fmt.Errorf("core_connections.ws.url must use wss scheme")
		}

		if strings.TrimSpace(c.CoreConnections.WS.TLS.CAPath) == "" {
			return fmt.Errorf("core_connections.ws.tls.ca_path is required when core_connections.ws.enabled=true")
		}
		if strings.TrimSpace(c.CoreConnections.WS.TLS.CertPath) == "" {
			return fmt.Errorf("core_connections.ws.tls.cert_path is required when core_connections.ws.enabled=true")
		}
		if strings.TrimSpace(c.CoreConnections.WS.TLS.KeyPath) == "" {
			return fmt.Errorf("core_connections.ws.tls.key_path is required when core_connections.ws.enabled=true")
		}
	}
	return nil
}

func defaultConfig() *Config {
	return &Config{
		Logging: LogConfig{
			Level:                     "info",
			Format:                    "json",
			Dir:                       "/var/log/trader",
			ErrorPath:                 "/var/log/trader/error.log",
			OutRequestPath:            "/var/log/trader/out_request.log",
			WSCorePath:                "/var/log/trader/ws_core.log",
			WSExchangesPath:           "/var/log/trader/ws_exchanges.log",
			AuditPath:                 "/var/log/trader/audit.log",
			MaxFileSizeMB:             10,
			MaxBackups:                10,
			MaxAgeDays:                30,
			Compress:                  false,
			OutRequestToStdout:        true,
			WSCoreToStdout:            true,
			WSExchangesToStdout:       true,
			WSExchangesLogEnable:      true,
			WSExchangeSingleLogEnable: true,
			AuditToStdout:             true,
		},
		OrderBook: OrderBookConfig{
			DebugLogRaw: false,
			DebugLogMsg: false,
		},
		Role: "monitor",
		Monitor: MonitorConfig{
			OrderBookDepth: 20,
			BatchSize:      500,
			BatchInterval:  5,
			RingBufferSize: 10000,
			SaveInterval:   5,
		},
		Trader: TraderConfig{
			MaxOpenOrders:          10,
			MaxPositionSize:        1000.0,
			DefaultStrategy:        "grid",
			StrategyUpdateInterval: 10,
			SlippagePercent:        0.5,
			EnableBacktest:         false,
		},
		Databases: DatabasesConfig{
			System: DatabaseTargetConfig{Engine: ""},
			Audit:  DatabaseTargetConfig{Engine: ""},
			Quotes: DatabaseTargetConfig{
				Engine: "clickhouse",
				ClickHouse: ClickHouseConfig{
					Host:     "localhost",
					Port:     8123,
					Database: "crypto",
					TLS:      TLSConfig{Enabled: false, SkipVerify: false},
					Pool: ClickHousePoolConfig{
						ConnectTimeout:    10,
						MaxBatchSize:      10000,
						ReplicationFactor: 1,
					},
					Retry: RetryConfig{
						MaxAttempts:  3,
						InitialDelay: 1 * time.Second,
						MaxDelay:     5 * time.Second,
						Multiplier:   2.0,
					},
					Compression: true,
				},
			},
		},
		CoreConnections: CoreConnectionsConfig{
			WS: CoreWSConfig{
				Enabled:              false,
				URL:                  "wss://localhost:8081/ws",
				ReconnectDelaySec:    1,
				HeartbeatIntervalSec: 60,
				WriteTimeout:         5 * time.Second,
				ProtocolVersion:      "1",
				Region:               "local",
				TLS:                  CoreWSTLSConfig{},
			},
			REST: CoreRESTConfig{
				Enabled: false,
				URL:     "http://localhost:8081/api/v1",
			},
		},
	}
}

func applyDefaults(c *Config) {
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "json"
	}
	if c.Logging.Dir == "" {
		c.Logging.Dir = "/var/log/trader"
	}
	if c.Logging.ErrorPath == "" {
		c.Logging.ErrorPath = filepath.Join(c.Logging.Dir, "error.log")
	}
	if c.Logging.OutRequestPath == "" {
		c.Logging.OutRequestPath = filepath.Join(c.Logging.Dir, "out_request.log")
	}
	if c.Logging.WSCorePath == "" {
		c.Logging.WSCorePath = filepath.Join(c.Logging.Dir, "ws_core.log")
	}
	if c.Logging.WSExchangesPath == "" {
		c.Logging.WSExchangesPath = filepath.Join(c.Logging.Dir, "ws_exchanges.log")
	}
	if c.Logging.AuditPath == "" {
		c.Logging.AuditPath = filepath.Join(c.Logging.Dir, "audit.log")
	}
	if c.Logging.MaxFileSizeMB == 0 {
		c.Logging.MaxFileSizeMB = 10
	}
	if c.Logging.MaxBackups == 0 {
		c.Logging.MaxBackups = 10
	}
	if c.Logging.MaxAgeDays == 0 {
		c.Logging.MaxAgeDays = 30
	}

	if c.Role == "" {
		c.Role = "monitor"
	}

	if c.Monitor.OrderBookDepth == 0 {
		c.Monitor.OrderBookDepth = 20
	}
	if c.Monitor.BatchSize == 0 {
		c.Monitor.BatchSize = 500
	}
	if c.Monitor.BatchInterval == 0 {
		c.Monitor.BatchInterval = 5
	}
	if c.Monitor.RingBufferSize == 0 {
		c.Monitor.RingBufferSize = 10000
	}
	if c.Monitor.SaveInterval == 0 {
		c.Monitor.SaveInterval = 5
	}

	if c.Trader.MaxOpenOrders == 0 {
		c.Trader.MaxOpenOrders = 10
	}
	if c.Trader.MaxPositionSize == 0 {
		c.Trader.MaxPositionSize = 1000.0
	}
	if c.Trader.DefaultStrategy == "" {
		c.Trader.DefaultStrategy = "grid"
	}
	if c.Trader.StrategyUpdateInterval == 0 {
		c.Trader.StrategyUpdateInterval = 10
	}
	if c.Trader.SlippagePercent == 0 {
		c.Trader.SlippagePercent = 0.5
	}

	if c.Databases.Quotes.Engine == "" {
		c.Databases.Quotes.Engine = "clickhouse"
	}
	if c.Databases.Quotes.ClickHouse.Host == "" {
		c.Databases.Quotes.ClickHouse.Host = "localhost"
	}
	if c.Databases.Quotes.ClickHouse.Port == 0 {
		c.Databases.Quotes.ClickHouse.Port = 8123
	}
	if c.Databases.Quotes.ClickHouse.Database == "" {
		c.Databases.Quotes.ClickHouse.Database = "crypto"
	}
	if c.Databases.Quotes.ClickHouse.Pool.ConnectTimeout == 0 {
		c.Databases.Quotes.ClickHouse.Pool.ConnectTimeout = 10
	}
	if c.Databases.Quotes.ClickHouse.Retry.MaxAttempts == 0 {
		c.Databases.Quotes.ClickHouse.Retry.MaxAttempts = 3
	}
	if c.Databases.Quotes.ClickHouse.Retry.InitialDelay == 0 {
		c.Databases.Quotes.ClickHouse.Retry.InitialDelay = 1 * time.Second
	}
	if c.Databases.Quotes.ClickHouse.Retry.MaxDelay == 0 {
		c.Databases.Quotes.ClickHouse.Retry.MaxDelay = 5 * time.Second
	}
	if c.Databases.Quotes.ClickHouse.Retry.Multiplier == 0 {
		c.Databases.Quotes.ClickHouse.Retry.Multiplier = 2.0
	}
	if c.Databases.Quotes.ClickHouse.Pool.MaxBatchSize == 0 {
		c.Databases.Quotes.ClickHouse.Pool.MaxBatchSize = 10000
	}
	if c.Databases.Quotes.ClickHouse.Pool.ReplicationFactor == 0 {
		c.Databases.Quotes.ClickHouse.Pool.ReplicationFactor = 1
	}
	if c.CoreConnections.WS.URL == "" {
		c.CoreConnections.WS.URL = "wss://localhost:8081/ws"
	}
	if c.CoreConnections.WS.ReconnectDelaySec <= 0 {
		c.CoreConnections.WS.ReconnectDelaySec = 1
	}
	if c.CoreConnections.WS.HeartbeatIntervalSec <= 0 {
		c.CoreConnections.WS.HeartbeatIntervalSec = 60
	}
	if c.CoreConnections.WS.WriteTimeout <= 0 {
		c.CoreConnections.WS.WriteTimeout = 5 * time.Second
	}
	if c.CoreConnections.WS.ProtocolVersion == "" {
		c.CoreConnections.WS.ProtocolVersion = "1"
	}
	if c.CoreConnections.WS.Region == "" {
		c.CoreConnections.WS.Region = "local"
	}
	if c.CoreConnections.REST.URL == "" {
		c.CoreConnections.REST.URL = "http://localhost:8081/api/v1"
	}
}

func applyEnvOverrides(c *Config) {
	c.Role = envString("TRADER_ROLE", c.Role)

	c.Logging.Level = envString("TRADER_LOG_LEVEL", c.Logging.Level)
	c.Logging.Format = envString("TRADER_LOG_FORMAT", c.Logging.Format)
	c.Logging.Dir = envString("TRADER_LOG_DIR", c.Logging.Dir)
	c.Logging.ErrorPath = envString("TRADER_LOG_ERROR_PATH", c.Logging.ErrorPath)
	c.Logging.OutRequestPath = envString("TRADER_LOG_OUT_REQUEST_PATH", c.Logging.OutRequestPath)
	c.Logging.WSCorePath = envString("TRADER_LOG_WS_CORE_PATH", c.Logging.WSCorePath)
	c.Logging.WSExchangesPath = envString("TRADER_LOG_WS_EXCHANGES_PATH", c.Logging.WSExchangesPath)
	c.Logging.AuditPath = envString("TRADER_LOG_AUDIT_PATH", c.Logging.AuditPath)
	c.Logging.MaxFileSizeMB = envInt("TRADER_LOG_MAX_SIZE_MB", c.Logging.MaxFileSizeMB)
	c.Logging.MaxBackups = envInt("TRADER_LOG_MAX_BACKUPS", c.Logging.MaxBackups)
	c.Logging.MaxAgeDays = envInt("TRADER_LOG_MAX_AGE_DAYS", c.Logging.MaxAgeDays)
	c.Logging.Compress = envBool("TRADER_LOG_COMPRESS", c.Logging.Compress)
	c.Logging.OutRequestToStdout = envBool("TRADER_LOG_OUT_REQUEST_TO_STDOUT", c.Logging.OutRequestToStdout)
	c.Logging.WSCoreToStdout = envBool("TRADER_LOG_WS_CORE_TO_STDOUT", c.Logging.WSCoreToStdout)
	c.Logging.WSExchangesToStdout = envBool("TRADER_LOG_WS_EXCHANGES_TO_STDOUT", c.Logging.WSExchangesToStdout)
	c.Logging.WSExchangesLogEnable = envBool("TRADER_LOG_WS_EXCHANGES_LOG_ENABLE", c.Logging.WSExchangesLogEnable)
	c.Logging.WSExchangeSingleLogEnable = envBool("TRADER_LOG_WS_EXCHANGE_SINGLE_LOG_ENABLE", c.Logging.WSExchangeSingleLogEnable)
	c.Logging.AuditToStdout = envBool("TRADER_LOG_AUDIT_TO_STDOUT", c.Logging.AuditToStdout)

	c.OrderBook.DebugLogRaw = envBool("TRADER_ORDERBOOK_DEBUG_LOG_RAW", c.OrderBook.DebugLogRaw)
	c.OrderBook.DebugLogMsg = envBool("TRADER_ORDERBOOK_DEBUG_LOG_MSG", c.OrderBook.DebugLogMsg)

	c.Databases.Quotes.Engine = envString("TRADER_DATABASES_QUOTES_ENGINE", c.Databases.Quotes.Engine)
	c.Databases.Quotes.ClickHouse.Host = envString("TRADER_DATABASES_QUOTES_CLICKHOUSE_HOST", c.Databases.Quotes.ClickHouse.Host)
	c.Databases.Quotes.ClickHouse.Port = envInt("TRADER_DATABASES_QUOTES_CLICKHOUSE_PORT", c.Databases.Quotes.ClickHouse.Port)
	c.Databases.Quotes.ClickHouse.Database = envString("TRADER_DATABASES_QUOTES_CLICKHOUSE_DATABASE", c.Databases.Quotes.ClickHouse.Database)
	c.Databases.Quotes.ClickHouse.Username = envString("TRADER_DATABASES_QUOTES_CLICKHOUSE_USERNAME", c.Databases.Quotes.ClickHouse.Username)
	c.Databases.Quotes.ClickHouse.Password = envString("TRADER_DATABASES_QUOTES_CLICKHOUSE_PASSWORD", c.Databases.Quotes.ClickHouse.Password)
	c.Databases.Quotes.ClickHouse.TLS.Enabled = envBool("TRADER_DATABASES_QUOTES_CLICKHOUSE_TLS_ENABLED", c.Databases.Quotes.ClickHouse.TLS.Enabled)
	c.Databases.Quotes.ClickHouse.TLS.SkipVerify = envBool("TRADER_DATABASES_QUOTES_CLICKHOUSE_TLS_SKIP_VERIFY", c.Databases.Quotes.ClickHouse.TLS.SkipVerify)
	c.Databases.Quotes.ClickHouse.TLS.CAPath = envString("TRADER_DATABASES_QUOTES_CLICKHOUSE_TLS_CA_PATH", c.Databases.Quotes.ClickHouse.TLS.CAPath)
	c.Databases.Quotes.ClickHouse.TLS.CertPath = envString("TRADER_DATABASES_QUOTES_CLICKHOUSE_TLS_CERT_PATH", c.Databases.Quotes.ClickHouse.TLS.CertPath)
	c.Databases.Quotes.ClickHouse.TLS.KeyPath = envString("TRADER_DATABASES_QUOTES_CLICKHOUSE_TLS_KEY_PATH", c.Databases.Quotes.ClickHouse.TLS.KeyPath)
	c.Databases.Quotes.ClickHouse.Pool.ConnectTimeout = envInt("TRADER_DATABASES_QUOTES_CLICKHOUSE_POOL_CONNECT_TIMEOUT", c.Databases.Quotes.ClickHouse.Pool.ConnectTimeout)
	c.Databases.Quotes.ClickHouse.Pool.MaxBatchSize = envInt("TRADER_DATABASES_QUOTES_CLICKHOUSE_POOL_MAX_BATCH_SIZE", c.Databases.Quotes.ClickHouse.Pool.MaxBatchSize)
	c.Databases.Quotes.ClickHouse.Pool.ReplicationFactor = envInt("TRADER_DATABASES_QUOTES_CLICKHOUSE_POOL_REPLICATION_FACTOR", c.Databases.Quotes.ClickHouse.Pool.ReplicationFactor)
	c.Databases.Quotes.ClickHouse.Retry.MaxAttempts = envInt("TRADER_DATABASES_QUOTES_CLICKHOUSE_RETRY_MAX_ATTEMPTS", c.Databases.Quotes.ClickHouse.Retry.MaxAttempts)
	c.Databases.Quotes.ClickHouse.Retry.InitialDelay = envDuration("TRADER_DATABASES_QUOTES_CLICKHOUSE_RETRY_INITIAL_DELAY", c.Databases.Quotes.ClickHouse.Retry.InitialDelay)
	c.Databases.Quotes.ClickHouse.Retry.MaxDelay = envDuration("TRADER_DATABASES_QUOTES_CLICKHOUSE_RETRY_MAX_DELAY", c.Databases.Quotes.ClickHouse.Retry.MaxDelay)
	c.Databases.Quotes.ClickHouse.Retry.Multiplier = envFloat("TRADER_DATABASES_QUOTES_CLICKHOUSE_RETRY_MULTIPLIER", c.Databases.Quotes.ClickHouse.Retry.Multiplier)

	c.CoreConnections.WS.Enabled = envBool("TRADER_CORE_CONNECTIONS_WS_ENABLED", c.CoreConnections.WS.Enabled)
	c.CoreConnections.WS.URL = envString("TRADER_CORE_CONNECTIONS_WS_URL", c.CoreConnections.WS.URL)
	c.CoreConnections.WS.ReconnectDelaySec = envInt("TRADER_CORE_CONNECTIONS_WS_RECONNECT_DELAY_SEC", c.CoreConnections.WS.ReconnectDelaySec)
	c.CoreConnections.WS.HeartbeatIntervalSec = envInt("TRADER_CORE_CONNECTIONS_WS_HEARTBEAT_INTERVAL_SEC", c.CoreConnections.WS.HeartbeatIntervalSec)
	c.CoreConnections.WS.WriteTimeout = envDuration("TRADER_CORE_CONNECTIONS_WS_WRITE_TIMEOUT", c.CoreConnections.WS.WriteTimeout)
	c.CoreConnections.WS.ProtocolVersion = envString("TRADER_CORE_CONNECTIONS_WS_PROTOCOL_VERSION", c.CoreConnections.WS.ProtocolVersion)
	c.CoreConnections.WS.Region = envString("TRADER_CORE_CONNECTIONS_WS_REGION", c.CoreConnections.WS.Region)
	c.CoreConnections.WS.TLS.CAPath = envString("TRADER_CORE_CONNECTIONS_WS_TLS_CA_PATH", c.CoreConnections.WS.TLS.CAPath)
	c.CoreConnections.WS.TLS.CertPath = envString("TRADER_CORE_CONNECTIONS_WS_TLS_CERT_PATH", c.CoreConnections.WS.TLS.CertPath)
	c.CoreConnections.WS.TLS.KeyPath = envString("TRADER_CORE_CONNECTIONS_WS_TLS_KEY_PATH", c.CoreConnections.WS.TLS.KeyPath)
	c.CoreConnections.WS.TLS.ServerName = envString("TRADER_CORE_CONNECTIONS_WS_TLS_SERVER_NAME", c.CoreConnections.WS.TLS.ServerName)

	c.CoreConnections.REST.Enabled = envBool("TRADER_CORE_CONNECTIONS_REST_ENABLED", c.CoreConnections.REST.Enabled)
	c.CoreConnections.REST.URL = envString("TRADER_CORE_CONNECTIONS_REST_URL", c.CoreConnections.REST.URL)
}

func envString(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func envFloat(key string, fallback float64) float64 {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return fallback
	}
	return parsed
}
