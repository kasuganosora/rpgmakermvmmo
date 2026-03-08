// TemplateEvent 插件指令：处理 TemplateEvent.js 的服务端指令分发。
package npc

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

var (
	reSvRef = regexp.MustCompile(`(?i)\\sv\[(\d+)\]`)
	reVRef  = regexp.MustCompile(`(?i)\\v\[(\d+)\]`)
)

// handleTECallOriginEvent 检查插件指令（代码 356）是否为 TemplateEvent.js 需服务端处理的指令。
// 返回 true 表示指令已处理（或被刻意跳过），false 表示应转发给客户端。
//
// 支持的 TE 指令：
//   - TE固有イベント呼び出し / TE_CALL_ORIGIN_EVENT — 执行原始（模板替换前）事件页
//   - TEテンプレート呼び出し / TE_CALL_MAP_EVENT — 按名称调用模板事件的指定页
//   - TE_SET_SELF_VARIABLE — 设置独立变量（静默吸收，服务端暂不追踪）
//   - TE関連データ値デバッグ表示 — 调试显示（服务端跳过）
func (e *Executor) handleTECallOriginEvent(ctx context.Context, s *player.PlayerSession, cmd *resource.EventCommand, opts *ExecuteOpts, depth int) bool {
	if len(cmd.Parameters) == 0 {
		return false
	}
	raw, _ := cmd.Parameters[0].(string)
	if raw == "" {
		return false
	}

	// 解析 "CommandName arg1 arg2 ..."（RMMV 代码 356 格式）
	parts := strings.Fields(raw)
	cmdName := parts[0]
	cmdArgs := parts[1:]

	switch cmdName {
	case "TE固有イベント呼び出し", "TE_CALL_ORIGIN_EVENT":
		return e.teCallOriginEvent(ctx, s, cmdArgs, opts, depth)

	case "TEテンプレート呼び出し", "TE_CALL_MAP_EVENT":
		return e.teCallMapEvent(ctx, s, cmdArgs, opts, depth)

	case "TE_SET_SELF_VARIABLE", "TEセルフ変数の操作":
		return e.teSetSelfVariable(s, cmdArgs, opts)

	case "TE_SET_RANGE_SELF_VARIABLE", "TEセルフ変数の一括操作":
		return e.teSetRangeSelfVariable(s, cmdArgs, opts)

	case "TE関連データ値デバッグ表示":
		// 调试显示 — 服务端跳过
		return true
	}

	// 非 TE 指令，交由调用方转发给客户端
	return false
}

// teCallOriginEvent 处理 TE_CALL_ORIGIN_EVENT：执行事件的原始（模板替换前）页面指令。
// 当事件被 TemplateEvent.js 替换为模板时，原始页面保存在 OriginalPages 中。
func (e *Executor) teCallOriginEvent(ctx context.Context, s *player.PlayerSession, args []string, opts *ExecuteOpts, depth int) bool {
	if opts == nil || opts.MapID <= 0 || opts.EventID <= 0 {
		e.logger.Warn("TE_CALL_ORIGIN_EVENT: missing map/event context")
		return true
	}

	mapEvent := e.findMapEvent(opts.MapID, opts.EventID)
	if mapEvent == nil {
		e.logger.Warn("TE_CALL_ORIGIN_EVENT: event not found",
			zap.Int("map_id", opts.MapID), zap.Int("event_id", opts.EventID))
		return true
	}

	if len(mapEvent.OriginalPages) == 0 {
		e.logger.Warn("TE_CALL_ORIGIN_EVENT: no original pages",
			zap.Int("map_id", opts.MapID), zap.Int("event_id", opts.EventID))
		return true
	}

	// TemplateEvent.js 使用 1 起始页面索引：pages[pageIndex - 1 || _pageIndex]
	// arg=0 或缺省 → page 0；arg=1 → page 0（JS 中 0 为 falsy）；arg≥2 → page arg-1
	pageIdx := 0
	if len(args) > 0 {
		if idx, err := strconv.Atoi(args[0]); err == nil && idx >= 2 {
			pageIdx = idx - 1
		}
	}
	if pageIdx >= len(mapEvent.OriginalPages) {
		pageIdx = 0
	}

	origPage := mapEvent.OriginalPages[pageIdx]
	if origPage == nil || len(origPage.List) == 0 {
		return true
	}

	e.logger.Info("TE_CALL_ORIGIN_EVENT: executing original page",
		zap.Int("map_id", opts.MapID),
		zap.Int("event_id", opts.EventID),
		zap.Int("page_idx", pageIdx),
		zap.Int("cmd_count", len(origPage.List)))

	e.executeList(ctx, s, origPage.List, opts, depth+1)
	return true
}

// teCallMapEvent 处理 TE_CALL_MAP_EVENT：按名称从模板地图查找事件，执行指定页面的指令。
// 格式："TE_CALL_MAP_EVENT 模板名称 页面索引"（页面索引为 1 起始，与 TemplateEvent.js 一致）。
func (e *Executor) teCallMapEvent(ctx context.Context, s *player.PlayerSession, args []string, opts *ExecuteOpts, depth int) bool {
	if len(args) < 1 {
		e.logger.Warn("TE_CALL_MAP_EVENT: missing template name/id")
		return true
	}
	tmplNameOrID := args[0]
	pageIdx := 0
	if len(args) > 1 {
		if idx, err := strconv.Atoi(args[1]); err == nil && idx >= 0 {
			pageIdx = idx
		}
	}

	// TemplateEvent.js: 先按数值 eventId 查找当前地图，再按名称查找当前地图
	var tmplEvent *resource.MapEvent
	if opts != nil && opts.MapID > 0 && e.res != nil {
		md := e.res.Maps[opts.MapID]
		if md != nil {
			// 尝试按数值 ID 查找
			if numID, err := strconv.Atoi(tmplNameOrID); err == nil && numID > 0 {
				for _, ev := range md.Events {
					if ev != nil && ev.ID == numID {
						tmplEvent = ev
						break
					}
				}
			}
			// 按名称查找
			if tmplEvent == nil {
				for _, ev := range md.Events {
					if ev != nil && ev.Name == tmplNameOrID {
						tmplEvent = ev
						break
					}
				}
			}
		}
	}

	if tmplEvent == nil {
		e.logger.Warn("TE_CALL_MAP_EVENT: template not found",
			zap.String("name", tmplNameOrID))
		return true
	}

	// TemplateEvent.js 使用 1 起始页面索引：pages[pageIndex - 1 || _pageIndex]
	// arg=0 或缺省 → page 0；arg=1 → page 0（JS 中 0 为 falsy）；arg≥2 → page arg-1
	arrayIdx := 0
	if pageIdx >= 2 {
		arrayIdx = pageIdx - 1
	}
	if arrayIdx >= len(tmplEvent.Pages) {
		arrayIdx = 0
	}

	page := tmplEvent.Pages[arrayIdx]
	if page == nil || len(page.List) == 0 {
		return true
	}

	e.logger.Info("TE_CALL_MAP_EVENT: executing template page",
		zap.String("template", tmplNameOrID),
		zap.Int("page_idx", arrayIdx),
		zap.Int("cmd_count", len(page.List)))

	e.executeList(ctx, s, page.List, opts, depth+1)
	return true
}

// findMapEvent 按地图 ID 和事件 ID 查找地图事件。
func (e *Executor) findMapEvent(mapID, eventID int) *resource.MapEvent {
	if e.res == nil {
		return nil
	}
	md, ok := e.res.Maps[mapID]
	if !ok {
		return nil
	}
	for _, ev := range md.Events {
		if ev != nil && ev.ID == eventID {
			return ev
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// TE_SET_SELF_VARIABLE / TE_SET_RANGE_SELF_VARIABLE
// ---------------------------------------------------------------------------

// teSetSelfVariable 处理 TE_SET_SELF_VARIABLE：设置/修改单个独立变量。
// 格式："TE_SET_SELF_VARIABLE [Index] [OperationType] [Operand]"
// OperationType: 0=赋值, 1=加, 2=减, 3=乘, 4=除, 5=取模
func (e *Executor) teSetSelfVariable(s *player.PlayerSession, args []string, opts *ExecuteOpts) bool {
	if opts == nil || opts.GameState == nil || opts.MapID <= 0 || opts.EventID <= 0 {
		return true
	}
	if len(args) < 3 {
		e.logger.Warn("TE_SET_SELF_VARIABLE: need 3 args [index, opType, operand]",
			zap.Strings("args", args))
		return true
	}

	index, err := strconv.Atoi(args[0])
	if err != nil {
		return true
	}
	opType, err := strconv.Atoi(args[1])
	if err != nil {
		return true
	}
	// TemplateEvent.js convertAllSelfVariables: 解析 \sv[N] 和 \v[N] 引用
	operand, err := e.resolveTEOperand(args[2], opts)
	if err != nil {
		return true
	}

	current := opts.GameState.GetSelfVariable(opts.MapID, opts.EventID, index)
	result := operateSelfVariable(current, opType, operand)
	opts.GameState.SetSelfVariable(opts.MapID, opts.EventID, index, result)

	// 推送自变量变更给客户端，使客户端 TemplateEvent.js 状态同步。
	e.sendSelfVarChange(s, opts.MapID, opts.EventID, index, result)

	e.logger.Debug("TE_SET_SELF_VARIABLE",
		zap.Int("map_id", opts.MapID),
		zap.Int("event_id", opts.EventID),
		zap.Int("index", index),
		zap.Int("op", opType),
		zap.Int("operand", operand),
		zap.Int("old", current),
		zap.Int("new", result))
	return true
}

// teSetRangeSelfVariable 处理 TE_SET_RANGE_SELF_VARIABLE：批量设置独立变量。
// 格式："TE_SET_RANGE_SELF_VARIABLE [StartIndex] [EndIndex] [OperationType] [Operand]"
func (e *Executor) teSetRangeSelfVariable(s *player.PlayerSession, args []string, opts *ExecuteOpts) bool {
	if opts == nil || opts.GameState == nil || opts.MapID <= 0 || opts.EventID <= 0 {
		return true
	}
	if len(args) < 4 {
		e.logger.Warn("TE_SET_RANGE_SELF_VARIABLE: need 4 args [start, end, opType, operand]",
			zap.Strings("args", args))
		return true
	}

	startIdx, err := strconv.Atoi(args[0])
	if err != nil {
		return true
	}
	endIdx, err := strconv.Atoi(args[1])
	if err != nil {
		return true
	}
	opType, err := strconv.Atoi(args[2])
	if err != nil {
		return true
	}
	operand, err := e.resolveTEOperand(args[3], opts)
	if err != nil {
		return true
	}

	for i := startIdx; i <= endIdx; i++ {
		current := opts.GameState.GetSelfVariable(opts.MapID, opts.EventID, i)
		result := operateSelfVariable(current, opType, operand)
		opts.GameState.SetSelfVariable(opts.MapID, opts.EventID, i, result)
		e.sendSelfVarChange(s, opts.MapID, opts.EventID, i, result)
	}

	e.logger.Debug("TE_SET_RANGE_SELF_VARIABLE",
		zap.Int("map_id", opts.MapID),
		zap.Int("event_id", opts.EventID),
		zap.Int("start", startIdx),
		zap.Int("end", endIdx),
		zap.Int("op", opType),
		zap.Int("operand", operand))
	return true
}

// sendSelfVarChange 推送自变量变更给客户端。
// 客户端收到后更新本地 $gameSelfSwitches._variableData，使 TemplateEvent.js 状态同步。
func (e *Executor) sendSelfVarChange(s *player.PlayerSession, mapID, eventID, index, value int) {
	payload, _ := json.Marshal(map[string]interface{}{
		"map_id":   mapID,
		"event_id": eventID,
		"index":    index,
		"value":    value,
	})
	s.Send(&player.Packet{Type: "self_var_change", Payload: payload})
}

// resolveTEOperand 解析 TE 操作数，支持 \sv[N]（独立变量引用）和 \v[N]（游戏变量引用）。
// 对应 TemplateEvent.js 的 convertAllSelfVariables 行为。
func (e *Executor) resolveTEOperand(raw string, opts *ExecuteOpts) (int, error) {
	if opts != nil && opts.GameState != nil {
		// \sv[N] → 独立变量值
		if m := reSvRef.FindStringSubmatch(raw); len(m) == 2 {
			idx, _ := strconv.Atoi(m[1])
			return opts.GameState.GetSelfVariable(opts.MapID, opts.EventID, idx), nil
		}
		// \v[N] → 游戏变量值
		if m := reVRef.FindStringSubmatch(raw); len(m) == 2 {
			idx, _ := strconv.Atoi(m[1])
			return opts.GameState.GetVariable(idx), nil
		}
	}
	return strconv.Atoi(raw)
}

// operateSelfVariable 对独立变量执行算术操作。
// opType: 0=赋值, 1=加, 2=减, 3=乘, 4=除, 5=取模
func operateSelfVariable(current, opType, operand int) int {
	switch opType {
	case 0: // 赋值
		return operand
	case 1: // 加
		return current + operand
	case 2: // 减
		return current - operand
	case 3: // 乘
		return current * operand
	case 4: // 除
		if operand != 0 {
			return current / operand
		}
		return current
	case 5: // 取模
		if operand != 0 {
			return current % operand
		}
		return current
	default:
		return current
	}
}

// ---------------------------------------------------------------------------
// MPP_CallCommonByName: CallCommon / CCT
// ---------------------------------------------------------------------------

// handleCallCommon 检查插件指令是否为 CallCommon 或 CCT（按名称调用公共事件）。
// 返回 true 表示已处理（服务端执行 CE），false 表示非 CallCommon 指令。
func (e *Executor) handleCallCommon(ctx context.Context, s *player.PlayerSession, cmd *resource.EventCommand, opts *ExecuteOpts, depth int) bool {
	if len(cmd.Parameters) == 0 {
		return false
	}
	raw, _ := cmd.Parameters[0].(string)
	if raw == "" {
		return false
	}

	parts := strings.Fields(raw)
	cmdName := parts[0]

	switch cmdName {
	case "CallCommon":
		if len(parts) < 2 {
			return true
		}
		ceName := strings.Join(parts[1:], " ")
		ceID := e.res.FindCommonEventByName(ceName)
		if ceID <= 0 {
			e.logger.Warn("CallCommon: CE not found by name",
				zap.String("name", ceName))
			return true
		}
		e.callCommonEvent(ctx, s, ceID, opts, depth)
		return true

	case "CCT":
		if len(parts) < 2 {
			return true
		}
		prefix := strings.Join(parts[1:], " ")
		ceID := e.res.FindCommonEventByPrefix(prefix)
		if ceID <= 0 {
			e.logger.Warn("CCT: CE not found by prefix",
				zap.String("prefix", prefix))
			return true
		}
		e.callCommonEvent(ctx, s, ceID, opts, depth)
		return true
	}

	return false
}
