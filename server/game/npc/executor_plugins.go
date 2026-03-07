// 插件指令服务端执行：CulSkillEffect、ParaCheck 等纯计算型插件。
// 这些插件通过 Goja VM 执行，读写 $gameVariables/_data 和 $dataArmors.meta 等数据。
package npc

import (
	"time"

	"github.com/dop251/goja"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"go.uber.org/zap"
)

// culSkillEffectJS 是 CulSkillEffect.js 插件的核心逻辑（服务端版本）。
// 读取装备和技能的 meta.Skill01-10 数组，计算装备/技能效果累加值。
// 注意：AddSkillEffectBase 依赖 $tag_skill_list（全局变量），服务端跳过。
const culSkillEffectJS = `
(function() {
	// SkillResetAll: 重置所有效果累加变量
	for (var i = 4221; i <= 4322; i++) {
		$gameVariables._data[i] = 0;
	}

	function SkillReset() {
		for (var i = 4211; i <= 4220; i++) {
			$gameVariables._data[i] = 0;
		}
	}

	function AddSkillEffect() {
		for (var i = 4211; i <= 4220; i++) {
			var v = $gameVariables.value(i);
			if (v != 0 && v !== undefined && v !== null) {
				if (typeof v === 'object' && v.length >= 2) {
					$gameVariables._data[v[0] + 4200] += v[1];
				}
			}
		}
	}

	// 装備スキルチェック
	var equips = $gameActors._data[1]._equips;
	var Other1 = 4;
	var Other2 = 5;
	var Other1EqID = 0;
	var EquipNo = 1;

	for (var i = 0; i < equips.length; i++) {
		var EquipID = 0;
		if (equips[EquipNo] != null) {
			EquipID = equips[EquipNo]._itemId;
		}
		if (EquipID >= 1 && $dataArmors[EquipID]) {
			if (EquipNo == Other1) { Other1EqID = EquipID; }
			if (EquipNo == Other2 && EquipID == Other1EqID) {
				// 同一装備の場合スルー
			} else {
				$gameVariables._data[4211] = $dataArmors[EquipID].meta.Skill01;
				$gameVariables._data[4212] = $dataArmors[EquipID].meta.Skill02;
				$gameVariables._data[4213] = $dataArmors[EquipID].meta.Skill03;
				$gameVariables._data[4214] = $dataArmors[EquipID].meta.Skill04;
				$gameVariables._data[4215] = $dataArmors[EquipID].meta.Skill05;
				$gameVariables._data[4216] = $dataArmors[EquipID].meta.Skill06;
				$gameVariables._data[4217] = $dataArmors[EquipID].meta.Skill07;
				$gameVariables._data[4218] = $dataArmors[EquipID].meta.Skill08;
				$gameVariables._data[4219] = $dataArmors[EquipID].meta.Skill09;
				$gameVariables._data[4220] = $dataArmors[EquipID].meta.Skill10;
				AddSkillEffect();
			}
		}
		EquipNo += 1;
	}

	SkillReset();

	// Pスキル効果 — 需要 addedSkills()，服务端暂时跳过技能部分
	// （技能效果累加量相比装备效果较小，后续可补充）

	SkillReset();

	// AddSkillEffectBase — 依赖 $tag_skill_list 全局变量，服务端跳过
	// 直接应用基础值逻辑中最关键的部分
	var naked = ($gameActors._data[1]._equips[1] && $gameActors._data[1]._equips[1]._itemId == 0) ? 40 : 0;
	$gameVariables._data[1200] += naked;
})();
`

// paraCheckCoreJS 是 ParaCheck 插件的核心变量计算部分（服务端版本）。
// 跳过客户端专用的部分（$gameMap, $gamePlayer, addState 等）。
const paraCheckCoreJS = `
(function() {
	$gameVariables._data[1006] = __playerLevel;
	$gameVariables._data[215] = __gold;

	// 衣装耐久スキル
	$gameVariables._data[722] = $gameVariables.value(1209);

	// 魂の侵蝕
	$gameVariables._data[1178] = Math.floor($gameVariables.value(1280) / 15);
	if ($gameVariables.value(1178) > 6) { $gameVariables._data[1178] = 6; }

	// 魂侵蝕 switches
	for (var i = 0; i < 5; i++) {
		$gameSwitches._data[1031 + i] = ($gameVariables.value(1178) >= (i + 1));
	}

	// スキル入力
	$gameVariables._data[1111] = $gameVariables.value(1272);
	$gameVariables._data[1112] = $gameVariables.value(1273);
	$gameVariables._data[1113] = $gameVariables.value(1274);

	// スキルによるパラメータ上限変動
	$gameVariables._data[722] = $gameVariables.value(1209);
	$gameVariables._data[1034] = $gameVariables.value(1260);
	$gameVariables._data[1037] = $gameVariables.value(1259);

	// 装備情報の取得
	var ClothEqNum = 1;
	if ($gameActors._data[1]._equips[ClothEqNum] &&
		$gameActors._data[1]._equips[ClothEqNum]._itemId >= 5) {
		var StandEqNum = $gameActors._data[1]._equips[ClothEqNum]._itemId;
		if ($dataArmors[StandEqNum]) {
			$gameVariables._data[762] = $dataArmors[StandEqNum].meta.ClothName || 0;
		}
	} else {
		$gameVariables._data[762] = "Naked";
	}

	// 変身中チェック
	if ($gameSwitches.value(131)) {
		$gameVariables._data[235] = 1;
	} else {
		$gameVariables._data[235] = 0;
	}

	// ステルス
	$gameSwitches._data[162] = ($gameVariables.value(1215) > 0);
	// 眼罩
	$gameSwitches._data[2917] = ($gameVariables.value(1210) > 0);

	// パラメータ上限チェック (clamp functions)
	function clamp(val, low, high) {
		if (typeof val !== 'number' || isNaN(val)) val = 0;
		return Math.max(low, Math.min(high, val));
	}

	// 発情
	if (__classId > 2) {
		$gameVariables._data[1027] = 0;
	} else {
		$gameVariables._data[1027] = clamp($gameVariables.value(1027), $gameVariables.value(1282), 200);
	}

	// 淫欲
	$gameVariables._data[1021] = clamp($gameVariables.value(1021), 0, $gameVariables.value(1034));

	// 戦意
	$gameVariables._data[1029] = clamp($gameVariables.value(1029), 0, 100);

	// 瘴気
	$gameVariables._data[1030] = clamp($gameVariables.value(1030), 0, $gameVariables.value(1037));

	// 侵蝕
	var erosionHigh = 100;
	$gameVariables._data[1022] = clamp($gameVariables.value(1022), $gameVariables.value(1280), erosionHigh);

	// 知名度
	$gameVariables._data[1025] = clamp($gameVariables.value(1025), 0, 100);

	// 魂
	$gameVariables._data[217] = clamp($gameVariables.value(217), 0, 99999);

	// 学園評価
	$gameVariables._data[1023] = clamp($gameVariables.value(1023), -100, 100);

	// 市民評価
	$gameVariables._data[1024] = clamp($gameVariables.value(1024), -100, 100);

	// 支配度
	$gameVariables._data[212] = clamp($gameVariables.value(212), 0, 100);

	// 性感
	var extasyHigh = $gameVariables.value(1031);
	$gameVariables._data[1026] = clamp($gameVariables.value(1026), 0, extasyHigh);

	// 衣装耐久
	$gameVariables._data[702] = clamp($gameVariables.value(702), 0, $gameVariables.value(722));

	// ターン
	$gameVariables._data[202] = clamp($gameVariables.value(202), 0, 999);

	// 催眠
	$gameVariables._data[1019] = clamp($gameVariables.value(1019), 0, 100);

	if ($gameVariables.value(1020) > 30) $gameVariables._data[1020] = 30;

	// 衣装耐久ゲージ表示用
	$gameVariables._data[741] = $gameVariables.value(702);
	$gameVariables._data[742] = $gameVariables.value(722);
})();
`

// execCulSkillEffect 在 Goja VM 中执行 CulSkillEffect 插件逻辑。
func (e *Executor) execCulSkillEffect(s *player.PlayerSession, opts *ExecuteOpts) {
	if opts == nil || opts.GameState == nil {
		return
	}

	vm := goja.New()
	timer := time.AfterFunc(200*time.Millisecond, func() {
		vm.Interrupt("CulSkillEffect timeout")
	})
	defer timer.Stop()

	if opts.TransientVars == nil {
		opts.TransientVars = make(map[int]interface{})
	}
	mutations := &scriptMutations{
		varChanges:    make(map[int]int),
		switchChanges: make(map[int]bool),
		varAnyChanges: opts.TransientVars,
	}

	injectScriptMath(vm)
	injectScriptGameStateMutable(vm, opts.GameState, true, mutations)
	if s != nil {
		injectScriptGameActors(vm, s)
	}
	if e.res != nil {
		injectScriptDataArrays(vm, e.res)
	}

	var runErr error
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, runErr = vm.RunString(culSkillEffectJS)
	}()
	if panicked || runErr != nil {
		e.logger.Warn("CulSkillEffect execution failed", zap.Error(runErr))
		return
	}

	// Apply mutations
	for id, val := range mutations.varChanges {
		opts.GameState.SetVariable(id, val)
		e.sendVarChange(s, id, val)
	}
	for id, val := range mutations.switchChanges {
		opts.GameState.SetSwitch(id, val)
		e.sendSwitchChange(s, id, val)
	}

	// AddSkillEffectBase: apply TagSkillList base values + accumulated effects.
	// For each entry (index 21-122): v[BaseVar] = BaseNum + v[AddVar]
	if e.res != nil && e.res.TagSkillList != nil {
		for i := 21; i <= 122; i++ {
			entry := e.res.TagSkillList[i]
			if entry == nil {
				continue
			}
			addVal := opts.GameState.GetVariable(entry.AddVar)
			result := entry.BaseNum + addVal
			if result != opts.GameState.GetVariable(entry.BaseVar) {
				opts.GameState.SetVariable(entry.BaseVar, result)
				e.sendVarChange(s, entry.BaseVar, result)
			}
		}
	}

	e.logger.Info("CulSkillEffect executed",
		zap.Int64("char_id", s.CharID),
		zap.Int("v1209", opts.GameState.GetVariable(1209)))
}

// execParaCheck 在 Goja VM 中执行 ParaCheck 插件核心逻辑。
func (e *Executor) execParaCheck(s *player.PlayerSession, opts *ExecuteOpts) {
	if opts == nil || opts.GameState == nil {
		return
	}

	vm := goja.New()
	timer := time.AfterFunc(200*time.Millisecond, func() {
		vm.Interrupt("ParaCheck timeout")
	})
	defer timer.Stop()

	if opts.TransientVars == nil {
		opts.TransientVars = make(map[int]interface{})
	}
	mutations := &scriptMutations{
		varChanges:    make(map[int]int),
		switchChanges: make(map[int]bool),
		varAnyChanges: opts.TransientVars,
	}

	injectScriptMath(vm)
	injectScriptGameStateMutable(vm, opts.GameState, true, mutations)
	if s != nil {
		injectScriptGameActors(vm, s)
	}
	if e.res != nil {
		injectScriptDataArrays(vm, e.res)
	}

	// Inject player-specific values not available in standard VM
	vm.Set("__playerLevel", s.Level)
	gold := int64(0)
	if e.store != nil {
		g, err := e.store.GetGold(nil, s.CharID)
		if err == nil {
			gold = g
		}
	}
	vm.Set("__gold", gold)
	vm.Set("__classId", s.ClassID)

	var runErr error
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, runErr = vm.RunString(paraCheckCoreJS)
	}()
	if panicked || runErr != nil {
		e.logger.Warn("ParaCheck execution failed", zap.Error(runErr))
		return
	}

	// Apply mutations
	for id, val := range mutations.varChanges {
		opts.GameState.SetVariable(id, val)
		e.sendVarChange(s, id, val)
	}
	for id, val := range mutations.switchChanges {
		opts.GameState.SetSwitch(id, val)
		e.sendSwitchChange(s, id, val)
	}

	e.logger.Info("ParaCheck executed",
		zap.Int64("char_id", s.CharID),
		zap.Int("v722", opts.GameState.GetVariable(722)),
		zap.Int("v702", opts.GameState.GetVariable(702)),
		zap.Int("v741", opts.GameState.GetVariable(741)),
		zap.Int("v742", opts.GameState.GetVariable(742)))
}
