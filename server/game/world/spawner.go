package world

import (
	"sync"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// Spawner manages monster respawn timers for a MapRoom.
type Spawner struct {
	room    *MapRoom
	res     *resource.ResourceLoader
	configs []SpawnConfig
	mu      sync.Mutex
	logger  *zap.Logger
}

// NewSpawner creates a Spawner for a MapRoom.
func NewSpawner(room *MapRoom, res *resource.ResourceLoader, configs []SpawnConfig, logger *zap.Logger) *Spawner {
	return &Spawner{room: room, res: res, configs: configs, logger: logger}
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
		if m.SpawnID == cfgIndex && m.State != 6 { // 6 = StateDead equivalent
			alive++
		}
	}
	toSpawn := cfg.MaxCount - alive
	for i := 0; i < toSpawn; i++ {
		m := NewMonster(template, cfgIndex, cfg.X+i, cfg.Y)
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
	go func() {
		time.Sleep(delay)
		sp.mu.Lock()
		defer sp.mu.Unlock()
		sp.spawnGroup(cfgIndex, sp.configs[cfgIndex])
	}()
}
