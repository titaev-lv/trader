package manager

import (
	"context"
	"testing"

	"trader/internal/config"
	"trader/internal/core/exchange"
	"trader/internal/task"
)

func newTestManager() *Manager {
	return New(&config.Config{
		Trade: config.TradeConfig{UpdateInterval: 60},
	})
}

func TestApplyUpdateCommandReplaceUpdatesSource(t *testing.T) {
	m := newTestManager()

	cmd := task.UpdateCommand{
		Mode: task.UpdateModeReplace,
		Data: &task.TasksData{
			MonitoringTasks: []*exchange.MonitoringTask{
				{ID: 1, ExchangeID: exchange.Binance, MarketType: exchange.MarketSpot, TradePair: "BTC/USDT", OrderbookDepth: 20},
			},
			TradingTasks: []*exchange.TradingTask{
				{ID: 2, ExchangeID: exchange.OKX, MarketType: exchange.MarketSpot, TradePair: "ETH/USDT", StrategyID: "s1"},
			},
		},
		Source:    "test",
		RequestID: "req-1",
	}

	if err := m.ApplyUpdateCommand(cmd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tasksData, err := m.loopSource.GetTasks(context.Background())
	if err != nil {
		t.Fatalf("unexpected source error: %v", err)
	}
	if len(tasksData.MonitoringTasks) != 1 || len(tasksData.TradingTasks) != 1 {
		t.Fatalf("unexpected tasks size: monitoring=%d trading=%d", len(tasksData.MonitoringTasks), len(tasksData.TradingTasks))
	}
}

func TestRunIterationEventUpdatesSnapshot(t *testing.T) {
	m := newTestManager()

	if err := m.ApplyUpdateCommand(task.UpdateCommand{
		Mode: task.UpdateModeReplace,
		Data: &task.TasksData{
			MonitoringTasks: []*exchange.MonitoringTask{
				{ID: 1, ExchangeID: exchange.Binance, MarketType: exchange.MarketSpot, TradePair: "BTC/USDT", OrderbookDepth: 20},
			},
		},
		Source: "test",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m.runIteration("event")

	snapshot := m.RuntimeSnapshot()
	if snapshot.LastTrigger != "event" {
		t.Fatalf("unexpected trigger: %s", snapshot.LastTrigger)
	}
	if snapshot.Iteration != 1 {
		t.Fatalf("unexpected iteration: %d", snapshot.Iteration)
	}
	if snapshot.LastSuccessAt == nil {
		t.Fatal("expected successful iteration timestamp")
	}
	if snapshot.LastSubscribe == 0 {
		t.Fatal("expected subscribe operations in first event iteration")
	}
}

func TestApplyUpdateEnvelopeRemoveByID(t *testing.T) {
	m := newTestManager()

	if err := m.ApplyUpdateCommand(task.UpdateCommand{
		Mode: task.UpdateModeReplace,
		Data: &task.TasksData{
			MonitoringTasks: []*exchange.MonitoringTask{
				{ID: 1, ExchangeID: exchange.Binance, MarketType: exchange.MarketSpot, TradePair: "BTC/USDT", OrderbookDepth: 20},
				{ID: 2, ExchangeID: exchange.OKX, MarketType: exchange.MarketSpot, TradePair: "ETH/USDT", OrderbookDepth: 20},
			},
		},
		Source: "test",
	}); err != nil {
		t.Fatalf("unexpected setup error: %v", err)
	}

	raw := []byte(`{"type":"request","action":"task.remove","request_id":"req-rm","payload":{"monitoring_task_ids":[1]}}`)
	if err := m.ApplyUpdateEnvelope(raw); err != nil {
		t.Fatalf("unexpected envelope apply error: %v", err)
	}

	tasksData, err := m.loopSource.GetTasks(context.Background())
	if err != nil {
		t.Fatalf("unexpected source error: %v", err)
	}
	if len(tasksData.MonitoringTasks) != 1 {
		t.Fatalf("unexpected tasks size: monitoring=%d", len(tasksData.MonitoringTasks))
	}
	if tasksData.MonitoringTasks[0].ID != 2 {
		t.Fatalf("unexpected remaining task id: %d", tasksData.MonitoringTasks[0].ID)
	}
}
