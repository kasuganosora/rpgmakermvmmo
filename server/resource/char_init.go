// char_init.go — 从 CommonEvents.json 中提取角色创建时的初始化变量和装备。
// 替代 character.go 中的硬编码值，直接从原版游戏数据读取。
package resource

import (
	"fmt"
	"strconv"
	"strings"
)

// CharInitVar 表示角色创建时需要初始化的一个变量。
type CharInitVar struct {
	VariableID int
	Value      int
}

// CharInitEquip 表示角色创建时需要初始化的一件装备。
type CharInitEquip struct {
	ArmorID   int
	SlotIndex int
}

// CharInitData 包含从游戏 CE 中提取的角色初始化数据。
type CharInitData struct {
	Variables []CharInitVar
	Equips    []CharInitEquip
}

// equipSlotTypeMap 将 EquipChange 插件命令的槽位类型名映射到槽位索引。
// 与 executor_stats.go 中的映射保持一致。
var initEquipSlotMap = map[string]int{
	"Weapon": 0, "武装": 0, "0": 0,
	"Cloth": 1, "衣装": 1, "1": 1,
	"ClothOption": 2, "衣装オプション": 2, "2": 2,
	"Option": 3, "追加外装": 3, "3": 3,
	"Other": 4, "その他": 4, "4": 4,
	"Special": 6, "特殊": 6, "6": 6,
	"Leg": 7, "脚": 7, "7": 7,
	"Special1": 8, "13": 8,
	"Special2": 9, "14": 9,
	"Special3": 10, "15": 10,
	"Special4": 11, "16": 11,
	"Special5": 12, "17": 12,
	"Special6": 13, "18": 13,
}

// ExtractCharInit 从指定 CE（及其递归调用的子 CE）中提取变量赋值和装备初始化。
// 处理所有指令（含条件分支内的），因为 CE 1 是初始化事件，所有路径最终都会执行。
// 对同一变量的多次赋值保留最后一次值（模拟顺序执行）。
// ceID 通常为 1（游戏的"初期値設定"公共事件）。
func (rl *ResourceLoader) ExtractCharInit(ceID int) *CharInitData {
	if rl == nil || ceID <= 0 || ceID >= len(rl.CommonEvents) {
		return nil
	}
	raw := &CharInitData{}
	// vars 用于跟踪已赋值的变量，以支持变量引用（operandType=1）。
	vars := make(map[int]int)
	visited := make(map[int]bool) // 防止循环调用
	rl.extractFromCE(ceID, raw, vars, visited)

	// 去重变量：同一变量多次赋值只保留最后一次。
	// 跳过 value=0 的赋值（DB 默认值已是 0，无需存储）。
	data := &CharInitData{Equips: raw.Equips}
	seen := make(map[int]bool)
	for i := len(raw.Variables) - 1; i >= 0; i-- {
		v := raw.Variables[i]
		if !seen[v.VariableID] {
			seen[v.VariableID] = true
			if v.Value != 0 {
				data.Variables = append(data.Variables, v)
			}
		}
	}
	// 反转恢复原始顺序（可选，不影响功能）
	for i, j := 0, len(data.Variables)-1; i < j; i, j = i+1, j-1 {
		data.Variables[i], data.Variables[j] = data.Variables[j], data.Variables[i]
	}
	return data
}

func (rl *ResourceLoader) extractFromCE(ceID int, data *CharInitData, vars map[int]int, visited map[int]bool) {
	if ceID <= 0 || ceID >= len(rl.CommonEvents) || visited[ceID] {
		return
	}
	visited[ceID] = true
	ce := rl.CommonEvents[ceID]
	if ce == nil || len(ce.List) == 0 {
		return
	}

	for _, cmd := range ce.List {
		if cmd == nil {
			continue
		}
		switch cmd.Code {
		case 122: // Control Variables
			extractVarAssignment(cmd.Parameters, data, vars)
		case 117: // Call Common Event
			subCEID := paramIntRaw(cmd.Parameters, 0)
			rl.extractFromCE(subCEID, data, vars, visited)
		case 356: // Plugin Command
			extractEquipChange(cmd.Parameters, data)
		}
	}
}

// extractVarAssignment 解析 code 122 (Control Variables) 指令。
// 参数: [0]=startID, [1]=endID, [2]=op, [3]=operandType, [4]=operand, [5]=max(random)
func extractVarAssignment(params []interface{}, data *CharInitData, vars map[int]int) {
	startID := paramIntRaw(params, 0)
	endID := paramIntRaw(params, 1)
	op := paramIntRaw(params, 2)
	operandType := paramIntRaw(params, 3)
	operandVal := paramIntRaw(params, 4)

	// 仅处理"设置"操作（op=0）的常量(0)和变量引用(1)。
	// 跳过随机(2)、游戏数据(3)、脚本(4)等不确定值。
	if op != 0 || (operandType != 0 && operandType != 1) {
		return
	}

	val := operandVal
	if operandType == 1 {
		// 变量引用：从已解析的变量中取值
		val = vars[operandVal]
	}

	for id := startID; id <= endID; id++ {
		vars[id] = val
		data.Variables = append(data.Variables, CharInitVar{VariableID: id, Value: val})
	}
}

// extractEquipChange 解析 "EquipChange <SlotType> <ArmorID>" 插件命令。
func extractEquipChange(params []interface{}, data *CharInitData) {
	if len(params) == 0 {
		return
	}
	s, ok := params[0].(string)
	if !ok {
		return
	}
	if !strings.HasPrefix(s, "EquipChange ") {
		return
	}
	parts := strings.Fields(s)
	if len(parts) < 3 {
		return
	}
	slotIndex, ok := initEquipSlotMap[parts[1]]
	if !ok {
		return
	}
	armorID, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}
	data.Equips = append(data.Equips, CharInitEquip{ArmorID: armorID, SlotIndex: slotIndex})
}

// paramIntRaw extracts an int from a raw JSON parameter slice.
func paramIntRaw(params []interface{}, idx int) int {
	if idx >= len(params) {
		return 0
	}
	switch v := params[idx].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

// String returns a human-readable summary for logging.
func (d *CharInitData) String() string {
	return fmt.Sprintf("CharInitData: %d variables, %d equips", len(d.Variables), len(d.Equips))
}
