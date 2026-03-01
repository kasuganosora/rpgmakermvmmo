// 对话与选项：RMMV 对话显示、选项等待、文本转义码解析。
package npc

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
)

// sendDialog 向客户端发送 NPC 对话消息。
// 参数对应 RMMV ShowText 指令：faceName=头像文件名, faceIndex=头像索引,
// background=窗口背景(0=窗口,1=暗化,2=透明), positionType=位置(0=上,1=中,2=下)。
func (e *Executor) sendDialog(s *player.PlayerSession, faceName string, faceIndex, background, positionType int, lines []string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"face":          faceName,
		"face_index":    faceIndex,
		"background":    background,
		"position_type": positionType,
		"lines":         lines,
	})
	s.Send(&player.Packet{Type: "npc_dialog", Payload: payload})
}

// sendChoices 向客户端发送选项列表。
// defaultType=默认高亮选项索引, cancelType=取消行为(-1=禁止取消, 0-N=取消时跳转的分支)。
func (e *Executor) sendChoices(s *player.PlayerSession, choices []string, defaultType, cancelType, positionType, background int) {
	payload, _ := json.Marshal(map[string]interface{}{
		"choices":       choices,
		"default_type":  defaultType,
		"cancel_type":   cancelType,
		"position_type": positionType,
		"background":    background,
	})
	s.Send(&player.Packet{Type: "npc_choices", Payload: payload})
}

// sendDialogWithChoices 向客户端发送合并的对话+选项消息。
// RMMV 中当文本指令紧接选项指令时，两者合并显示在同一窗口。
func (e *Executor) sendDialogWithChoices(s *player.PlayerSession, faceName string, faceIndex, background, positionType int, lines []string, choices []string, choiceDefault, choiceCancel, choicePosition, choiceBg int) {
	payload, _ := json.Marshal(map[string]interface{}{
		"face":              faceName,
		"face_index":        faceIndex,
		"background":        background,
		"position_type":     positionType,
		"lines":             lines,
		"choices":           choices,
		"choice_default":    choiceDefault,
		"choice_cancel":     choiceCancel,
		"choice_position":   choicePosition,
		"choice_background": choiceBg,
	})
	s.Send(&player.Packet{Type: "npc_dialog_choices", Payload: payload})
}

// sendDialogEnd 向客户端发送对话结束信号。
// 执行完毕后调用，通知客户端关闭对话窗口。
func (e *Executor) sendDialogEnd(s *player.PlayerSession) {
	s.Send(&player.Packet{Type: "npc_dialog_end"})
}

// waitForDialogAck 阻塞等待客户端确认对话已显示。
// 仅在断开连接 (s.Done) 或上下文取消时中止，无固定超时。
// 返回 true 表示收到确认，false 表示连接中断或上下文已取消。
func (e *Executor) waitForDialogAck(ctx context.Context, s *player.PlayerSession) bool {
	select {
	case <-s.DialogAckCh:
		return true
	case <-s.Done:
		return false
	case <-ctx.Done():
		return false
	}
}

// waitForChoice 阻塞等待玩家选择选项。
// 仅在断开连接或上下文取消时中止，无固定超时。
// 返回选项索引（从 0 开始），中止时返回 -1。
func (e *Executor) waitForChoice(ctx context.Context, s *player.PlayerSession) int {
	select {
	case idx := <-s.ChoiceCh:
		return idx
	case <-s.Done:
		return -1
	case <-ctx.Done():
		return -1
	}
}

// ---- 文本转义码解析 ----

// textCodeRe 匹配 RMMV 文本转义码：\N[n], \V[n], \P[n]（不区分大小写）。
var textCodeRe = regexp.MustCompile(`(?i)\\([NVP])\[(\d+)\]`)

// resolveTextCodes 替换 RMMV 对话文本中的转义码：
//   - \N[n] → 角色名（Actor 1/101 = MMO 中的玩家名）
//   - \V[n] → 游戏变量值
//   - \P[n] → 队伍成员名（MMO 中 P[1] = 玩家）
func (e *Executor) resolveTextCodes(text string, s *player.PlayerSession, opts *ExecuteOpts) string {
	return textCodeRe.ReplaceAllStringFunc(text, func(match string) string {
		sub := textCodeRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		code := strings.ToUpper(sub[1])
		id, err := strconv.Atoi(sub[2])
		if err != nil {
			return match
		}

		switch code {
		case "N":
			// \N[n] = 角色名。MMO 中 Actor 1 映射为玩家角色。
			if id == 1 || id == 101 {
				return s.CharName
			}
			// 尝试从资源数据中查找角色名
			if e.res != nil && id > 0 && id < len(e.res.Actors) && e.res.Actors[id] != nil {
				return e.res.Actors[id].Name
			}
			return match

		case "V":
			// \V[n] = 游戏变量值
			if opts != nil && opts.GameState != nil {
				return strconv.Itoa(opts.GameState.GetVariable(id))
			}
			return "0"

		case "P":
			// \P[n] = 队伍成员名。MMO 中队伍成员 1 = 玩家。
			if id == 1 {
				return s.CharName
			}
			return match
		}
		return match
	})
}

// resolveDialogLines 对所有对话行应用文本转义码解析。
func (e *Executor) resolveDialogLines(lines []string, s *player.PlayerSession, opts *ExecuteOpts) []string {
	resolved := make([]string, len(lines))
	for i, line := range lines {
		resolved[i] = e.resolveTextCodes(line, s, opts)
	}
	return resolved
}

// resolveChoices 对所有选项标签应用文本转义码解析。
func (e *Executor) resolveChoices(choices []string, s *player.PlayerSession, opts *ExecuteOpts) []string {
	resolved := make([]string, len(choices))
	for i, choice := range choices {
		resolved[i] = e.resolveTextCodes(choice, s, opts)
	}
	return resolved
}
