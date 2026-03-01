// 效果转发：视觉/音效指令转发、地图传送、图片处理、移动路线、帧等待。
package npc

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// sendEffect 将视觉/音效 RMMV 指令作为 npc_effect 消息转发给客户端。
// 客户端收到后执行对应的 RMMV 渲染函数。
func (e *Executor) sendEffect(s *player.PlayerSession, cmd *resource.EventCommand) {
	payload, _ := json.Marshal(map[string]interface{}{
		"code":   cmd.Code,
		"indent": cmd.Indent,
		"params": cmd.Parameters,
	})
	s.Send(&player.Packet{Type: "npc_effect", Payload: payload})
}

// transferPlayer 处理 RMMV 地图传送指令（代码 201）。
// 参数格式：[0]=模式(0=直接指定,1=变量引用), [1]=地图ID, [2]=X, [3]=Y, [4]=朝向。
// 优先调用 TransferFn 回调执行服务端传送；未配置时退回客户端处理。
func (e *Executor) transferPlayer(s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	mode := paramInt(params, 0)
	mapID := paramInt(params, 1)
	x := paramInt(params, 2)
	y := paramInt(params, 3)
	dir := paramInt(params, 4)

	// 模式 1：从游戏变量解析实际值
	if mode == 1 && opts != nil && opts.GameState != nil {
		mapID = opts.GameState.GetVariable(mapID)
		x = opts.GameState.GetVariable(x)
		y = opts.GameState.GetVariable(y)
	}

	if dir <= 0 {
		dir = 2
	}

	e.logger.Info("executor transferPlayer",
		zap.Int64("char_id", s.CharID),
		zap.Int("mode", mode),
		zap.Int("dest_map", mapID),
		zap.Int("dest_x", x),
		zap.Int("dest_y", y),
		zap.Int("dest_dir", dir),
		zap.Int("from_map", opts.MapID),
		zap.Int("event_id", opts.EventID))

	if opts != nil && opts.TransferFn != nil {
		opts.TransferFn(s, mapID, x, y, dir)
		return
	}

	// 退回方案：发送 transfer_player 给客户端（无服务端处理器）
	e.logger.Warn("no TransferFn set, sending client-side transfer",
		zap.Int("map_id", mapID), zap.Int("x", x), zap.Int("y", y))
	payload, _ := json.Marshal(map[string]interface{}{
		"map_id": mapID,
		"x":      x,
		"y":      y,
		"dir":    dir,
	})
	s.Send(&player.Packet{Type: "transfer_player", Payload: payload})
}

// waitFrames 等待 N 帧（按 60fps 换算为毫秒），或在上下文取消时提前返回。
func (e *Executor) waitFrames(ctx context.Context, frames int) {
	if frames <= 0 {
		return
	}
	wait := time.Duration(frames) * time.Second / 60
	select {
	case <-time.After(wait):
	case <-ctx.Done():
	}
}

// estimateMoveRouteFrames 估算移动路线所需的帧数。
// 移动/跳跃指令约 16 帧（普通速度），等待指令按实际帧数计算，
// 转向/速度等指令约 2 帧。最少返回 10 帧。
func estimateMoveRouteFrames(mr map[string]interface{}) int {
	listRaw, ok := mr["list"]
	if !ok {
		return 30
	}
	list, ok := listRaw.([]interface{})
	if !ok || len(list) == 0 {
		return 30
	}
	frames := 0
	for _, item := range list {
		cmd, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		codeRaw, _ := cmd["code"]
		code := 0
		if f, ok := codeRaw.(float64); ok {
			code = int(f)
		}
		switch {
		case code >= 1 && code <= 14:
			// 移动/跳跃指令：普通速度约 16 帧
			frames += 16
		case code == 15:
			// 等待指令：params[0] = 等待帧数
			if params, ok := cmd["parameters"].([]interface{}); ok && len(params) > 0 {
				if f, ok := params[0].(float64); ok {
					frames += int(f)
				}
			}
		case code >= 16 && code <= 44:
			// 转向/速度等指令：几乎瞬间完成
			frames += 2
		}
	}
	if frames < 10 {
		frames = 10
	}
	return frames
}

// sendShowPicture 解析变量坐标并转发显示图片指令。
// RMMV 参数：[图片ID, 名称, 原点, 指定方式, X, Y, 缩放X, 缩放Y, 不透明度, 混合模式]。
// 当 params[3]==1 时，X/Y 取自游戏变量而非直接值，解析后标记为直接指定以避免客户端重复解析。
func (e *Executor) sendShowPicture(s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	resolved := make([]interface{}, len(params))
	copy(resolved, params)

	designation := paramInt(params, 3)
	if designation == 1 && opts != nil && opts.GameState != nil {
		varX := paramInt(params, 4)
		varY := paramInt(params, 5)
		if len(resolved) > 4 {
			resolved[4] = float64(opts.GameState.GetVariable(varX))
		}
		if len(resolved) > 5 {
			resolved[5] = float64(opts.GameState.GetVariable(varY))
		}
		// 标记为直接指定，客户端不再二次解析
		if len(resolved) > 3 {
			resolved[3] = float64(0)
		}
	}

	e.sendEffect(s, &resource.EventCommand{Code: CmdShowPicture, Parameters: resolved})
}

// sendMoveRoute 解析 charId=0（"当前事件"）为实际事件 ID 后转发移动路线。
// RMMV 中 charId=0 表示触发事件自身，需替换为具体 eventID 以供客户端定位精灵。
func (e *Executor) sendMoveRoute(s *player.PlayerSession, cmd *resource.EventCommand, opts *ExecuteOpts) {
	charID := paramInt(cmd.Parameters, 0)
	if charID == 0 && opts != nil && opts.EventID > 0 {
		resolved := make([]interface{}, len(cmd.Parameters))
		copy(resolved, cmd.Parameters)
		resolved[0] = float64(opts.EventID)
		e.sendEffect(s, &resource.EventCommand{Code: CmdSetMoveRoute, Parameters: resolved})
		return
	}
	e.sendEffect(s, cmd)
}

// sendMovePicture 解析变量坐标并转发移动图片指令，支持等待完成。
// RMMV 参数：[图片ID, 原点, 指定方式, X, Y, 缩放X, 缩放Y, 不透明度, 混合模式, 持续帧数, 是否等待]。
func (e *Executor) sendMovePicture(ctx context.Context, s *player.PlayerSession, params []interface{}, opts *ExecuteOpts) {
	resolved := make([]interface{}, len(params))
	copy(resolved, params)

	designation := paramInt(params, 2)
	if designation == 1 && opts != nil && opts.GameState != nil {
		varX := paramInt(params, 3)
		varY := paramInt(params, 4)
		if len(resolved) > 3 {
			resolved[3] = float64(opts.GameState.GetVariable(varX))
		}
		if len(resolved) > 4 {
			resolved[4] = float64(opts.GameState.GetVariable(varY))
		}
		if len(resolved) > 2 {
			resolved[2] = float64(0)
		}
	}

	e.sendEffect(s, &resource.EventCommand{Code: CmdMovePicture, Parameters: resolved})

	// params[10]=true 时等待动画完成
	if len(params) > 10 && asBool(params[10]) {
		frames := paramInt(params, 9)
		if frames > 0 {
			e.waitFrames(ctx, frames)
		}
	}
}
