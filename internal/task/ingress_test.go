package task

import (
	"testing"

	"trader/internal/core/exchange"
)

func TestDecodeUpdateCommandFromJSONAssign(t *testing.T) {
	raw := []byte(`{"type":"request","action":"task.assign","request_id":"req-1","payload":{"monitoring_tasks":[{"id":1,"exchange_id":"binance","market_type":"spot","trade_pair":"BTC/USDT"}],"trading_tasks":[{"id":2,"exchange_id":"okx","market_type":"spot","trade_pair":"ETH/USDT","strategy_id":"s1"}]}}`)

	cmd, err := DecodeUpdateCommandFromJSON(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Mode != UpdateModeReplace {
		t.Fatalf("unexpected mode: %s", cmd.Mode)
	}
	if cmd.RequestID != "req-1" {
		t.Fatalf("unexpected request id: %s", cmd.RequestID)
	}
	if cmd.Data == nil || len(cmd.Data.MonitoringTasks) != 1 || len(cmd.Data.TradingTasks) != 1 {
		t.Fatal("expected decoded monitoring and trading tasks")
	}
}

func TestDecodeUpdateCommandFromJSONRemove(t *testing.T) {
	raw := []byte(`{"type":"request","action":"task.remove","request_id":"req-2","payload":{"task_id":"task-1"}}`)

	cmd, err := DecodeUpdateCommandFromJSON(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Mode != UpdateModeReset {
		t.Fatalf("unexpected mode: %s", cmd.Mode)
	}
	if cmd.Data == nil || len(cmd.Data.MonitoringTasks) != 0 || len(cmd.Data.TradingTasks) != 0 {
		t.Fatal("expected reset payload")
	}
}

func TestDecodeUpdateCommandFromJSONWithBaseAssignUpsert(t *testing.T) {
	base := &TasksData{
		Timestamp: 1,
		MonitoringTasks: []*exchange.MonitoringTask{
			{ID: 1, ExchangeID: exchange.Binance, MarketType: exchange.MarketSpot, TradePair: "BTC/USDT", OrderbookDepth: 20},
		},
	}

	raw := []byte(`{"type":"request","action":"task.assign","request_id":"req-3","payload":{"monitoring_tasks":[{"id":2,"exchange_id":"okx","market_type":"spot","trade_pair":"ETH/USDT","orderbook_depth":20}]}}`)

	cmd, err := DecodeUpdateCommandFromJSONWithBase(raw, base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Mode != UpdateModeReplace {
		t.Fatalf("unexpected mode: %s", cmd.Mode)
	}
	if cmd.Data == nil || len(cmd.Data.MonitoringTasks) != 2 {
		t.Fatalf("expected merged monitoring tasks, got %d", len(cmd.Data.MonitoringTasks))
	}
}

func TestDecodeUpdateCommandFromJSONWithBaseRemoveByID(t *testing.T) {
	base := &TasksData{
		Timestamp: 1,
		MonitoringTasks: []*exchange.MonitoringTask{
			{ID: 1, ExchangeID: exchange.Binance, MarketType: exchange.MarketSpot, TradePair: "BTC/USDT", OrderbookDepth: 20},
			{ID: 2, ExchangeID: exchange.OKX, MarketType: exchange.MarketSpot, TradePair: "ETH/USDT", OrderbookDepth: 20},
		},
	}

	raw := []byte(`{"type":"request","action":"task.remove","request_id":"req-4","payload":{"monitoring_task_ids":[1]}}`)

	cmd, err := DecodeUpdateCommandFromJSONWithBase(raw, base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Mode != UpdateModeReplace {
		t.Fatalf("unexpected mode: %s", cmd.Mode)
	}
	if cmd.Data == nil || len(cmd.Data.MonitoringTasks) != 1 {
		t.Fatalf("expected one monitoring task after remove, got %d", len(cmd.Data.MonitoringTasks))
	}
	if cmd.Data.MonitoringTasks[0].ID != 2 {
		t.Fatalf("unexpected remaining task id: %d", cmd.Data.MonitoringTasks[0].ID)
	}
}

func TestDecodeUpdateCommandFromJSONWithBaseUpdateReplacesExisting(t *testing.T) {
	base := &TasksData{
		Timestamp: 1,
		TradingTasks: []*exchange.TradingTask{
			{ID: 10, ExchangeID: exchange.Binance, MarketType: exchange.MarketSpot, TradePair: "BTC/USDT", StrategyID: "s1"},
		},
	}

	raw := []byte(`{"type":"event","action":"task.update","request_id":"req-5","payload":{"trading_tasks":[{"id":10,"exchange_id":"binance","market_type":"spot","trade_pair":"BTC/USDT","strategy_id":"s2"}]}}`)

	cmd, err := DecodeUpdateCommandFromJSONWithBase(raw, base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Data == nil || len(cmd.Data.TradingTasks) != 1 {
		t.Fatal("expected single trading task")
	}
	if cmd.Data.TradingTasks[0].StrategyID != "s2" {
		t.Fatalf("expected updated strategy id, got %s", cmd.Data.TradingTasks[0].StrategyID)
	}
}

func TestDecodeUpdateCommandFromJSONUnsupportedAction(t *testing.T) {
	raw := []byte(`{"type":"request","action":"task.unknown","payload":{}}`)
	if _, err := DecodeUpdateCommandFromJSON(raw); err == nil {
		t.Fatal("expected error for unsupported action")
	}
}

func TestDecodeUpdateCommandFromJSONUnsupportedType(t *testing.T) {
	raw := []byte(`{"type":"response","action":"task.assign","payload":{}}`)
	if _, err := DecodeUpdateCommandFromJSON(raw); err == nil {
		t.Fatal("expected error for unsupported envelope type")
	}
}
