package player

import (
	"github.com/kasuganosora/rpgmakermvmmo/server/game/battle"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// CalcStats computes the effective CharacterStats for a player character,
// incorporating base class stats (from Classes.json params[level]),
// equipment bonuses, and buff trait modifiers.
func CalcStats(char *model.Character, res *resource.ResourceLoader, equips []*resource.EquipStats) *battle.CharacterStats {
	stats := &battle.CharacterStats{
		HP:    char.HP,
		MP:    char.MP,
		Level: char.Level,
	}

	// Base stats from Classes.json params.
	if res != nil {
		for _, cls := range res.Classes {
			if cls == nil || cls.ID != char.ClassID {
				continue
			}
			level := char.Level
			if level < 1 {
				level = 1
			}
			// params[paramIndex][level-1] (0-indexed level)
			idx := level - 1
			// Params layout: [paramID][level], paramID: 0=maxHP,1=maxMP,2=atk,3=def,4=mat,5=mdf,6=agi,7=luk
			safeGet := func(paramID int) int {
				if paramID >= len(cls.Params) {
					return 0
				}
				row := cls.Params[paramID]
				if idx >= len(row) {
					return 0
				}
				return row[idx]
			}
			stats.Atk = safeGet(2)
			stats.Def = safeGet(3)
			stats.Mat = safeGet(4)
			stats.Mdf = safeGet(5)
			stats.Agi = safeGet(6)
			stats.Luk = safeGet(7)
			break
		}
	}

	// Equipment stat bonuses.
	for _, e := range equips {
		stats.Atk += e.Atk
		stats.Def += e.Def
		stats.Mat += e.Mat
		stats.Mdf += e.Mdf
		stats.Agi += e.Agi
		stats.Luk += e.Luk
	}

	// Fallback if class data missing.
	if stats.Atk == 0 {
		stats.Atk = char.Atk
		stats.Def = char.Def
		stats.Mat = char.Mat
		stats.Mdf = char.Mdf
		stats.Agi = char.Agi
		stats.Luk = char.Luk
	}

	return stats
}
