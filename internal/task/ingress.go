package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"trader/internal/core/exchange"
)

var (
	ErrUnsupportedEnvelopeType = errors.New("unsupported envelope type")
	ErrUnsupportedAction       = errors.New("unsupported action")
	ErrEmptyPatchPayload       = errors.New("empty patch payload")
)

type inboundEnvelope struct {
	Type      string          `json:"type"`
	Action    string          `json:"action"`
	RequestID string          `json:"request_id"`
	Payload   json.RawMessage `json:"payload"`
}

type inboundTasksPayload struct {
	Timestamp         int64                   `json:"timestamp"`
	MonitoringTasks   []inboundMonitoringTask `json:"monitoring_tasks"`
	TradingTasks      []inboundTradingTask    `json:"trading_tasks"`
	MonitoringTaskIDs []int                   `json:"monitoring_task_ids"`
	TradingTaskIDs    []int                   `json:"trading_task_ids"`
	TaskID            string                  `json:"task_id"`
}

type inboundMonitoringTask struct {
	ID               int    `json:"id"`
	UID              int    `json:"uid"`
	ExchangeID       string `json:"exchange_id"`
	ExchangeName     string `json:"exchange_name"`
	MarketType       string `json:"market_type"`
	TradePairID      int    `json:"trade_pair_id"`
	TradePair        string `json:"trade_pair"`
	OrderbookDepth   int    `json:"orderbook_depth"`
	BatchSize        int    `json:"batch_size"`
	BatchIntervalSec int    `json:"batch_interval_sec"`
	RingBufferSize   int    `json:"ring_buffer_size"`
	SaveIntervalSec  int    `json:"save_interval_sec"`
}

type inboundTradingTask struct {
	ID                int    `json:"id"`
	UID               int    `json:"uid"`
	TradeType         int    `json:"trade_type"`
	ExchangeID        string `json:"exchange_id"`
	ExchangeName      string `json:"exchange_name"`
	MarketType        string `json:"market_type"`
	TradePairID       int    `json:"trade_pair_id"`
	TradePair         string `json:"trade_pair"`
	StrategyID        string `json:"strategy_id"`
	StrategyParams    string `json:"strategy_params"`
	ExchangeAccountID int    `json:"exchange_account_id"`
}

// DecodeUpdateCommandFromJSON переводит входящее WS task.* сообщение в внутреннюю команду обновления.
func DecodeUpdateCommandFromJSON(raw []byte) (UpdateCommand, error) {
	return DecodeUpdateCommandFromJSONWithBase(raw, nil)
}

// DecodeUpdateCommandFromJSONWithBase переводит входящее WS task.* сообщение в команду,
// учитывая текущее desired-state для детерминированных частичных обновлений.
func DecodeUpdateCommandFromJSONWithBase(raw []byte, base *TasksData) (UpdateCommand, error) {
	var env inboundEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return UpdateCommand{}, fmt.Errorf("decode envelope: %w", err)
	}

	if env.Type != "request" && env.Type != "event" {
		return UpdateCommand{}, ErrUnsupportedEnvelopeType
	}

	switch env.Action {
	case "task.assign", "task.update":
		return decodeUpsertCommand(env, base)
	case "task.remove":
		return decodeRemoveCommand(env, base)
	default:
		return UpdateCommand{}, ErrUnsupportedAction
	}
}

func decodeUpsertCommand(env inboundEnvelope, base *TasksData) (UpdateCommand, error) {
	payload := inboundTasksPayload{}
	if len(env.Payload) > 0 {
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return UpdateCommand{}, fmt.Errorf("decode tasks payload: %w", err)
		}
	}

	if payload.MonitoringTasks == nil && payload.TradingTasks == nil {
		return UpdateCommand{}, ErrEmptyPatchPayload
	}

	next := cloneTasksData(normalizeBaseTasks(base))
	if payload.MonitoringTasks != nil {
		next.MonitoringTasks = upsertMonitoringTasks(next.MonitoringTasks, mapMonitoringTasks(payload.MonitoringTasks))
	}
	if payload.TradingTasks != nil {
		next.TradingTasks = upsertTradingTasks(next.TradingTasks, mapTradingTasks(payload.TradingTasks))
	}

	if payload.Timestamp <= 0 {
		payload.Timestamp = time.Now().UTC().Unix()
	}
	next.Timestamp = payload.Timestamp

	return UpdateCommand{
		Mode:      UpdateModeReplace,
		Data:      next,
		Source:    "ws",
		RequestID: env.RequestID,
	}.Normalized()
}

func decodeRemoveCommand(env inboundEnvelope, base *TasksData) (UpdateCommand, error) {
	payload := inboundTasksPayload{}
	if len(env.Payload) > 0 {
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return UpdateCommand{}, fmt.Errorf("decode tasks payload: %w", err)
		}
	}

	hasSelectors := len(payload.MonitoringTaskIDs) > 0 || len(payload.TradingTaskIDs) > 0 || len(payload.MonitoringTasks) > 0 || len(payload.TradingTasks) > 0
	if !hasSelectors {
		if payload.TaskID == "" {
			return UpdateCommand{Mode: UpdateModeReset, Source: "ws", RequestID: env.RequestID}.Normalized()
		}
		if _, err := strconv.Atoi(payload.TaskID); err != nil {
			return UpdateCommand{Mode: UpdateModeReset, Source: "ws", RequestID: env.RequestID}.Normalized()
		}
	}

	next := cloneTasksData(normalizeBaseTasks(base))
	monIDs := make(map[int]struct{})
	for _, id := range payload.MonitoringTaskIDs {
		if id > 0 {
			monIDs[id] = struct{}{}
		}
	}
	for _, item := range payload.MonitoringTasks {
		if item.ID > 0 {
			monIDs[item.ID] = struct{}{}
		}
	}

	tradeIDs := make(map[int]struct{})
	for _, id := range payload.TradingTaskIDs {
		if id > 0 {
			tradeIDs[id] = struct{}{}
		}
	}
	for _, item := range payload.TradingTasks {
		if item.ID > 0 {
			tradeIDs[item.ID] = struct{}{}
		}
	}

	if taskID, err := strconv.Atoi(payload.TaskID); err == nil && taskID > 0 {
		monIDs[taskID] = struct{}{}
		tradeIDs[taskID] = struct{}{}
	}

	next.MonitoringTasks = removeMonitoringTasks(next.MonitoringTasks, monIDs)
	next.TradingTasks = removeTradingTasks(next.TradingTasks, tradeIDs)
	if payload.Timestamp > 0 {
		next.Timestamp = payload.Timestamp
	} else {
		next.Timestamp = time.Now().UTC().Unix()
	}

	return UpdateCommand{
		Mode:      UpdateModeReplace,
		Data:      next,
		Source:    "ws",
		RequestID: env.RequestID,
	}.Normalized()
}

func normalizeBaseTasks(base *TasksData) *TasksData {
	if base == nil {
		return &TasksData{
			Timestamp:       time.Now().UTC().Unix(),
			MonitoringTasks: []*exchange.MonitoringTask{},
			TradingTasks:    []*exchange.TradingTask{},
		}
	}
	return base
}

func cloneTasksData(in *TasksData) *TasksData {
	return &TasksData{
		Timestamp:       in.Timestamp,
		MonitoringTasks: cloneMonitoringTasks(in.MonitoringTasks),
		TradingTasks:    cloneTradingTasks(in.TradingTasks),
	}
}

func upsertMonitoringTasks(base []*exchange.MonitoringTask, patch []*exchange.MonitoringTask) []*exchange.MonitoringTask {
	if len(patch) == 0 {
		return base
	}

	out := cloneMonitoringTasks(base)
	idxByID := make(map[int]int, len(out))
	idxByKey := make(map[string]int, len(out))
	for i, item := range out {
		if item == nil {
			continue
		}
		if item.ID > 0 {
			idxByID[item.ID] = i
		}
		idxByKey[exchange.GetMonitoringTaskKey(*item)] = i
	}

	for _, item := range patch {
		if item == nil {
			continue
		}
		copyItem := *item
		if copyItem.ID > 0 {
			if idx, ok := idxByID[copyItem.ID]; ok {
				out[idx] = &copyItem
				idxByKey[exchange.GetMonitoringTaskKey(copyItem)] = idx
				continue
			}
		}

		if idx, ok := idxByKey[exchange.GetMonitoringTaskKey(copyItem)]; ok {
			out[idx] = &copyItem
			if copyItem.ID > 0 {
				idxByID[copyItem.ID] = idx
			}
			continue
		}

		out = append(out, &copyItem)
		idx := len(out) - 1
		if copyItem.ID > 0 {
			idxByID[copyItem.ID] = idx
		}
		idxByKey[exchange.GetMonitoringTaskKey(copyItem)] = idx
	}

	return out
}

func upsertTradingTasks(base []*exchange.TradingTask, patch []*exchange.TradingTask) []*exchange.TradingTask {
	if len(patch) == 0 {
		return base
	}

	out := cloneTradingTasks(base)
	idxByID := make(map[int]int, len(out))
	idxByKey := make(map[string]int, len(out))
	for i, item := range out {
		if item == nil {
			continue
		}
		if item.ID > 0 {
			idxByID[item.ID] = i
		}
		idxByKey[exchange.GetTradingTaskKey(*item)] = i
	}

	for _, item := range patch {
		if item == nil {
			continue
		}
		copyItem := *item
		if copyItem.ID > 0 {
			if idx, ok := idxByID[copyItem.ID]; ok {
				out[idx] = &copyItem
				idxByKey[exchange.GetTradingTaskKey(copyItem)] = idx
				continue
			}
		}

		if idx, ok := idxByKey[exchange.GetTradingTaskKey(copyItem)]; ok {
			out[idx] = &copyItem
			if copyItem.ID > 0 {
				idxByID[copyItem.ID] = idx
			}
			continue
		}

		out = append(out, &copyItem)
		idx := len(out) - 1
		if copyItem.ID > 0 {
			idxByID[copyItem.ID] = idx
		}
		idxByKey[exchange.GetTradingTaskKey(copyItem)] = idx
	}

	return out
}

func removeMonitoringTasks(base []*exchange.MonitoringTask, ids map[int]struct{}) []*exchange.MonitoringTask {
	if len(ids) == 0 {
		return cloneMonitoringTasks(base)
	}
	out := make([]*exchange.MonitoringTask, 0, len(base))
	for _, item := range base {
		if item == nil {
			continue
		}
		if _, remove := ids[item.ID]; remove {
			continue
		}
		copyItem := *item
		out = append(out, &copyItem)
	}
	return out
}

func removeTradingTasks(base []*exchange.TradingTask, ids map[int]struct{}) []*exchange.TradingTask {
	if len(ids) == 0 {
		return cloneTradingTasks(base)
	}
	out := make([]*exchange.TradingTask, 0, len(base))
	for _, item := range base {
		if item == nil {
			continue
		}
		if _, remove := ids[item.ID]; remove {
			continue
		}
		copyItem := *item
		out = append(out, &copyItem)
	}
	return out
}

func mapMonitoringTasks(in []inboundMonitoringTask) []*exchange.MonitoringTask {
	if len(in) == 0 {
		return []*exchange.MonitoringTask{}
	}
	out := make([]*exchange.MonitoringTask, 0, len(in))
	for _, item := range in {
		out = append(out, &exchange.MonitoringTask{
			ID:               item.ID,
			UID:              item.UID,
			ExchangeID:       item.ExchangeID,
			ExchangeName:     item.ExchangeName,
			MarketType:       item.MarketType,
			TradePairID:      item.TradePairID,
			TradePair:        item.TradePair,
			OrderbookDepth:   item.OrderbookDepth,
			BatchSize:        item.BatchSize,
			BatchIntervalSec: item.BatchIntervalSec,
			RingBufferSize:   item.RingBufferSize,
			SaveIntervalSec:  item.SaveIntervalSec,
		})
	}
	return out
}

func mapTradingTasks(in []inboundTradingTask) []*exchange.TradingTask {
	if len(in) == 0 {
		return []*exchange.TradingTask{}
	}
	out := make([]*exchange.TradingTask, 0, len(in))
	for _, item := range in {
		out = append(out, &exchange.TradingTask{
			ID:                item.ID,
			UID:               item.UID,
			TradeType:         item.TradeType,
			ExchangeID:        item.ExchangeID,
			ExchangeName:      item.ExchangeName,
			MarketType:        item.MarketType,
			TradePairID:       item.TradePairID,
			TradePair:         item.TradePair,
			StrategyID:        item.StrategyID,
			StrategyParams:    item.StrategyParams,
			ExchangeAccountID: item.ExchangeAccountID,
		})
	}
	return out
}
