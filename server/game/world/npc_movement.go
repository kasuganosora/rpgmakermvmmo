package world

import (
	"encoding/json"
	"math/rand"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
)

// RMMV MoveFrequency → tick interval mapping.
// MoveFrequency 1 (lowest) to 5 (highest).
// At 20 TPS: frequency 3 = ~60 ticks = 3 seconds between moves.
var moveFreqTicks = [6]int{
	0,   // 0: unused
	120, // 1: lowest — 6 seconds
	90,  // 2: lower — 4.5 seconds
	60,  // 3: normal — 3 seconds
	30,  // 4: higher — 1.5 seconds
	10,  // 5: highest — 0.5 seconds
}

// RMMV directions
const (
	dirDown  = 2
	dirLeft  = 4
	dirRight = 6
	dirUp    = 8
)

var directions = [4]int{dirDown, dirLeft, dirRight, dirUp}

// dx/dy for each direction.
func dirDelta(dir int) (dx, dy int) {
	switch dir {
	case dirDown:
		return 0, 1
	case dirLeft:
		return -1, 0
	case dirRight:
		return 1, 0
	case dirUp:
		return 0, -1
	}
	return 0, 0
}

// tickNPCs runs one movement tick for all NPCs and broadcasts dirty positions.
func (room *MapRoom) tickNPCs() {
	room.mu.Lock()
	for _, npc := range room.npcs {
		if npc.ActivePage == nil {
			continue
		}
		moveType := npc.ActivePage.MoveType
		if moveType == 0 {
			continue // fixed — no movement
		}

		npc.moveTimer--
		if npc.moveTimer > 0 {
			continue
		}

		// Reset timer based on move frequency.
		freq := npc.ActivePage.MoveFrequency
		if freq < 1 {
			freq = 3
		}
		if freq > 5 {
			freq = 5
		}
		baseTicks := moveFreqTicks[freq]
		// Add some randomness (±25%) to prevent synchronized movement.
		jitter := baseTicks / 4
		npc.moveTimer = baseTicks - jitter + rand.Intn(jitter*2+1)

		switch moveType {
		case 1: // random
			room.moveNPCRandom(npc)
		case 2: // approach nearest player
			room.moveNPCApproach(npc)
		case 3: // custom route
			room.moveNPCCustomRoute(npc)
		}
	}

	// Collect dirty NPCs and broadcast.
	var dirtyNPCs []*NPCRuntime
	for _, npc := range room.npcs {
		if npc.dirty {
			dirtyNPCs = append(dirtyNPCs, npc)
			npc.dirty = false
		}
	}
	room.mu.Unlock()

	// Broadcast outside the lock.
	for _, npc := range dirtyNPCs {
		payload, _ := json.Marshal(map[string]interface{}{
			"event_id": npc.EventID,
			"x":        npc.X,
			"y":        npc.Y,
			"dir":      npc.Dir,
		})
		pkt, _ := json.Marshal(&player.Packet{Type: "npc_sync", Payload: payload})
		room.broadcastRaw(pkt)
	}
}

// moveNPCRandom moves an NPC in a random passable direction.
func (room *MapRoom) moveNPCRandom(npc *NPCRuntime) {
	dir := directions[rand.Intn(4)]
	room.tryMoveNPC(npc, dir)
}

// moveNPCApproach moves an NPC toward the nearest player.
func (room *MapRoom) moveNPCApproach(npc *NPCRuntime) {
	var nearest *player.PlayerSession
	minDist := 999999
	for _, p := range room.players {
		px, py, _ := p.Position()
		dist := abs(px-npc.X) + abs(py-npc.Y)
		if dist < minDist {
			minDist = dist
			nearest = p
		}
	}
	if nearest == nil {
		return
	}

	px, py, _ := nearest.Position()
	dx := px - npc.X
	dy := py - npc.Y

	// Pick the axis with greater distance, prefer horizontal if equal.
	if abs(dx) >= abs(dy) {
		if dx > 0 {
			if !room.tryMoveNPC(npc, dirRight) && dy != 0 {
				if dy > 0 {
					room.tryMoveNPC(npc, dirDown)
				} else {
					room.tryMoveNPC(npc, dirUp)
				}
			}
		} else {
			if !room.tryMoveNPC(npc, dirLeft) && dy != 0 {
				if dy > 0 {
					room.tryMoveNPC(npc, dirDown)
				} else {
					room.tryMoveNPC(npc, dirUp)
				}
			}
		}
	} else {
		if dy > 0 {
			if !room.tryMoveNPC(npc, dirDown) && dx != 0 {
				if dx > 0 {
					room.tryMoveNPC(npc, dirRight)
				} else {
					room.tryMoveNPC(npc, dirLeft)
				}
			}
		} else {
			if !room.tryMoveNPC(npc, dirUp) && dx != 0 {
				if dx > 0 {
					room.tryMoveNPC(npc, dirRight)
				} else {
					room.tryMoveNPC(npc, dirLeft)
				}
			}
		}
	}
}

// RMMV move route command codes.
const (
	moveRouteEnd       = 0
	moveRouteDown      = 1
	moveRouteLeft      = 2
	moveRouteRight     = 3
	moveRouteUp        = 4
	moveRouteTurnDown  = 35
	moveRouteTurnLeft  = 36
	moveRouteTurnRight = 37
	moveRouteTurnUp    = 38
)

// moveNPCCustomRoute executes the next command in a custom move route.
func (room *MapRoom) moveNPCCustomRoute(npc *NPCRuntime) {
	if npc.ActivePage == nil || npc.ActivePage.MoveRoute == nil {
		return
	}
	route := npc.ActivePage.MoveRoute
	if len(route.List) == 0 {
		return
	}

	cmd := route.List[npc.routeIdx]
	if cmd == nil {
		npc.routeIdx++
		if npc.routeIdx >= len(route.List) {
			if route.Repeat {
				npc.routeIdx = 0
			} else {
				npc.routeIdx = len(route.List) - 1
			}
		}
		return
	}

	switch cmd.Code {
	case moveRouteEnd:
		if route.Repeat {
			npc.routeIdx = 0
		}
		return
	case moveRouteDown:
		room.tryMoveNPC(npc, dirDown)
	case moveRouteLeft:
		room.tryMoveNPC(npc, dirLeft)
	case moveRouteRight:
		room.tryMoveNPC(npc, dirRight)
	case moveRouteUp:
		room.tryMoveNPC(npc, dirUp)
	case moveRouteTurnDown:
		npc.Dir = dirDown
		npc.dirty = true
	case moveRouteTurnLeft:
		npc.Dir = dirLeft
		npc.dirty = true
	case moveRouteTurnRight:
		npc.Dir = dirRight
		npc.dirty = true
	case moveRouteTurnUp:
		npc.Dir = dirUp
		npc.dirty = true
	}

	npc.routeIdx++
	if npc.routeIdx >= len(route.List) {
		if route.Repeat {
			npc.routeIdx = 0
		} else {
			npc.routeIdx = len(route.List) - 1
		}
	}
}

// reverseDir returns the opposite RMMV direction.
func reverseDir(dir int) int {
	switch dir {
	case dirDown:
		return dirUp
	case dirUp:
		return dirDown
	case dirLeft:
		return dirRight
	case dirRight:
		return dirLeft
	}
	return dir
}

// tryMoveNPC attempts to move an NPC one tile in the given direction.
// Returns true if the move succeeded.
//
// Checks (in order, matching RMMV + YEP_RegionRestrictions):
//  1. Bounds check (always enforced, even for Through NPCs)
//  2. Through flag — if set, skip ALL passability/region/collision checks (RMMV behavior)
//  3. Region restrictions (destination tile) — forbid overrides everything
//  4. Region allow (destination tile) — allow skips tile passability check
//  5. Tile passability (source AND destination in reverse direction)
//  6. NPC-NPC collision
func (room *MapRoom) tryMoveNPC(npc *NPCRuntime, dir int) bool {
	dx, dy := dirDelta(dir)
	nx, ny := npc.X+dx, npc.Y+dy

	// 1. Bounds check — always enforced to prevent out-of-map movement.
	if room.passMap != nil {
		if nx < 0 || nx >= room.passMap.Width || ny < 0 || ny >= room.passMap.Height {
			return false
		}
	} else {
		w, h := room.mapWidth, room.mapHeight
		if w <= 0 || h <= 0 {
			return false
		}
		if nx < 0 || nx >= w || ny < 0 || ny >= h {
			return false
		}
	}

	// 2. Through flag — in RMMV, characters with Through=true ignore all
	// collision (walls, events, characters). Skip all remaining checks.
	isThrough := npc.ActivePage != nil && npc.ActivePage.Through
	if isThrough {
		npc.X = nx
		npc.Y = ny
		npc.Dir = dir
		npc.dirty = true
		return true
	}

	// 3-5. Region and tile passability checks.
	regionAllowed := false
	if room.passMap != nil {
		// Check YEP_RegionRestrictions on the DESTINATION tile.
		if room.res != nil && room.res.RegionRestr != nil {
			rr := room.res.RegionRestr
			destRegion := room.passMap.RegionAt(nx, ny)
			if rr.IsEventRestricted(destRegion) {
				return false // region forbids event movement
			}
			if rr.IsEventAllowed(destRegion) {
				regionAllowed = true // skip tile passability
			}
		}

		if !regionAllowed {
			// Source tile: can we leave in this direction?
			if !room.passMap.CanPass(npc.X, npc.Y, dir) {
				return false
			}
			// Destination tile: can we enter from the reverse direction?
			if !room.passMap.CanPass(nx, ny, reverseDir(dir)) {
				return false
			}
		}
	}

	// 6. Check collision with other NPCs (same priority).
	if npc.ActivePage != nil && npc.ActivePage.PriorityType == 1 {
		for _, other := range room.npcs {
			if other == npc || other.ActivePage == nil {
				continue
			}
			if other.ActivePage.PriorityType == 1 && !other.ActivePage.Through {
				if other.X == nx && other.Y == ny {
					return false
				}
			}
		}
	}

	npc.X = nx
	npc.Y = ny
	npc.Dir = dir
	npc.dirty = true
	return true
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
