package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/world"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const resetPosCooldown = 3 * time.Minute

// GameHandlers bundles the dependencies needed by in-game WS message handlers.
type GameHandlers struct {
	db     *gorm.DB
	wm     *world.WorldManager
	sm     *player.SessionManager
	res    *resource.ResourceLoader
	logger *zap.Logger
}

// NewGameHandlers creates a new GameHandlers.
func NewGameHandlers(db *gorm.DB, wm *world.WorldManager, sm *player.SessionManager, res *resource.ResourceLoader, logger *zap.Logger) *GameHandlers {
	return &GameHandlers{db: db, wm: wm, sm: sm, res: res, logger: logger}
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

	// Determine spawn position and validate passability.
	spawnX, spawnY, spawnDir := char.MapX, char.MapY, char.Direction
	// Validate DB position is still passable (map may have been updated).
	spawnX, spawnY = gh.findNearestPassable(mapID, spawnX, spawnY)

	gh.enterMapRoom(s, mapID, spawnX, spawnY, spawnDir)
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

	// Distance check: allow at most 1 tile per tick with 1.3× tolerance.
	dx := req.X - curX
	dy := req.Y - curY
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if float64(dx+dy) > maxMovePerTick {
		gh.logger.Warn("speed hack detected",
			zap.Int64("char_id", s.CharID),
			zap.Int("dx", dx), zap.Int("dy", dy))
		sendError(s, "move rejected: speed violation")
		return nil
	}

	// Passability check.
	if gh.res != nil {
		pm := gh.res.Passability[s.MapID]
		if pm != nil && !pm.CanPass(req.X, req.Y, req.Dir) {
			sendError(s, "move rejected: impassable tile")
			return nil
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
	if room := gh.wm.Get(s.MapID); room != nil {
		room.BroadcastExcept(syncPkt, s.CharID)
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

	// Validate destination passability, find nearest passable if needed.
	destX, destY := gh.findNearestPassable(req.MapID, req.X, req.Y)
	destDir := req.Dir
	if destDir <= 0 {
		destDir = 2
	}

	fromMap := s.MapID
	gh.enterMapRoom(s, req.MapID, destX, destY, destDir)

	gh.logger.Info("map transfer",
		zap.Int64("char_id", s.CharID),
		zap.Int("from_map", fromMap),
		zap.Int("to_map", req.MapID),
		zap.Int("to_x", destX),
		zap.Int("to_y", destY))
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

	// Find passable tiles from server passability data.
	if gh.res == nil {
		sendError(s, "no resource data")
		return nil
	}
	pm := gh.res.Passability[s.MapID]
	if pm == nil {
		sendError(s, "no passability data for current map")
		return nil
	}

	type tile struct{ x, y int }
	var candidates []tile
	for y := 0; y < pm.Height; y++ {
		for x := 0; x < pm.Width; x++ {
			if pm.CanPass(x, y, 2) || pm.CanPass(x, y, 4) ||
				pm.CanPass(x, y, 6) || pm.CanPass(x, y, 8) {
				candidates = append(candidates, tile{x, y})
			}
		}
	}

	if len(candidates) == 0 {
		payload, _ := json.Marshal(map[string]interface{}{
			"error": "No walkable tile found on this map.",
		})
		s.Send(&player.Packet{Type: "reset_pos", Payload: payload})
		return nil
	}

	pick := candidates[rand.Intn(len(candidates))]
	s.SetPosition(pick.x, pick.y, 2)

	// Set cooldown.
	s.SetResetPosCooldown()

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
func (gh *GameHandlers) enterMapRoom(s *player.PlayerSession, mapID, x, y, dir int) {
	// Leave current map if any.
	if s.MapID != 0 {
		leaveMap(s, gh.wm, gh.logger)
	}

	s.MapID = mapID
	s.SetPosition(x, y, dir)

	// Join the target MapRoom.
	room := gh.wm.GetOrCreate(mapID)
	room.AddPlayer(s)

	// Push map_init to the joining player.
	x0, y0, dir0 := s.Position()
	initPayload, _ := json.Marshal(map[string]interface{}{
		"self": map[string]interface{}{
			"char_id":    s.CharID,
			"name":       s.CharName,
			"walk_name":  s.WalkName,
			"walk_index": s.WalkIndex,
			"level":      s.Level,
			"exp":        s.Exp,
			"hp":         s.HP,
			"max_hp":     s.MaxHP,
			"mp":         s.MP,
			"max_mp":     s.MaxMP,
			"x":          x0,
			"y":          y0,
			"dir":        dir0,
			"map_id":     s.MapID,
		},
		"players":  room.PlayerSnapshot(),
		"npcs":     []interface{}{},
		"monsters": []interface{}{},
		"drops":    []interface{}{},
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
