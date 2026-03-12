package ai

import (
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// ---- A* Pathfinding Benchmarks ----

// makePassMap creates an open passability map of size w×h (all passable).
func makePassMap(w, h int) *resource.PassabilityMap {
	pm := resource.NewPassabilityMap(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pm.SetPass(x, y, 2, true)
			pm.SetPass(x, y, 4, true)
			pm.SetPass(x, y, 6, true)
			pm.SetPass(x, y, 8, true)
		}
	}
	return pm
}

func BenchmarkAStar_20x15_Short(b *testing.B) {
	pm := makePassMap(20, 15)
	from := Point{0, 0}
	to := Point{5, 5}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AStar(pm, from, to)
	}
}

func BenchmarkAStar_20x15_Long(b *testing.B) {
	pm := makePassMap(20, 15)
	from := Point{0, 0}
	to := Point{19, 14}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AStar(pm, from, to)
	}
}

func BenchmarkAStar_50x50(b *testing.B) {
	pm := makePassMap(50, 50)
	from := Point{0, 0}
	to := Point{49, 49}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AStar(pm, from, to)
	}
}

func BenchmarkAStar_100x100(b *testing.B) {
	pm := makePassMap(100, 100)
	from := Point{0, 0}
	to := Point{99, 99}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AStar(pm, from, to)
	}
}

// ---- CanPass Benchmark (inner loop of pathfinding) ----

func BenchmarkCanPass(b *testing.B) {
	pm := makePassMap(50, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm.CanPass(25, 25, 2)
		pm.CanPass(25, 25, 4)
		pm.CanPass(25, 25, 6)
		pm.CanPass(25, 25, 8)
	}
}

// ---- Behavior Tree Benchmarks ----

func BenchmarkBehaviorTree_PassiveTick(b *testing.B) {
	bt := BuildTree(DefaultProfiles["passive"])
	m := &mockMonster{x: 5, y: 5, spawnX: 5, spawnY: 5}
	room := &mockRoom{moveResult: true}
	ctx := &AIContext{Monster: m, Room: room, Config: DefaultProfiles["passive"]}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.moveTimer = 0 // allow move each iteration
		bt.Tick(ctx)
	}
}

func BenchmarkBehaviorTree_AggressiveTick_NoTarget(b *testing.B) {
	bt := BuildTree(DefaultProfiles["aggressive"])
	m := &mockMonster{x: 5, y: 5, spawnX: 5, spawnY: 5}
	room := &mockRoom{moveResult: true}
	ctx := &AIContext{Monster: m, Room: room, Config: DefaultProfiles["aggressive"]}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.moveTimer = 0
		m.target = 0
		bt.Tick(ctx)
	}
}

func BenchmarkBehaviorTree_AggressiveTick_WithTarget(b *testing.B) {
	bt := BuildTree(DefaultProfiles["aggressive"])
	m := &mockMonster{x: 5, y: 5, spawnX: 5, spawnY: 5, target: 1}
	room := &mockRoom{
		players:    []PlayerInfo{{CharID: 1, X: 6, Y: 5, HP: 100}},
		moveResult: true,
	}
	ctx := &AIContext{
		Monster:        m,
		Room:           room,
		Config:         DefaultProfiles["aggressive"],
		DamageCallback: func(MonsterAccessor, int64) {},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.attackTimer = 0
		bt.Tick(ctx)
	}
}

func BenchmarkBehaviorTree_AggressiveTick_Chase(b *testing.B) {
	bt := BuildTree(DefaultProfiles["aggressive"])
	m := &mockMonster{x: 5, y: 5, spawnX: 5, spawnY: 5, target: 1}
	room := &mockRoom{
		players:    []PlayerInfo{{CharID: 1, X: 10, Y: 10, HP: 100}},
		moveResult: true,
	}
	ctx := &AIContext{Monster: m, Room: room, Config: DefaultProfiles["aggressive"]}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.moveTimer = 0
		m.x, m.y = 5, 5
		bt.Tick(ctx)
	}
}

// ---- ThreatTable Benchmarks ----

func BenchmarkThreatTable_AddThreat(b *testing.B) {
	tt := NewThreatTable()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.AddThreat(int64(i%20), 50)
	}
}

func BenchmarkThreatTable_TopThreat_20Entries(b *testing.B) {
	tt := NewThreatTable()
	for i := 0; i < 20; i++ {
		tt.AddThreat(int64(i+1), (i+1)*10)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.TopThreat()
	}
}

func BenchmarkThreatTable_Decay(b *testing.B) {
	tt := NewThreatTable()
	for i := 0; i < 20; i++ {
		tt.AddThreat(int64(i+1), 1000)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.Decay(5) // typical 5% per tick
	}
}

// ---- Node-level Benchmarks ----

func BenchmarkCheckPlayerInRange_20Players(b *testing.B) {
	m := &mockMonster{x: 50, y: 50}
	players := make([]PlayerInfo, 20)
	for i := 0; i < 20; i++ {
		players[i] = PlayerInfo{CharID: int64(i + 1), X: 45 + i%10, Y: 45 + i/10, HP: 100}
	}
	room := &mockRoom{players: players}
	ctx := &AIContext{Monster: m, Room: room, Config: &AIProfile{AggroRange: 8}}
	node := &CheckPlayerInRange{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.target = 0
		node.Tick(ctx)
	}
}

func BenchmarkChaseTarget_SimpleMove(b *testing.B) {
	m := &mockMonster{x: 5, y: 5, target: 1}
	room := &mockRoom{
		players:    []PlayerInfo{{CharID: 1, X: 15, Y: 15, HP: 100}},
		moveResult: true,
	}
	ctx := &AIContext{Monster: m, Room: room, Config: &AIProfile{MoveIntervalTicks: 10}}
	node := &ChaseTarget{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.moveTimer = 0
		m.x, m.y = 5, 5
		m.cachedPath = nil
		node.Tick(ctx)
	}
}

// ---- Config Parsing Benchmarks ----

func BenchmarkParseAIProfile(b *testing.B) {
	note := "<AI:aggressive>\n<AI Aggro Range:10>\n<AI Attack Range:2>\n<AI Flee HP:25>"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseAIProfile(note, nil)
	}
}

func BenchmarkParseAIProfile_NoTags(b *testing.B) {
	note := "A normal enemy with no AI tags at all."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseAIProfile(note, nil)
	}
}
