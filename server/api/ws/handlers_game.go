package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/battle"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const resetPosCooldown = 3 * time.Minute

// AutorunFunc is called after a player enters a map to execute autorun events.
type AutorunFunc func(s *player.PlayerSession, mapID int)

// GameHandlers bundles the dependencies needed by in-game WS message handlers.
type GameHandlers struct {
	db        *gorm.DB
	wm        *world.WorldManager
	sm        *player.SessionManager
	res       *resource.ResourceLoader
	logger    *zap.Logger
	autorunFn AutorunFunc // called after entering a map to execute autorun events
}

// NewGameHandlers creates a new GameHandlers.
func NewGameHandlers(db *gorm.DB, wm *world.WorldManager, sm *player.SessionManager, res *resource.ResourceLoader, logger *zap.Logger) *GameHandlers {
	return &GameHandlers{db: db, wm: wm, sm: sm, res: res, logger: logger}
}

// SetAutorunFunc sets the callback for executing autorun events when a player enters a map.
func (gh *GameHandlers) SetAutorunFunc(fn AutorunFunc) {
	gh.autorunFn = fn
}

// RegisterHandlers registers all in-game handlers on the given Router.
func (gh *GameHandlers) RegisterHandlers(r *Router) {
	r.On("ping", gh.HandlePing)
	r.On("enter_map", gh.HandleEnterMap)
	r.On("player_move", gh.HandleMove)
	r.On("map_transfer", gh.HandleMapTransfer)
	r.On("reset_pos", gh.HandleResetPos)
}

// ------------------------------------------------------------------ ping

type pingPayload struct {
	TS int64 `json:"ts"`
}

// HandlePing responds to client heartbeat pings.
func (gh *GameHandlers) HandlePing(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var p pingPayload
	_ = json.Unmarshal(raw, &p)
	s.SendHeartbeatPong(p.TS)
	return nil
}

// ------------------------------------------------------------------ enter_map

type enterMapReq struct {
	MapID  int   `json:"map_id"`
	CharID int64 `json:"char_id"`
	X      *int  `json:"x,omitempty"`
	Y      *int  `json:"y,omitempty"`
	Dir    *int  `json:"dir,omitempty"`
}

// HandleEnterMap processes an enter_map message from the client.
func (gh *GameHandlers) HandleEnterMap(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req enterMapReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}

	// Validate that the requested character belongs to this account.
	var char model.Character
	if err := gh.db.Where("id = ? AND account_id = ?", req.CharID, s.AccountID).First(&char).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			sendError(s, "invalid character")
			return nil
		}
		return err
	}

	// Populate session fields from DB.
	oldCharID := s.CharID
	s.CharID = char.ID
	s.CharName = char.Name
	s.WalkName = char.WalkName
	s.WalkIndex = char.WalkIndex
	s.FaceName = char.FaceName
	s.FaceIndex = char.FaceIndex
	s.ClassID = char.ClassID
	s.HP = char.HP
	s.MaxHP = char.MaxHP
	s.MP = char.MP
	s.MaxMP = char.MaxMP
	s.Level = char.Level
	s.Exp = char.Exp

	// Re-register in session manager under the correct CharID.
	// Initial registration happens with CharID=0 at WS connect time.
	if gh.sm != nil && s.CharID != oldCharID {
		gh.sm.Unregister(oldCharID)
		gh.sm.Register(s)
	}

	mapID := req.MapID
	if mapID <= 0 {
		mapID = char.MapID
	}
	if mapID <= 0 {
		mapID = 1
	}

	// Use saved position directly — map maker's coordinates are authoritative.
	spawnX, spawnY, spawnDir := char.MapX, char.MapY, char.Direction

	gh.EnterMapRoom(s, mapID, spawnX, spawnY, spawnDir)
	return nil
}

// ------------------------------------------------------------------ player_move

type moveReq struct {
	X   int    `json:"x"`
	Y   int    `json:"y"`
	Dir int    `json:"dir"`
	Seq uint64 `json:"seq"`
}

const maxMovePerTick = 1.3 // max tiles per 50ms tick with tolerance

// HandleMove validates and applies a movement request.
func (gh *GameHandlers) HandleMove(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req moveReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}

	curX, curY, _ := s.Position()

	// After a map transfer or resetpos, skip the speed check for a grace period.
	// The server has already switched maps but the client may still be mid-fade,
	// causing a position desync. Accepting moves without speed check allows the
	// server to re-sync with the client's actual position on the new map.
	inGrace := time.Since(s.LastTransfer) < 3*time.Second

	if !inGrace {
		// Distance check: allow at most 1 tile per tick with 1.3× tolerance.
		// Use Euclidean distance to properly handle diagonal movement.
		dx := float64(req.X - curX)
		dy := float64(req.Y - curY)
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist > maxMovePerTick {
			gh.logger.Warn("speed hack detected",
				zap.Int64("char_id", s.CharID),
				zap.Float64("dx", dx), zap.Float64("dy", dy),
				zap.Float64("distance", dist))
			sendMoveReject(s)
			return nil
		}
	}

	// Passability check — match RMMV's two-way check:
	// 1. Can leave current tile in the move direction
	// 2. Can enter destination tile from the reverse direction
	if gh.res != nil {
		pm := gh.res.Passability[s.MapID]
		if pm != nil {
			// Determine the movement direction from position delta.
			moveDir := req.Dir
			ddx, ddy := req.X-curX, req.Y-curY
			if ddx == 1 && ddy == 0 {
				moveDir = 6 // right
			} else if ddx == -1 && ddy == 0 {
				moveDir = 4 // left
			} else if ddx == 0 && ddy == 1 {
				moveDir = 2 // down
			} else if ddx == 0 && ddy == -1 {
				moveDir = 8 // up
			}
			// Check source tile: can leave in the move direction.
			if !pm.CanPass(curX, curY, moveDir) {
				sendMoveReject(s)
				return nil
			}
			// Check destination tile: can enter from the reverse direction.
			revDir := moveDir
			switch moveDir {
			case 2:
				revDir = 8
			case 4:
				revDir = 6
			case 6:
				revDir = 4
			case 8:
				revDir = 2
			}
			if !pm.CanPass(req.X, req.Y, revDir) {
				sendMoveReject(s)
				return nil
			}
		}
	}

	// Update position.
	s.SetPosition(req.X, req.Y, req.Dir)

	// Immediately broadcast for low latency, then clear dirty to avoid duplicate tick broadcast.
	s.ResetDirty()
	x, y, dir := s.Position()
	syncPayload, _ := json.Marshal(map[string]interface{}{
		"char_id":   s.CharID,
		"x":         x,
		"y":         y,
		"dir":       dir,
		"hp":        s.HP,
		"mp":        s.MP,
		"state":     "normal",
	})
	syncPkt, _ := json.Marshal(&player.Packet{Type: "player_sync", Payload: syncPayload})
	room := gh.wm.Get(s.MapID)
	if room != nil {
		room.BroadcastExcept(syncPkt, s.CharID)
	}

	// Server-side auto-transfer: check if player stepped on a transfer event
	// (trigger 1=Player Touch or 2=Event Touch with command 201=Transfer Player).
	// Since the client no longer processes events (_events = []), the server
	// must detect and execute map transfers.
	if room != nil {
		td := gh.getTransferForPlayer(s, room, req.X, req.Y)
		if td != nil && td.MapID > 0 {
			// Use exact coordinates from the map maker — do NOT adjust with
			// findNearestPassable. The BFS ring search ignores walls and can
			// place the player on the wrong side of a wall.
			destDir := td.Dir
			if destDir <= 0 {
				destDir = dir
			}
			fromMap := s.MapID
			gh.EnterMapRoom(s, td.MapID, td.X, td.Y, destDir)
			gh.logger.Info("auto-transfer triggered",
				zap.Int64("char_id", s.CharID),
				zap.Int("from_map", fromMap),
				zap.Int("to_map", td.MapID),
				zap.Int("to_x", td.X),
				zap.Int("to_y", td.Y))
		}
	}

	return nil
}

// ------------------------------------------------------------------ map_transfer

type mapTransferReq struct {
	MapID int `json:"map_id"`
	X     int `json:"x"`
	Y     int `json:"y"`
	Dir   int `json:"dir"`
}

// HandleMapTransfer processes a client request to transfer to another map.
// Triggered when the RMMV event interpreter encounters command 201 (Transfer Player).
// The server validates the destination and moves the player to the new map room.
func (gh *GameHandlers) HandleMapTransfer(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var req mapTransferReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return err
	}

	if req.MapID <= 0 {
		sendError(s, "invalid map_id")
		return nil
	}

	// Use exact coordinates from the event — the map maker's positions are authoritative.
	destDir := req.Dir
	if destDir <= 0 {
		destDir = 2
	}

	fromMap := s.MapID
	gh.EnterMapRoom(s, req.MapID, req.X, req.Y, destDir)

	gh.logger.Info("map transfer",
		zap.Int64("char_id", s.CharID),
		zap.Int("from_map", fromMap),
		zap.Int("to_map", req.MapID),
		zap.Int("to_x", req.X),
		zap.Int("to_y", req.Y))
	return nil
}

// ------------------------------------------------------------------ reset_pos

// HandleResetPos teleports the player to a random passable tile on their current map.
// 3-minute cooldown to prevent abuse.
func (gh *GameHandlers) HandleResetPos(_ context.Context, s *player.PlayerSession, _ json.RawMessage) error {
	// Cooldown check.
	since := s.CheckResetPosCooldown()
	if since < resetPosCooldown {
		remaining := resetPosCooldown - since
		secs := int(remaining.Seconds())
		payload, _ := json.Marshal(map[string]interface{}{
			"error": "Cooldown: " + formatCooldown(secs) + " remaining",
		})
		s.Send(&player.Packet{Type: "reset_pos", Payload: payload})
		return nil
	}

	// Use incoming transfer destinations as reset targets.
	// These are the coordinates where players arrive when transferring TO this map
	// from other maps — guaranteed valid positions within the playable area.
	incoming := gh.res.IncomingTransfers[s.MapID]
	if len(incoming) == 0 {
		payload, _ := json.Marshal(map[string]interface{}{
			"error": "No entry point found on this map.",
		})
		s.Send(&player.Packet{Type: "reset_pos", Payload: payload})
		return nil
	}

	// Pick the nearest entry point to the player's current position.
	curX, curY, _ := s.Position()
	bestIdx := 0
	bestDist := 999999
	for i, p := range incoming {
		dx := p.X - curX
		dy := p.Y - curY
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		dist := dx + dy
		if dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}
	type tile struct{ x, y int }
	pick := tile{incoming[bestIdx].X, incoming[bestIdx].Y}
	s.SetPosition(pick.x, pick.y, 2)

	// Set cooldown and transfer grace period (resetpos causes same desync as transfers).
	s.SetResetPosCooldown()
	s.LastTransfer = time.Now()

	// Send new position to the requesting player.
	payload, _ := json.Marshal(map[string]interface{}{
		"x": pick.x, "y": pick.y, "dir": 2,
	})
	s.Send(&player.Packet{Type: "reset_pos", Payload: payload})

	// Broadcast updated position to other players.
	syncPayload, _ := json.Marshal(map[string]interface{}{
		"char_id": s.CharID,
		"x":       pick.x,
		"y":       pick.y,
		"dir":     2,
		"hp":      s.HP,
		"mp":      s.MP,
		"state":   "normal",
	})
	syncPkt, _ := json.Marshal(&player.Packet{Type: "player_sync", Payload: syncPayload})
	if room := gh.wm.Get(s.MapID); room != nil {
		room.BroadcastExcept(syncPkt, s.CharID)
	}

	gh.logger.Info("player reset position",
		zap.Int64("char_id", s.CharID),
		zap.Int("map_id", s.MapID),
		zap.Int("x", pick.x),
		zap.Int("y", pick.y))
	return nil
}

func formatCooldown(totalSecs int) string {
	m := totalSecs / 60
	sec := totalSecs % 60
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, sec)
	}
	return fmt.Sprintf("%ds", sec)
}

// ------------------------------------------------------------------ enterMapRoom

// enterMapRoom moves a player into a map room at the given position,
// sends map_init, broadcasts player_join, and schedules protection end.
// It is used by HandleEnterMap (login) and server-side map transfers.
func (gh *GameHandlers) EnterMapRoom(s *player.PlayerSession, mapID, x, y, dir int) {
	// Leave current map if any.
	if s.MapID != 0 {
		leaveMap(s, gh.wm, gh.logger)
	}

	s.MapID = mapID
	s.SetPosition(x, y, dir)
	s.LastTransfer = time.Now()

	// Clear NPC channels to prevent stale dialog acks from previous map.
	s.ClearNPCChannels()

	// Join the target MapRoom.
	room := gh.wm.GetOrCreate(mapID)
	room.AddPlayer(s)

	// Push map_init to the joining player.
	x0, y0, dir0 := s.Position()

	// Compute class name from resource data.
	className := ""
	if gh.res != nil {
		if cls := gh.res.ClassByID(s.ClassID); cls != nil {
			className = cls.Name
		}
	}

	// Load character from DB for gold and equipment stats.
	var char model.Character
	gold := int64(0)
	atk, def, mat, mdf, agi, luk := 0, 0, 0, 0, 0, 0
	if err := gh.db.First(&char, s.CharID).Error; err == nil {
		gold = char.Gold
		// Compute combat stats (base + class + equipment).
		var equips []*resource.EquipStats
		if gh.res != nil {
			var invItems []model.Inventory
			gh.db.Where("char_id = ? AND equipped = ?", s.CharID, true).Find(&invItems)
			for _, inv := range invItems {
				var es *resource.EquipStats
				if inv.Kind == 2 { // weapon
					for _, w := range gh.res.Weapons {
						if w != nil && w.ID == inv.ItemID {
							s := resource.EquipStatsFromParams(w.Params)
							es = &s
							break
						}
					}
				} else if inv.Kind == 3 { // armor
					for _, a := range gh.res.Armors {
						if a != nil && a.ID == inv.ItemID {
							s := resource.EquipStatsFromParams(a.Params)
							es = &s
							break
						}
					}
				}
				if es != nil {
					equips = append(equips, es)
				}
			}
		}
		stats := player.CalcStats(&char, gh.res, equips)
		atk = stats.Atk
		def = stats.Def
		mat = stats.Mat
		mdf = stats.Mdf
		agi = stats.Agi
		luk = stats.Luk
	}

	// Load character skills from DB and resolve display data from resources.
	var charSkills []model.CharSkill
	gh.db.Where("char_id = ?", s.CharID).Find(&charSkills)
	var skillList []map[string]interface{}
	for _, cs := range charSkills {
		sk := gh.res.SkillByID(cs.SkillID)
		if sk == nil {
			continue
		}
		skillList = append(skillList, map[string]interface{}{
			"skill_id":   sk.ID,
			"name":       sk.Name,
			"icon_index": sk.IconIndex,
			"mp_cost":    sk.MPCost,
			"cd_ms":      1000, // default cooldown
		})
	}
	if skillList == nil {
		skillList = []map[string]interface{}{}
	}

	// Build map audio data from resource loader.
	var mapAudio map[string]interface{}
	if gh.res != nil {
		if md, ok := gh.res.Maps[mapID]; ok {
			mapAudio = map[string]interface{}{}
			if md.AutoplayBgm && md.Bgm != nil && md.Bgm.Name != "" {
				mapAudio["bgm"] = md.Bgm
			}
			if md.AutoplayBgs && md.Bgs != nil && md.Bgs.Name != "" {
				mapAudio["bgs"] = md.Bgs
			}
		}
	}

	initPayload, _ := json.Marshal(map[string]interface{}{
		"self": map[string]interface{}{
			"char_id":    s.CharID,
			"name":       s.CharName,
			"walk_name":  s.WalkName,
			"walk_index": s.WalkIndex,
			"face_name":  s.FaceName,
			"face_index": s.FaceIndex,
			"class_name": className,
			"level":      s.Level,
			"exp":        s.Exp,
			"next_exp":   battle.ExpNeeded(s.Level),
			"hp":         s.HP,
			"max_hp":     s.MaxHP,
			"mp":         s.MP,
			"max_mp":     s.MaxMP,
			"atk":        atk,
			"def":        def,
			"mat":        mat,
			"mdf":        mdf,
			"agi":        agi,
			"luk":        luk,
			"gold":       gold,
			"x":          x0,
			"y":          y0,
			"dir":        dir0,
			"map_id":     s.MapID,
		},
		"skills":      skillList,
		"players":     room.PlayerSnapshot(),
		"npcs":        gh.npcSnapshotForPlayer(s, room),
		"monsters":    room.MonsterSnapshot(),
		"drops":       []interface{}{},
		"passability": room.PassabilitySnapshot(),
		"audio":       mapAudio,
	})
	s.Send(&player.Packet{Type: "map_init", Payload: initPayload})

	// Broadcast player_join to everyone else.
	joinPayload, _ := json.Marshal(map[string]interface{}{
		"char_id":    s.CharID,
		"name":       s.CharName,
		"walk_name":  s.WalkName,
		"walk_index": s.WalkIndex,
		"x":          x0,
		"y":          y0,
		"dir":        dir0,
		"hp":         s.HP,
		"max_hp":     s.MaxHP,
		"mp":         s.MP,
		"max_mp":     s.MaxMP,
		"buffs":      []interface{}{},
	})
	joinPkt, _ := json.Marshal(&player.Packet{Type: "player_join", Payload: joinPayload})
	room.BroadcastExcept(joinPkt, s.CharID)

	// Schedule 3s protection end.
	go func() {
		time.Sleep(3 * time.Second)
		protPayload, _ := json.Marshal(map[string]interface{}{"char_id": s.CharID})
		s.Send(&player.Packet{Type: "protection_end", Payload: protPayload})
	}()

	gh.logger.Info("player entered map",
		zap.Int64("char_id", s.CharID),
		zap.Int("map_id", mapID))

	// Execute autorun events (trigger=3) for this player on the new map.
	if gh.autorunFn != nil {
		autorunFn := gh.autorunFn
		go func() {
			defer func() {
				if r := recover(); r != nil {
					gh.logger.Error("autorun panic recovered",
						zap.Int64("char_id", s.CharID),
						zap.Int("map_id", mapID),
						zap.Any("panic", r))
				}
			}()
			autorunFn(s, mapID)
		}()
	}
}

// findNearestPassable finds a passable tile near (x, y) on the given map.
// Searches in expanding rings (BFS). Returns the original coords if no passability data.
func (gh *GameHandlers) findNearestPassable(mapID, x, y int) (int, int) {
	if gh.res == nil {
		return x, y
	}
	pm := gh.res.Passability[mapID]
	if pm == nil {
		return x, y
	}
	// Check original position first.
	if pm.CanPass(x, y, 0) || pm.CanPass(x, y, 2) || pm.CanPass(x, y, 4) ||
		pm.CanPass(x, y, 6) || pm.CanPass(x, y, 8) {
		return x, y
	}
	// BFS expanding ring.
	for radius := 1; radius <= 10; radius++ {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if dx > -radius && dx < radius && dy > -radius && dy < radius {
					continue // skip inner ring (already checked)
				}
				nx, ny := x+dx, y+dy
				if nx < 0 || ny < 0 || nx >= pm.Width || ny >= pm.Height {
					continue
				}
				if pm.CanPass(nx, ny, 2) || pm.CanPass(nx, ny, 4) ||
					pm.CanPass(nx, ny, 6) || pm.CanPass(nx, ny, 8) {
					return nx, ny
				}
			}
		}
	}
	return x, y
}

// ------------------------------------------------------------------ helpers

// leaveMap removes the player from their current map room and broadcasts player_leave.
func leaveMap(s *player.PlayerSession, wm *world.WorldManager, logger *zap.Logger) {
	room := wm.Get(s.MapID)
	if room == nil {
		return
	}
	room.RemovePlayer(s.CharID)

	leavePayload, _ := json.Marshal(map[string]interface{}{"char_id": s.CharID})
	leavePkt, _ := json.Marshal(&player.Packet{Type: "player_leave", Payload: leavePayload})
	room.Broadcast(leavePkt)

	logger.Info("player left map",
		zap.Int64("char_id", s.CharID),
		zap.Int("map_id", s.MapID))
}

func sendError(s *player.PlayerSession, msg string) {
	payload, _ := json.Marshal(map[string]string{"message": msg})
	s.Send(&player.Packet{Type: "error", Payload: payload})
}

// sendMoveReject sends the server's authoritative position to the client
// so it can correct any position desync caused by a rejected move.
func sendMoveReject(s *player.PlayerSession) {
	x, y, dir := s.Position()
	payload, _ := json.Marshal(map[string]interface{}{
		"x":   x,
		"y":   y,
		"dir": dir,
	})
	s.Send(&player.Packet{Type: "move_reject", Payload: payload})
}

// npcSnapshotForPlayer returns NPC snapshots using the player's per-player state.
// Falls back to the global snapshot on error.
func (gh *GameHandlers) npcSnapshotForPlayer(s *player.PlayerSession, room *world.MapRoom) []map[string]interface{} {
	composite, err := gh.wm.PlayerStateManager().GetComposite(s.CharID)
	if err != nil {
		gh.logger.Error("failed to get player state for npc snapshot", zap.Error(err))
		return room.NPCSnapshot()
	}
	return room.NPCSnapshotForPlayer(composite)
}

// getTransferForPlayer checks transfer events using the player's per-player state.
// Falls back to the global check on error.
func (gh *GameHandlers) getTransferForPlayer(s *player.PlayerSession, room *world.MapRoom, x, y int) *world.TransferData {
	composite, err := gh.wm.PlayerStateManager().GetComposite(s.CharID)
	if err != nil {
		return room.GetTransferAt(x, y)
	}
	return room.GetTransferAtForPlayer(x, y, composite)
}
