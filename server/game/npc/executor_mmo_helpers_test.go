package npc

// executor_mmo_helpers_test.go — test helpers for MMOConfig-dependent tests.
// Provides inline script constants and helper functions so tests don't need
// real filesystem access to the projectb plugin scripts.

import "github.com/kasuganosora/rpgmakermvmmo/server/resource"

// culSkillEffectScript is the CulSkillEffect_server.js content used in tests.
// Kept in sync with projectb/www/js/plugins/CulSkillEffect_server.js.
const culSkillEffectScript = `
(function() {
	for (var i = 4221; i <= 4322; i++) { $gameVariables._data[i] = 0; }
	function SkillReset() { for (var i = 4211; i <= 4220; i++) { $gameVariables._data[i] = 0; } }
	function AddSkillEffect() {
		for (var i = 4211; i <= 4220; i++) {
			var v = $gameVariables.value(i);
			if (v != 0 && v !== undefined && v !== null) {
				if (typeof v === 'object' && v.length >= 2) { $gameVariables._data[v[0] + 4200] += v[1]; }
			}
		}
	}
	var equips = $gameActors._data[1]._equips;
	var Other1 = 4, Other2 = 5, Other1EqID = 0, EquipNo = 1;
	for (var i = 0; i < equips.length; i++) {
		var EquipID = 0;
		if (equips[EquipNo] != null) { EquipID = equips[EquipNo]._itemId; }
		if (EquipID >= 1 && $dataArmors[EquipID]) {
			if (EquipNo == Other1) { Other1EqID = EquipID; }
			if (EquipNo == Other2 && EquipID == Other1EqID) {
				// duplicate skip
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
	SkillReset();
	var naked = ($gameActors._data[1]._equips[1] && $gameActors._data[1]._equips[1]._itemId == 0) ? 40 : 0;
	$gameVariables._data[1200] += naked;
})();
`

// paraCheckScript is the ParaCheck_server.js content used in tests.
// Kept in sync with projectb/www/js/plugins/ParaCheck_server.js.
const paraCheckScript = `
(function() {
	$gameVariables._data[1006] = __playerLevel;
	$gameVariables._data[215]  = __gold;
	$gameVariables._data[722]  = $gameVariables.value(1209);
	$gameVariables._data[1178] = Math.floor($gameVariables.value(1280) / 15);
	if ($gameVariables.value(1178) > 6) { $gameVariables._data[1178] = 6; }
	for (var i = 0; i < 5; i++) {
		$gameSwitches._data[1031 + i] = ($gameVariables.value(1178) >= (i + 1));
	}
	$gameVariables._data[1111] = $gameVariables.value(1272);
	$gameVariables._data[1112] = $gameVariables.value(1273);
	$gameVariables._data[1113] = $gameVariables.value(1274);
	$gameVariables._data[722]  = $gameVariables.value(1209);
	$gameVariables._data[1034] = $gameVariables.value(1260);
	$gameVariables._data[1037] = $gameVariables.value(1259);
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
	if ($gameSwitches.value(131)) { $gameVariables._data[235] = 1; } else { $gameVariables._data[235] = 0; }
	$gameSwitches._data[162]  = ($gameVariables.value(1215) > 0);
	$gameSwitches._data[2917] = ($gameVariables.value(1210) > 0);
	function clamp(val, low, high) {
		if (typeof val !== 'number' || isNaN(val)) val = 0;
		return Math.max(low, Math.min(high, val));
	}
	if (__classId > 2) {
		$gameVariables._data[1027] = 0;
	} else {
		$gameVariables._data[1027] = clamp($gameVariables.value(1027), $gameVariables.value(1282), 200);
	}
	$gameVariables._data[1021] = clamp($gameVariables.value(1021), 0, $gameVariables.value(1034));
	$gameVariables._data[1029] = clamp($gameVariables.value(1029), 0, 100);
	$gameVariables._data[1030] = clamp($gameVariables.value(1030), 0, $gameVariables.value(1037));
	$gameVariables._data[1022] = clamp($gameVariables.value(1022), $gameVariables.value(1280), 100);
	$gameVariables._data[1025] = clamp($gameVariables.value(1025), 0, 100);
	$gameVariables._data[217]  = clamp($gameVariables.value(217), 0, 99999);
	$gameVariables._data[1023] = clamp($gameVariables.value(1023), -100, 100);
	$gameVariables._data[1024] = clamp($gameVariables.value(1024), -100, 100);
	$gameVariables._data[212]  = clamp($gameVariables.value(212), 0, 100);
	var extasyHigh = $gameVariables.value(1031);
	$gameVariables._data[1026] = clamp($gameVariables.value(1026), 0, extasyHigh);
	$gameVariables._data[702]  = clamp($gameVariables.value(702), 0, $gameVariables.value(722));
	$gameVariables._data[202]  = clamp($gameVariables.value(202), 0, 999);
	$gameVariables._data[1019] = clamp($gameVariables.value(1019), 0, 100);
	if ($gameVariables.value(1020) > 30) $gameVariables._data[1020] = 30;
	$gameVariables._data[741]  = $gameVariables.value(702);
	$gameVariables._data[742]  = $gameVariables.value(722);
})();
`

// testMMOConfig returns a *resource.MMOConfig pre-configured for unit tests.
// It includes CulSkillEffect + ParaCheck scripts, blocked plugin commands,
// safe script prefixes, and safe screen methods — matching the projectb config.
func testMMOConfig() *resource.MMOConfig {
	return &resource.MMOConfig{
		BlockedPluginCmds: []string{
			"CallStand", "CallStandForce",
			"EraceStand", "EraceStand1",
			"CallCutin", "EraceCutin",
			"CallAM",
			"CulPartLV", "CulLustLV", "CulMiasmaLV",
		},
		ServerExecPlugins: map[string]*resource.ServerExecPlugin{
			"CulSkillEffect": {
				LoadedScript:  culSkillEffectScript,
				Timeout:       500,
				TagSkillRange: []int{21, 100},
			},
			"ParaCheck": {
				LoadedScript: paraCheckScript,
				Timeout:      500,
			},
		},
		SafeScriptPrefixes: []string{"AudioManager."},
		SafeScreenMethods: []string{
			"movePicture", "erasePicture", "showPicture", "picture",
			"tintPicture", "rotatePicture",
			"startFadeOut", "startFadeIn", "startTint", "startFlash", "startShake",
			"setWeather", "showBalloon", "startZoom", "setZoom", "clearZoom",
			"updateFadeOut", "updateFadeIn", "clearPictures",
		},
	}
}

// withTestMMOConfig adds testMMOConfig() to the resource loader and returns it.
// This allows existing helpers like testResourceWithArmors to be augmented.
func withTestMMOConfig(res *resource.ResourceLoader) *resource.ResourceLoader {
	res.MMOConfig = testMMOConfig()
	return res
}

// testResMMO returns a minimal ResourceLoader with testMMOConfig and no armors.
// Use for dispatch/blocking/script-forwarding tests that don't need armors.
func testResMMO() *resource.ResourceLoader {
	return withTestMMOConfig(&resource.ResourceLoader{})
}
