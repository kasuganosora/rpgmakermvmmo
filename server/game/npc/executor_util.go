// 工具函数：RMMV 事件指令参数提取与类型转换。
package npc

// paramStr 从参数列表中提取指定索引的字符串值。
// 索引越界或类型不匹配时返回空字符串。
func paramStr(params []interface{}, idx int) string {
	if idx >= len(params) {
		return ""
	}
	if s, ok := params[idx].(string); ok {
		return s
	}
	return ""
}

// paramInt 从参数列表中提取指定索引的整数值。
// 支持 int、float64、int64 三种 JSON 反序列化可能产生的数值类型。
// 索引越界或类型不匹配时返回 0。
func paramInt(params []interface{}, idx int) int {
	if idx >= len(params) {
		return 0
	}
	switch v := params[idx].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	}
	return 0
}

// paramList 从参数列表中提取指定索引的字符串数组。
// RMMV 中选项列表等参数以 []interface{} 形式存储，此方法将其转为 []string。
func paramList(params []interface{}, idx int) []string {
	if idx >= len(params) {
		return nil
	}
	raw, ok := params[idx].([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// asBool 将 interface{} 转换为布尔值。
// 支持 bool、float64、int、string 类型，用于处理 RMMV 中"等待完成"等标志参数。
func asBool(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case int:
		return val != 0
	case string:
		return val == "true" || val == "1"
	}
	return false
}
