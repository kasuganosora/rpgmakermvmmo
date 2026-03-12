package world

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/ai"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
)

// ---- Helpers ----

func benchLogger() *zap.Logger {
	l, _ := zap.NewDevelopment()
	if l == nil {
		l = zap.NewNop()
	}
	return l
}

// benchPassMap creates a passability map of w×h, all passable.
func benchPassMap(w, h int) *resource.PassabilityMap {
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

// benchSession creates a minimal PlayerSession for benchmarks (no websocket).
func benchSession(charID int64, x, y int) *player.PlayerSession {
	s := &player.PlayerSession{
		CharID:   charID,
		CharName: "BenchPlayer",
		X:        x,
		Y:        y,
		Dir:      2,
		HP:       100,
		MaxHP:    100,
		MP:       50,
		MaxMP:    50,
		SendChan: make(chan []byte, 256),
		Done:     make(chan struct{}),
	}
	s.SetLogger(zap.NewNop())
	// Drain send channel in background to prevent blocking.
	go func() {
		for {
			select {
			case <-s.SendChan:
			case <-s.Done:
				return
			}
		}
	}()
	return s
}

// benchRoom creates a MapRoom with the given number of players spread across the map.
func benchRoom(numPlayers int, w, h int) *MapRoom {
	room := &MapRoom{
		MapID:           1,
		players:         make(map[int64]*player.PlayerSession),
		npcs:            []*NPCRuntime{},
		monsters:        []*MonsterInstance{},
		runtimeMonsters: make(map[int64]*MonsterRuntime),
		drops:           make(map[int64]*DropRuntimeEntry),
		broadcastQ:      make(chan []byte, 2048),
		stopCh:          make(chan struct{}),
		logger:          zap.NewNop(),
		passMap:         benchPassMap(w, h),
		mapWidth:        w,
		mapHeight:       h,
	}
	for i := 0; i < numPlayers; i++ {
		s := benchSession(int64(i+1), i%w, i/w%h)
		room.players[s.CharID] = s
	}
	return room
}

// cleanup closes all player sessions in the room.
func cleanupRoom(room *MapRoom) {
	for _, s := range room.players {
		select {
		case <-s.Done:
		default:
			close(s.Done)
		}
	}
}

// ---- MapRoom Tick Benchmarks ----

func BenchmarkBroadcastRaw_10Players(b *testing.B) {
	room := benchRoom(10, 20, 15)
	defer cleanupRoom(room)
	data := []byte(`{"type":"test","payload":null}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.broadcastRaw(data)
	}
}

func BenchmarkBroadcastRaw_50Players(b *testing.B) {
	room := benchRoom(50, 50, 50)
	defer cleanupRoom(room)
	data := []byte(`{"type":"test","payload":null}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.broadcastRaw(data)
	}
}

func BenchmarkBroadcastRaw_100Players(b *testing.B) {
	room := benchRoom(100, 100, 100)
	defer cleanupRoom(room)
	data := []byte(`{"type":"test","payload":null}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.broadcastRaw(data)
	}
}

// ---- JSON Serialization (player_sync pattern) ----

func BenchmarkPlayerSyncMarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payload, _ := json.Marshal(&playerSyncPayload{
			CharID: 1,
			X:      10,
			Y:      15,
			Dir:    2,
			HP:     100,
			MP:     50,
			State:  "normal",
		})
		json.Marshal(&player.Packet{
			Type:    "player_sync",
			Payload: payload,
		})
	}
}

func BenchmarkMonsterSyncMarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payload, _ := json.Marshal(&monsterSyncPayload{
			InstID: 1,
			X:      10,
			Y:      15,
			Dir:    2,
			HP:     80,
			MaxHP:  100,
			Name:   "Slime",
			State:  0,
		})
		json.Marshal(&player.Packet{Type: "monster_sync", Payload: payload})
	}
}

// ---- broadcastDirtyPlayers simulation ----

func BenchmarkBroadcastDirtyPlayers_10(b *testing.B) {
	room := benchRoom(10, 20, 15)
	defer cleanupRoom(room)
	// Mark all players dirty.
	for _, s := range room.players {
		s.Dirty = true
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Re-dirty all players each iteration.
		for _, s := range room.players {
			s.Dirty = true
		}
		room.broadcastDirtyPlayers()
	}
}

func BenchmarkBroadcastDirtyPlayers_50(b *testing.B) {
	room := benchRoom(50, 50, 50)
	defer cleanupRoom(room)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, s := range room.players {
			s.Dirty = true
		}
		room.broadcastDirtyPlayers()
	}
}

// ---- NPC Movement Benchmarks ----

func BenchmarkTryMoveNPC_Passable(b *testing.B) {
	room := benchRoom(0, 20, 15)
	defer cleanupRoom(room)
	npc := &NPCRuntime{
		EventID: 1,
		X:       10,
		Y:       7,
		Dir:     2,
		ActivePage: &resource.EventPage{PriorityType: 1},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		npc.X = 10
		npc.Y = 7
		room.tryMoveNPC(npc, dirDown)
	}
}

func BenchmarkTryMoveNPC_WithCollision_20NPCs(b *testing.B) {
	room := benchRoom(0, 20, 15)
	defer cleanupRoom(room)
	// Add 20 NPCs scattered around.
	for i := 0; i < 20; i++ {
		room.npcs = append(room.npcs, &NPCRuntime{
			EventID: i + 100,
			X:       i % 20,
			Y:       i / 20,
			Dir:     2,
			ActivePage: &resource.EventPage{PriorityType: 1},
		})
	}
	npc := room.npcs[0]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		npc.X = 10
		npc.Y = 7
		room.tryMoveNPC(npc, dirDown)
	}
}

func BenchmarkTryMoveNPC_WithCollision_100NPCs(b *testing.B) {
	room := benchRoom(0, 100, 100)
	defer cleanupRoom(room)
	for i := 0; i < 100; i++ {
		room.npcs = append(room.npcs, &NPCRuntime{
			EventID: i + 100,
			X:       i % 100,
			Y:       i / 100,
			Dir:     2,
			ActivePage: &resource.EventPage{PriorityType: 1},
		})
	}
	npc := room.npcs[0]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		npc.X = 50
		npc.Y = 50
		room.tryMoveNPC(npc, dirDown)
	}
}

// ---- TryMoveMonster Benchmark ----

func BenchmarkTryMoveMonster(b *testing.B) {
	room := benchRoom(0, 20, 15)
	defer cleanupRoom(room)
	m := &MonsterRuntime{
		InstID:  1,
		X:       10,
		Y:       7,
		Dir:     2,
		HP:      100,
		MaxHP:   100,
		State:   ai.StateIdle,
		Threat:  ai.NewThreatTable(),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.X = 10
		m.Y = 7
		room.TryMoveMonster(m, 2) // down
	}
}

// ---- tickMonsters Benchmark ----

func benchRoomWithMonsters(numMonsters, numPlayers int) *MapRoom {
	room := benchRoom(numPlayers, 50, 50)
	for i := 0; i < numMonsters; i++ {
		enemy := &resource.Enemy{ID: 1, Name: "Slime", HP: 100, Agi: 10}
		m := NewMonster(enemy, 0, 10+i%40, 10+i/40)
		m.Profile = ai.DefaultProfiles["aggressive"]
		m.AITree = ai.BuildTree(m.Profile)
		room.runtimeMonsters[m.InstID] = m
	}
	return room
}

func BenchmarkTickMonsters_10Monsters_5Players(b *testing.B) {
	room := benchRoomWithMonsters(10, 5)
	defer cleanupRoom(room)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.tickMonsters()
	}
}

func BenchmarkTickMonsters_50Monsters_10Players(b *testing.B) {
	room := benchRoomWithMonsters(50, 10)
	defer cleanupRoom(room)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.tickMonsters()
	}
}

func BenchmarkTickMonsters_100Monsters_20Players(b *testing.B) {
	room := benchRoomWithMonsters(100, 20)
	defer cleanupRoom(room)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.tickMonsters()
	}
}

// ---- PlayerSession Benchmarks ----

func BenchmarkPlayerPosition(b *testing.B) {
	s := benchSession(1, 10, 15)
	defer func() { close(s.Done) }()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Position()
	}
}

func BenchmarkPlayerStats(b *testing.B) {
	s := benchSession(1, 10, 15)
	defer func() { close(s.Done) }()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Stats()
	}
}

func BenchmarkPlayerSendRaw(b *testing.B) {
	s := benchSession(1, 10, 15)
	defer func() { close(s.Done) }()
	data := []byte(`{"type":"test"}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.SendRaw(data)
	}
}

func BenchmarkPlayerRateLimit(b *testing.B) {
	s := benchSession(1, 10, 15)
	defer func() { close(s.Done) }()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.RateLimit()
	}
}

// ---- PlayersInRange Benchmark ----

func BenchmarkPlayersInRange_20Players(b *testing.B) {
	room := benchRoom(20, 50, 50)
	defer cleanupRoom(room)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.PlayersInRange(25, 25, 8)
	}
}

func BenchmarkPlayersInRange_100Players(b *testing.B) {
	room := benchRoom(100, 100, 100)
	defer cleanupRoom(room)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.PlayersInRange(50, 50, 8)
	}
}

// ---- Full Tick Benchmark ----

func BenchmarkFullTick_10Players_20NPCs_10Monsters(b *testing.B) {
	room := benchRoomWithMonsters(10, 10)
	defer cleanupRoom(room)
	// Add NPCs.
	for i := 0; i < 20; i++ {
		room.npcs = append(room.npcs, &NPCRuntime{
			EventID: i + 1,
			X:       i % 20,
			Y:       i / 20,
			Dir:     2,
			ActivePage: &resource.EventPage{
				MoveType:     1, // random
				MoveFrequency: 3,
				PriorityType: 1,
			},
			moveTimer: 1, // will decrement each tick
		})
	}
	// Mark some players dirty.
	cnt := 0
	for _, s := range room.players {
		if cnt < 5 {
			s.Dirty = true
		}
		cnt++
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.tick()
	}
}

// ---- Concurrent access benchmarks ----

func BenchmarkBroadcastRaw_Concurrent(b *testing.B) {
	room := benchRoom(20, 50, 50)
	defer cleanupRoom(room)
	data := []byte(`{"type":"test","payload":null}`)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			room.broadcastRaw(data)
		}
	})
}

func BenchmarkPlayerPosition_Concurrent(b *testing.B) {
	s := benchSession(1, 10, 15)
	defer func() { close(s.Done) }()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Position()
		}
	})
}

// ---- Lock contention benchmark ----

func BenchmarkMixedReadWrite_Position(b *testing.B) {
	s := benchSession(1, 10, 15)
	defer func() { close(s.Done) }()
	var wg sync.WaitGroup
	// Start writer goroutine.
	stopCh := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopCh:
				return
			default:
				s.SetPosition(10, 15, 2)
			}
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Position()
	}
	b.StopTimer()
	close(stopCh)
	wg.Wait()
}
