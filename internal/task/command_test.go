package task

import (
	"testing"

	"trader/internal/core/exchange"
)

func TestUpdateCommandNormalizedReplaceNilPayload(t *testing.T) {
	_, err := (UpdateCommand{Mode: UpdateModeReplace}).Normalized()
	if err == nil {
		t.Fatal("expected error for nil replace payload")
	}
}

func TestUpdateCommandNormalizedReset(t *testing.T) {
	norm, err := (UpdateCommand{Mode: UpdateModeReset}).Normalized()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if norm.Data == nil {
		t.Fatal("expected data for reset command")
	}
	if norm.Data.Timestamp <= 0 {
		t.Fatal("expected normalized timestamp")
	}
	if len(norm.Data.MonitoringTasks) != 0 || len(norm.Data.TradingTasks) != 0 {
		t.Fatal("expected reset command to produce empty task slices")
	}
}

func TestUpdateCommandNormalizedReplaceClonesPayload(t *testing.T) {
	in := &TasksData{
		MonitoringTasks: []*exchange.MonitoringTask{{ID: 1, TradePair: "BTC/USDT"}},
		TradingTasks:    []*exchange.TradingTask{{ID: 2, TradePair: "ETH/USDT"}},
	}

	norm, err := (UpdateCommand{Mode: UpdateModeReplace, Data: in}).Normalized()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if norm.Data == nil {
		t.Fatal("expected normalized data")
	}
	if norm.Data == in {
		t.Fatal("expected deep copy of payload")
	}
	if len(norm.Data.MonitoringTasks) != 1 || len(norm.Data.TradingTasks) != 1 {
		t.Fatal("expected copied task entries")
	}

	in.MonitoringTasks[0].TradePair = "XRP/USDT"
	if norm.Data.MonitoringTasks[0].TradePair != "BTC/USDT" {
		t.Fatal("expected normalized payload to be immutable from source")
	}
}
