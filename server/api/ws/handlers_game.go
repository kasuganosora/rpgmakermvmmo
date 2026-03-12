package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/battle"
	"github.com/kasuganosora/rpgmakermvmmo/server/game/party"
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

// TouchEventFunc is called when a player steps onto a trigger 1/2 event
// that has no top-level transfer command (requires full executor execution).
type TouchEventFunc func(s *player.PlayerSession, mapID, x, y int)

// ParallelFunc is called after a player enters a map to start parallel events.
type ParallelFunc func(s *player.PlayerSession, mapID int, gen uint64)

// GameHandlers bundles the dependencies needed by in-game WS message handlers.
type GameHandlers struct {
	db           *gorm.DB
	wm           *world.WorldManager
	sm           *player.SessionManager
	res          *resource.ResourceLoader
	logger       *zap.Logger
	autorunFn    AutorunFunc    // called after entering a map to execute autorun events
	touchEventFn TouchEventFunc // called when player steps on a touch-trigger event
	parallelFn   ParallelFunc   // called after entering a map to start parallel events
	partyMgr     *party.Manager // party manager for instance map support
}

// NewGameHandlers creates a new GameHandlers.
func NewGameHandlers(db *gorm.DB, wm *world.WorldManager, sm *player.SessionManager, res *resource.ResourceLoader, logger *zap.Logger) *GameHandlers {
	return &GameHandlers{db: db, wm: wm, sm: sm, res: res, logger: logger}
}

// getInitState returns InitState from resource loader, or nil if res is nil.
func (gh *GameHandlers) getInitState() interface{} {
	if gh.res == nil {
		return nil
	}
	return gh.res.InitState
}

// getCombatMode returns the effective combat mode from MMOConfig, defaulting to "hybrid".
func (gh *GameHandlers) getCombatMode() string {
	if gh.res != nil && gh.res.MMOConfig != nil && gh.res.MMOConfig.Battle != nil {
		return gh.res.MMOConfig.Battle.GetCombatMode()
	}
	return "hybrid"
}

// SetAutorunFunc sets the callback for executing autorun events when a player enters a map.
func (gh *GameHandlers) SetAutorunFunc(fn AutorunFunc) {
	gh.autorunFn = fn
}

// SetTouchEventFunc sets the callback for executing touch-trigger events during movement.
func (gh *GameHandlers) SetTouchEventFunc(fn TouchEventFunc) {
	gh.touchEventFn = fn
}

// SetParallelFunc sets the callback for starting parallel map events when a player enters a map.
func (gh *GameHandlers) SetParallelFunc(fn ParallelFunc) {
	gh.parallelFn = fn
}

// SetPartyManager sets the party manager used for party instance lookups.
func (gh *GameHandlers) SetPartyManager(pm *party.Manager) {
	gh.partyMgr = pm
}

// RegisterHandlers registers all in-game handlers on the given Router.
func (gh *GameHandlers) RegisterHandlers(r *Router) {
	r.On("ping", gh.HandlePing)
	r.On("enter_map", gh.HandleEnterMap)
	r.On("player_move", gh.HandleMove)
	r.On("map_transfer", gh.HandleMapTransfer)
	r.On("reset_pos", gh.HandleResetPos)
	r.On("client_log", gh.HandleClientLog)
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

	// 事件执行期间拒绝移动。
	if !s.EventMu.TryLock() {
		sendMoveReject(s)
		return nil
	}
	s.EventMu.Unlock() // 不在事件中，立即释放后继续正常处理。

	curX, curY, _ := s.Position()

	// Grace period after map transfer/resetpos: skip SPEED check only (not passability).
	// Reduced to 1s to minimize exploitation window. Passability always enforced.
	inGrace := !s.CheckTransferCooldown(1 * time.Second)

	if !inGrace {
		// Distance check: allow at most 1 tile per tick with 1.3× tolerance.
		// Use Euclidean distance to properly handle diagonal movement.
		// Account for map wrapping: on looping maps, the shortest delta may wrap.
		ddx := req.X - curX
		ddy := req.Y - curY
		if gh.res != nil {
			if pm := gh.res.Passability[s.MapID]; pm != nil {
				if pm.IsLoopH() && pm.Width > 0 {
					if ddx > pm.Width/2 {
						ddx -= pm.Width
					} else if ddx < -pm.Width/2 {
						ddx += pm.Width
					}
				}
				if pm.IsLoopV() && pm.Height > 0 {
					if ddy > pm.Height/2 {
						ddy -= pm.Height
					} else if ddy < -pm.Height/2 {
						ddy += pm.Height
					}
				}
			}
		}
		dx := float64(ddx)
		dy := float64(ddy)
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist > maxMovePerTick {
			gh.logger.Warn("speed hack detected",
				zap.Int64("char_id", s.CharID),
				zap.Float64("dx", dx), zap.Float64("dy", dy),
				zap.Float64("distance", dist))
			if s.RecordSpeedHack() {
				gh.logger.Warn("kicking player for repeated speed hacks",
					zap.Int64("char_id", s.CharID))
				s.Close()
				return nil
			}
			sendMoveReject(s)
			return nil
		}
	}

	// Passability check — ALWAYS enforced (including during grace period).
	// 1. Can leave current tile in the move direction
	// 2. Can enter destination tile from the reverse direction
	// 3. YEP_RegionRestrictions: block player if destination region is restricted
	if gh.res != nil {
		pm := gh.res.Passability[s.MapID]
		if pm != nil {
			// Determine the movement direction from position delta.
			// Account for map wrapping: large positive delta means we wrapped backwards.
			moveDir := req.Dir
			mdx, mdy := req.X-curX, req.Y-curY
			if pm.IsLoopH() && pm.Width > 0 {
				if mdx > pm.Width/2 {
					mdx -= pm.Width
				} else if mdx < -pm.Width/2 {
					mdx += pm.Width
				}
			}
			if pm.IsLoopV() && pm.Height > 0 {
				if mdy > pm.Height/2 {
					mdy -= pm.Height
				} else if mdy < -pm.Height/2 {
					mdy += pm.Height
				}
			}
			if mdx == 1 && mdy == 0 {
				moveDir = 6 // right
			} else if mdx == -1 && mdy == 0 {
				moveDir = 4 // left
			} else if mdx == 0 && mdy == 1 {
				moveDir = 2 // down
			} else if mdx == 0 && mdy == -1 {
				moveDir = 8 // up
			}

			// YEP_RegionRestrictions: check destination region.
			destRegion := pm.RegionAt(req.X, req.Y)
			if rr := gh.res.RegionRestr; rr != nil {
				if rr.IsPlayerRestricted(destRegion) {
					sendMoveReject(s)
					return nil
				}
			}

			// Skip tile passability if region explicitly allows movement.
			regionAllowed := gh.res.RegionRestr != nil && gh.res.RegionRestr.IsPlayerAllowed(destRegion)
			if !regionAllowed {
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
				// Event tiles override static map when present (RMMV allTiles behavior).
				// Use event tile result if decided, otherwise fall back to static pm.
				srcOK, dstOK := true, true
				if room := gh.wm.GetPlayerRoom(s); room != nil {
					if ok, decided := room.CheckEventTileOnly(curX, curY, moveDir); decided {
						srcOK = ok
					} else {
						srcOK = pm.CanPass(curX, curY, moveDir)
					}
					if ok, decided := room.CheckEventTileOnly(req.X, req.Y, revDir); decided {
						dstOK = ok
					} else {
						dstOK = pm.CanPass(req.X, req.Y, revDir)
					}
				} else {
					srcOK = pm.CanPass(curX, curY, moveDir)
					dstOK = pm.CanPass(req.X, req.Y, revDir)
				}
				if !srcOK || !dstOK {
					sendMoveReject(s)
					return nil
				}
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
	room := gh.wm.GetPlayerRoom(s)
	if room != nil {
		room.BroadcastExcept(syncPkt, s.CharID)
	}

	// YEP_EventRegionTrigger: fire events whose region trigger list matches the
	// player's current region. Runs before transfer/touch-event checks so that
	// region-triggered autoruns can set up state before transfers execute.
	if room != nil && !inGrace && gh.touchEventFn != nil && gh.res != nil {
		if md := gh.res.Maps[s.MapID]; md != nil {
			if md.RegionTriggerIndex == nil {
				md.BuildRegionTriggerIndex()
			}
			if pm := gh.res.Passability[s.MapID]; pm != nil {
				playerRegion := pm.RegionAt(req.X, req.Y)
				if eventIDs, ok := md.RegionTriggerIndex[playerRegion]; ok {
					for _, eid := range eventIDs {
						if eid < len(md.Events) && md.Events[eid] != nil {
							gh.touchEventFn(s, s.MapID, md.Events[eid].X, md.Events[eid].Y)
						}
					}
				}
			}
		}
	}

	// Server-side auto-transfer: check if player stepped on a transfer event
	// (trigger 1=Player Touch or 2=Event Touch with command 201=Transfer Player).
	// Since the client no longer processes events (_events = []), the server
	// must detect and execute map transfers.
	// Skip during grace period after map entry to prevent transfer loops
	// (e.g., TE events that transfer to same map creating instant re-trigger).
	if room != nil && !inGrace {
		td := gh.getTransferForPlayer(s, room, req.X, req.Y)
		if td != nil && td.MapID > 0 {
			// Use exact coordinates from the map maker — do NOT adjust with
			// findNearestPassable. The BFS ring search ignores walls and can
			// place the player on the wrong side of a wall.
			// Skip if destination == current position (prevents infinite loop).
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
		} else if gh.touchEventFn != nil {
			// No top-level transfer found — check for touch-trigger events
			// that need full executor execution (e.g., conditional transfers,
			// dialog events with player touch trigger).
			gh.touchEventFn(s, s.MapID, req.X, req.Y)
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

	// 事件执行期间拒绝客户端发起的传送。
	if !s.EventMu.TryLock() {
		gh.logger.Warn("map_transfer rejected: player in event",
			zap.Int64("char_id", s.CharID))
		return nil
	}
	s.EventMu.Unlock()

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
	// 事件执行期间拒绝重置位置。
	if !s.EventMu.TryLock() {
		payload, _ := json.Marshal(map[string]interface{}{
			"error": "Cannot reset position during an event.",
		})
		s.Send(&player.Packet{Type: "reset_pos", Payload: payload})
		return nil
	}
	s.EventMu.Unlock()

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
	s.SetLastTransfer()

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
	if room := gh.wm.GetPlayerRoom(s); room != nil {
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

// ------------------------------------------------------------------ TransferPlayer

// TransferPlayer handles event-initiated player transfers (RMMV command 201).
// For same-map transfers, it repositions without full scene reload.
// For cross-map transfers, it delegates to EnterMapRoom.
func (gh *GameHandlers) TransferPlayer(s *player.PlayerSession, mapID, x, y, dir int) {
	if mapID == s.MapID {
		// Same-map transfer: lightweight reposition without full map reload.
		// This avoids sending map_init (which causes scene reload) and prevents
		// the client from entering a transitional state that blocks effect acks.
		s.SetPosition(x, y, dir)
		s.SetLastTransfer()

		// Notify the client to reposition via reserveTransfer (lightweight for same-map).
		payload, _ := json.Marshal(map[string]interface{}{
			"map_id": mapID,
			"x":      x,
			"y":      y,
			"dir":    dir,
		})
		s.Send(&player.Packet{Type: "transfer_player", Payload: payload})

		// Broadcast position update to other players on the same map.
		s.ResetDirty()
		syncPayload, _ := json.Marshal(map[string]interface{}{
			"char_id": s.CharID,
			"x":       x,
			"y":       y,
			"dir":     dir,
			"hp":      s.HP,
			"mp":      s.MP,
			"state":   "normal",
		})
		syncPkt, _ := json.Marshal(&player.Packet{Type: "player_sync", Payload: syncPayload})
		if room := gh.wm.GetPlayerRoom(s); room != nil {
			room.BroadcastExcept(syncPkt, s.CharID)
		}

		gh.logger.Info("same-map transfer",
			zap.Int64("char_id", s.CharID),
			zap.Int("map_id", mapID),
			zap.Int("x", x),
			zap.Int("y", y))
		return
	}
	gh.EnterMapRoom(s, mapID, x, y, dir)
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

	s.SetMapInfo(mapID, s.InstanceID)
	s.SetPosition(x, y, dir)
	s.SetLastTransfer()

	// Clear NPC channels to prevent stale dialog acks from previous map.
	s.ClearNPCChannels()

	// Check if this map is an instance map (tagged with <instance> in map Note).
	var room *world.MapRoom
	instCfg := gh.getInstanceConfig(mapID)
	if instCfg.Enabled {
		ownerID := s.CharID
		if instCfg.Party && gh.partyMgr != nil {
			if p := gh.partyMgr.GetParty(s.CharID); p != nil {
				ownerID = p.LeaderID
			}
		}
		var instID int64
		room, instID = gh.wm.GetOrCreateInstance(mapID, ownerID)
		s.InstanceID = instID
	} else {
		room = gh.wm.GetOrCreate(mapID)
		s.InstanceID = 0
	}
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
	var equippedItems []map[string]interface{} // slot_index → item info for client
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
				// Build equipped items list for client to sync $gameActors._equips.
				equippedItems = append(equippedItems, map[string]interface{}{
					"slot_index": inv.SlotIndex,
					"item_id":    inv.ItemID,
					"kind":       inv.Kind, // 2=weapon, 3=armor
				})
				// Track on session for server-side script evaluation
				s.SetEquip(inv.SlotIndex, inv.ItemID)
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
	if equippedItems == nil {
		equippedItems = []map[string]interface{}{}
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

	// Include player variables/switches so client-side parallel CEs work correctly.
	var varsSnap map[int]int
	var switchSnap map[int]bool
	if ps, err := gh.wm.PlayerStateManager().GetOrLoad(s.CharID); err == nil {
		// Compute time period from hour variable using MMOConfig.
		// CE 32 normally does this, but it requires autorun events on the map.
		// New/starting maps (e.g. Map 20) may lack autoruns, so we compute inline.
		if gh.res != nil && gh.res.MMOConfig != nil && gh.res.MMOConfig.TimePeriod != nil {
			tp := gh.res.MMOConfig.TimePeriod
			hour := ps.GetVariable(tp.HourVar)
			period := gh.res.MMOConfig.ComputeTimePeriod(hour)
			if period != 0 {
				ps.SetVariable(tp.PeriodVar, period)
			}
		}

		varsSnap = ps.VariablesSnapshot()
		switchSnap = ps.SwitchesSnapshot()
	}
	// Convert to string keys for JSON (JSON requires string keys).
	jsonVars := make(map[string]int, len(varsSnap))
	for k, v := range varsSnap {
		jsonVars[strconv.Itoa(k)] = v
	}
	jsonSwitches := make(map[string]bool, len(switchSnap))
	for k, v := range switchSnap {
		jsonSwitches[strconv.Itoa(k)] = v
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
		"variables":   jsonVars,
		"switches":    jsonSwitches,
		"equips":      equippedItems,
		"init_state":  gh.getInitState(),
		"combat_mode": gh.getCombatMode(),
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

	gen := s.IncrMapGen()

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
			// Stale check: if another EnterMapRoom already ran, this autorun is outdated.
			if s.GetMapGen() != gen {
				gh.logger.Info("autorun skipped: stale map generation",
					zap.Int64("char_id", s.CharID),
					zap.Int("map_id", mapID))
				return
			}
			autorunFn(s, mapID)
		}()
	}

	// Start parallel map events (trigger=4) independently from autoruns.
	if gh.parallelFn != nil {
		parallelFn := gh.parallelFn
		go func() {
			defer func() {
				if r := recover(); r != nil {
					gh.logger.Error("parallel event launcher panic",
						zap.Int64("char_id", s.CharID),
						zap.Int("map_id", mapID),
						zap.Any("panic", r))
				}
			}()
			parallelFn(s, mapID, gen)
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
// If the player was in an instance room, it is destroyed when empty (unless save mode).
func leaveMap(s *player.PlayerSession, wm *world.WorldManager, logger *zap.Logger) {
	room := wm.GetPlayerRoom(s)
	if room == nil {
		return
	}
	room.RemovePlayer(s.CharID)

	leavePayload, _ := json.Marshal(map[string]interface{}{"char_id": s.CharID})
	leavePkt, _ := json.Marshal(&player.Packet{Type: "player_leave", Payload: leavePayload})
	room.Broadcast(leavePkt)

	// Clean up instance room if empty.
	if s.InstanceID > 0 {
		instID := s.InstanceID
		s.InstanceID = 0
		wm.DestroyInstance(instID) // no-op if players remain (party instance)
	}

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

// getInstanceConfig returns the instance configuration for a map.
func (gh *GameHandlers) getInstanceConfig(mapID int) world.InstanceConfig {
	if gh.res == nil {
		return world.InstanceConfig{}
	}
	md, ok := gh.res.Maps[mapID]
	if !ok {
		return world.InstanceConfig{}
	}
	return world.ParseInstanceConfig(md.Note)
}

// EnterInstanceMidEvent switches the player to an instance of the current map
// without interrupting the running event. Called by the executor's EnterInstance
// plugin command. Sends instance_enter to the client with fresh NPC state.
func (gh *GameHandlers) EnterInstanceMidEvent(s *player.PlayerSession) {
	if s.InstanceID > 0 {
		return // already in an instance
	}
	mapID := s.MapID

	// Remove from shared room.
	if room := gh.wm.GetPlayerRoom(s); room != nil {
		room.RemovePlayer(s.CharID)
		// Broadcast player_leave to remaining players.
		leavePayload, _ := json.Marshal(map[string]interface{}{"char_id": s.CharID})
		leavePkt, _ := json.Marshal(&player.Packet{Type: "player_leave", Payload: leavePayload})
		room.Broadcast(leavePkt)
	}

	// Determine instance owner.
	ownerID := s.CharID
	if gh.partyMgr != nil {
		if p := gh.partyMgr.GetParty(s.CharID); p != nil {
			ownerID = p.LeaderID
		}
	}

	// Create/join instance room.
	instRoom, instID := gh.wm.GetOrCreateInstance(mapID, ownerID)
	s.InstanceID = instID
	instRoom.AddPlayer(s)

	// Send NPC snapshot from the clean instance room to the client.
	npcSnapshot := gh.npcSnapshotForPlayer(s, instRoom)
	payload, _ := json.Marshal(map[string]interface{}{
		"npcs": npcSnapshot,
	})
	s.Send(&player.Packet{Type: "instance_enter", Payload: payload})

	gh.logger.Info("player entered instance mid-event",
		zap.Int64("char_id", s.CharID),
		zap.Int("map_id", mapID),
		zap.Int64("instance_id", instID))
}

// LeaveInstanceMidEvent switches the player back to the shared map room
// without interrupting the running event. Called by the executor's LeaveInstance
// plugin command. Sends instance_leave to the client with shared NPC/player state.
func (gh *GameHandlers) LeaveInstanceMidEvent(s *player.PlayerSession) {
	if s.InstanceID == 0 {
		return // not in an instance
	}
	instID := s.InstanceID
	mapID := s.MapID

	// Remove from instance room.
	if instRoom := gh.wm.GetInstance(instID); instRoom != nil {
		instRoom.RemovePlayer(s.CharID)
	}

	// Clear instance state.
	s.InstanceID = 0

	// Destroy instance if empty.
	gh.wm.DestroyInstance(instID)

	// Join shared room.
	sharedRoom := gh.wm.GetOrCreate(mapID)
	sharedRoom.AddPlayer(s)

	// Send updated NPC + player state from shared room to client.
	npcSnapshot := gh.npcSnapshotForPlayer(s, sharedRoom)
	payload, _ := json.Marshal(map[string]interface{}{
		"npcs":    npcSnapshot,
		"players": sharedRoom.PlayerSnapshot(),
	})
	s.Send(&player.Packet{Type: "instance_leave", Payload: payload})

	// Broadcast player_join to other players in the shared room.
	x, y, dir := s.Position()
	joinPayload, _ := json.Marshal(map[string]interface{}{
		"char_id":    s.CharID,
		"name":       s.CharName,
		"walk_name":  s.WalkName,
		"walk_index": s.WalkIndex,
		"x":          x,
		"y":          y,
		"dir":        dir,
		"hp":         s.HP,
		"max_hp":     s.MaxHP,
		"mp":         s.MP,
		"max_mp":     s.MaxMP,
		"buffs":      []interface{}{},
	})
	joinPkt, _ := json.Marshal(&player.Packet{Type: "player_join", Payload: joinPayload})
	sharedRoom.BroadcastExcept(joinPkt, s.CharID)

	gh.logger.Info("player left instance mid-event",
		zap.Int64("char_id", s.CharID),
		zap.Int("map_id", mapID),
		zap.Int64("instance_id", instID))
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

// ------------------------------------------------------------------ client_log

type clientLogEntry struct {
	T int64  `json:"t"` // timestamp ms
	L string `json:"l"` // level: INFO, WARN, ERROR
	M string `json:"m"` // message
}

type clientLogPayload struct {
	Entries []clientLogEntry `json:"entries"`
}

// HandleClientLog receives batched console log entries from the client debug plugin.
func (gh *GameHandlers) HandleClientLog(_ context.Context, s *player.PlayerSession, raw json.RawMessage) error {
	var p clientLogPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil // ignore malformed
	}
	for _, e := range p.Entries {
		switch e.L {
		case "ERROR":
			gh.logger.Error("[CLIENT] "+e.M, zap.Int64("char_id", s.CharID))
		case "WARN":
			gh.logger.Warn("[CLIENT] "+e.M, zap.Int64("char_id", s.CharID))
		default:
			gh.logger.Info("[CLIENT] "+e.M, zap.Int64("char_id", s.CharID))
		}
	}
	return nil
}
