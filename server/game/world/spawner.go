package world

import (
	"sync"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/ai"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// Spawner manages monster respawn timers for a MapRoom.
type Spawner struct {
	room           *MapRoom
	res            *resource.ResourceLoader
	configs        []SpawnConfig
	customProfiles map[string]*ai.AIProfile // from MMOConfig
	mu             sync.Mutex
	stopCh         chan struct{}
	logger         *zap.Logger
}

// NewSpawner creates a Spawner for a MapRoom.
func NewSpawner(room *MapRoom, res *resource.ResourceLoader, configs []SpawnConfig, logger *zap.Logger) *Spawner {
	sp := &Spawner{room: room, res: res, configs: configs, stopCh: make(chan struct{}), logger: logger}
	// Convert MMOConfig custom profiles to ai.AIProfile.
	if res != nil && res.MMOConfig != nil && res.MMOConfig.MonsterAIProfiles != nil {
		sp.customProfiles = make(map[string]*ai.AIProfile, len(res.MMOConfig.MonsterAIProfiles))
		for name, rp := range res.MMOConfig.MonsterAIProfiles {
			sp.customProfiles[name] = &ai.AIProfile{
				Name:                name,
				AggroRange:          rp.AggroRange,
				LeashRange:          rp.LeashRange,
				AttackRange:         rp.AttackRange,
				AttackCooldownTicks: rp.AttackCooldownTicks,
				MoveIntervalTicks:   rp.MoveIntervalTicks,
				WanderRadius:        rp.WanderRadius,
				FleeHPPercent:       rp.FleeHPPercent,
			}
		}
	}
	return sp
}

// SpawnAll spawns all configured monsters immediately (called on room creation).
func (sp *Spawner) SpawnAll() {
	for i, cfg := range sp.configs {
		sp.spawnGroup(i, cfg)
	}
}

func (sp *Spawner) spawnGroup(cfgIndex int, cfg SpawnConfig) {
	if sp.res == nil {
		return
	}
	var template *resource.Enemy
	for _, e := range sp.res.Enemies {
		if e != nil && e.ID == cfg.MonsterID {
			template = e
			break
		}
	}
	if template == nil {
		sp.logger.Warn("unknown monster ID", zap.Int("monster_id", cfg.MonsterID))
		return
	}

	sp.room.mu.Lock()
	defer sp.room.mu.Unlock()
	// Count existing alive monsters for this spawnID.
	alive := 0
	for _, m := range sp.room.runtimeMonsters {
		if m.SpawnID == cfgIndex && m.State != ai.StateDead {
			alive++
		}
	}
	// Resolve AI profile: spawn config override > enemy Note > nil (no AI).
	var profile *ai.AIProfile
	if cfg.AIOverride != "" {
		profile = ai.ParseAIProfile("<AI:"+cfg.AIOverride+">", sp.customProfiles)
	}
	if profile == nil {
		profile = ai.ParseAIProfile(template.Note, sp.customProfiles)
	}
	var tree *ai.BehaviorTree
	if profile != nil {
		tree = ai.BuildTree(profile)
	}

	toSpawn := cfg.MaxCount - alive
	for i := 0; i < toSpawn; i++ {
		m := NewMonster(template, cfgIndex, cfg.X+i, cfg.Y)
		m.Profile = profile
		m.AITree = tree
		// Set spawn config for group assist.
		m.SpawnCfg = &cfg
		// Wire OnDamaged callback for group assist.
		if cfg.GroupID != "" && sp.room.groupMgr != nil {
			spIdx := cfgIndex
			m.OnDamaged = func(monster *MonsterRuntime, attackerCharID int64) {
				sp.room.groupMgr.OnMemberDamaged(spIdx, attackerCharID)
			}
			sp.room.groupMgr.Register(cfgIndex, cfg.GroupID, cfg.GroupType, m)
		}
		sp.room.runtimeMonsters[m.InstID] = m
		sp.room.monsters = append(sp.room.monsters, &MonsterInstance{
			ID:      m.InstID,
			EnemyID: template.ID,
			Name:    template.Name,
			X:       m.X,
			Y:       m.Y,
			HP:      m.HP,
			MaxHP:   m.MaxHP,
		})
	}
}

// CheckRespawns checks if any spawn points need new monsters.
// Should be called periodically (e.g., every few seconds from a scheduler).
func (sp *Spawner) CheckRespawns() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	for i, cfg := range sp.configs {
		sp.spawnGroup(i, cfg)
	}
}

// RespawnAfter schedules a single monster slot to respawn after a delay.
func (sp *Spawner) RespawnAfter(cfgIndex int, delay time.Duration) {
	if cfgIndex < 0 || cfgIndex >= len(sp.configs) {
		return
	}
	go func() {
		select {
		case <-time.After(delay):
			sp.mu.Lock()
			defer sp.mu.Unlock()
			sp.spawnGroup(cfgIndex, sp.configs[cfgIndex])
		case <-sp.stopCh:
			return
		}
	}()
}

// RespawnSec returns the configured respawn delay in seconds for a spawn group.
// Returns 0 if the cfgIndex is invalid or RespawnSec is not set (no auto-respawn).
func (sp *Spawner) RespawnSec(cfgIndex int) int {
	if cfgIndex < 0 || cfgIndex >= len(sp.configs) {
		return 0
	}
	return sp.configs[cfgIndex].RespawnSec
}

// Stop cancels any pending respawn goroutines. Safe to call multiple times.
func (sp *Spawner) Stop() {
	select {
	case <-sp.stopCh:
	default:
		close(sp.stopCh)
	}
}
