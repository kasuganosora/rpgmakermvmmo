// TemplateEvent 插件指令：处理 TemplateEvent.js 的服务端指令分发。
package npc

import (
	"context"
	"strconv"
	"strings"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
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

	case "TE_SET_SELF_VARIABLE":
		// 独立变量管理 — 静默吸收（服务端尚未追踪）
		return true

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

	// 可选的页面索引参数（默认为 0）
	pageIdx := 0
	if len(args) > 0 {
		if idx, err := strconv.Atoi(args[0]); err == nil && idx >= 0 {
			pageIdx = idx
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
		e.logger.Warn("TE_CALL_MAP_EVENT: missing template name")
		return true
	}
	tmplName := args[0]
	pageIdx := 0
	if len(args) > 1 {
		if idx, err := strconv.Atoi(args[1]); err == nil && idx >= 0 {
			pageIdx = idx
		}
	}

	// 在所有地图中按名称搜索模板事件
	var tmplEvent *resource.MapEvent
	for _, md := range e.res.Maps {
		if md == nil {
			continue
		}
		for _, ev := range md.Events {
			if ev != nil && ev.Name == tmplName {
				tmplEvent = ev
				break
			}
		}
		if tmplEvent != nil {
			break
		}
	}

	if tmplEvent == nil {
		e.logger.Warn("TE_CALL_MAP_EVENT: template not found",
			zap.String("name", tmplName))
		return true
	}

	// TemplateEvent.js 插件指令中页面索引为 1 起始，数组为 0 起始
	arrayIdx := pageIdx - 1
	if arrayIdx < 0 {
		arrayIdx = 0
	}
	if arrayIdx >= len(tmplEvent.Pages) {
		arrayIdx = 0
	}

	page := tmplEvent.Pages[arrayIdx]
	if page == nil || len(page.List) == 0 {
		return true
	}

	e.logger.Info("TE_CALL_MAP_EVENT: executing template page",
		zap.String("template", tmplName),
		zap.Int("page_idx", arrayIdx),
		zap.Int("cmd_count", len(page.List)))

	e.executeList(ctx, s, page.List, opts, depth+1)
	return true
}

// findMapEvent 按地图 ID 和事件 ID 查找地图事件。
func (e *Executor) findMapEvent(mapID, eventID int) *resource.MapEvent {
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
