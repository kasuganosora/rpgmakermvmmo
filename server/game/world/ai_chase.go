package world

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// ChaseState tracks the current pursuit state of a chasing NPC.
type ChaseState int

const (
	ChaseStateIdle  ChaseState = iota // no player nearby
	ChaseStateAlert                   // player spotted, alert balloon shown
	ChaseStateChase                   // actively pursuing player
	ChaseStateReturn                  // returning to home position
)

// ChaseAI holds the runtime state for YEP_EventChasePlayer behavior.
// Embedded in NPCRuntime for NPCs with <Chase Range: N> or <Flee Range: N> tags.
type ChaseAI struct {
	// Configuration (parsed from Note tags at init time).
	ChaseRange int  // tiles: trigger chase when player enters this range
	FleeRange  int  // tiles: trigger flee when player enters this range (flee mode)
	FleeMode   bool // true = flee away, false = chase toward

	// Alert balloon ID (default 1 = exclamation mark, RMMV balloon IDs).
	AlertBalloon int
	// Alert common event ID to fire when player is first spotted (0 = none).
	AlertCommonEvent int

	// State machine.
	State      ChaseState
	AlertTimer int // ticks until state transitions from Alert → Chase
	ReturnWait int // ticks to wait after reaching home before going Idle

	// Home position (where the NPC started, for Return behavior).
	HomeX, HomeY int

	// Sight lock: once player is spotted, chase for this many ticks even
	// if player leaves line-of-sight. 0 = no lock.
	SightLock      int
	sightLockTimer int

	// Target tracking.
	lastTargetX, lastTargetY int
}

// chaseAlertTicks is the default number of ticks between entering Alert and starting Chase.
// At 20 TPS: 60 ticks = 3 seconds (mirrors YEP default Alert Timer of 120 frames / 2 = 60 ticks).
const chaseAlertTicks = 60

// chaseReturnWaitTicks is the default number of ticks to wait at home before going Idle.
const chaseReturnWaitTicks = 60

// chaseDefaultSightLock is the default sight-lock duration in ticks (YEP default 300 frames / 2 = 150 ticks).
const chaseDefaultSightLock = 150

// ParseChaseAI reads YEP_EventChasePlayer Note tags from a map event and returns
// a configured ChaseAI if the event uses chase/flee behavior.
// Returns nil if the event has no chase configuration.
//
// Supported Note tags (mirrors YEP_EventChasePlayer script-call interface):
//
//	<Chase Range: N>        — chase player when within N tiles
//	<Flee Range: N>         — flee from player when within N tiles
//	<Alert Balloon: N>      — balloon ID to show on alert (default 1)
//	<Alert CE: N>           — common event to fire on alert (default 0)
//	<Sight Lock: N>         — ticks to maintain chase after LoS lost (default 150)
func ParseChaseAI(ev *resource.MapEvent) *ChaseAI {
	if ev == nil {
		return nil
	}
	meta := resource.ParseMetaGo(ev.Note)
	if meta == nil {
		return nil
	}

	chaseRange := metaIntMeta(meta, "Chase Range")
	fleeRange := metaIntMeta(meta, "Flee Range")
	if chaseRange <= 0 && fleeRange <= 0 {
		return nil // event doesn't use chase/flee
	}

	ai := &ChaseAI{
		AlertBalloon:     1,
		AlertCommonEvent: 0,
		SightLock:        chaseDefaultSightLock,
	}

	if chaseRange > 0 {
		ai.ChaseRange = chaseRange
		ai.FleeMode = false
	} else {
		ai.FleeRange = fleeRange
		ai.FleeMode = true
	}
	if v := metaIntMeta(meta, "Alert Balloon"); v > 0 {
		ai.AlertBalloon = v
	}
	if v := metaIntMeta(meta, "Alert CE"); v > 0 {
		ai.AlertCommonEvent = v
	}
	if v := metaIntMeta(meta, "Sight Lock"); v >= 0 {
		ai.SightLock = v
	}

	return ai
}

// metaIntMeta reads an integer value from a ParsedMeta map.
func metaIntMeta(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return -1
	}
	switch val := v.(type) {
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return -1
		}
		return n
	case float64:
		return int(val)
	case int:
		return val
	}
	return -1
}

// tickChaseAI runs one movement tick for NPCs that have ChaseAI.
// Called inside room.tickNPCs() after the standard movement logic.
// The room.mu lock must be held by the caller.
func (room *MapRoom) tickChaseAI(npc *NPCRuntime) {
	ai := npc.ChaseAI
	if ai == nil {
		return
	}

	// Find nearest player and their distance.
	nearest, dist := room.nearestPlayer(npc.X, npc.Y)

	triggerRange := ai.ChaseRange
	if ai.FleeMode {
		triggerRange = ai.FleeRange
	}

	switch ai.State {
	case ChaseStateIdle:
		if nearest != nil && dist <= triggerRange {
			// Transition to Alert.
			ai.State = ChaseStateAlert
			ai.AlertTimer = chaseAlertTicks
			ai.sightLockTimer = ai.SightLock
			px, py, _ := nearest.Position()
			ai.lastTargetX, ai.lastTargetY = px, py
			// Emit alert balloon via npc_effect.
			room.broadcastNPCBalloon(npc.EventID, ai.AlertBalloon)
		}

	case ChaseStateAlert:
		ai.AlertTimer--
		if ai.AlertTimer <= 0 {
			ai.State = ChaseStateChase
		}

	case ChaseStateChase:
		if nearest == nil || (dist > triggerRange && ai.sightLockTimer <= 0) {
			// Player left range and sight lock expired → return home.
			ai.State = ChaseStateReturn
			ai.ReturnWait = chaseReturnWaitTicks
			return
		}
		ai.sightLockTimer--
		if nearest != nil {
			px, py, _ := nearest.Position()
			ai.lastTargetX, ai.lastTargetY = px, py
		}
		if ai.FleeMode {
			room.moveNPCFlee(npc, ai.lastTargetX, ai.lastTargetY)
		} else {
			room.moveNPCToward(npc, ai.lastTargetX, ai.lastTargetY)
		}

	case ChaseStateReturn:
		if npc.X == ai.HomeX && npc.Y == ai.HomeY {
			ai.ReturnWait--
			if ai.ReturnWait <= 0 {
				ai.State = ChaseStateIdle
			}
			return
		}
		room.moveNPCToward(npc, ai.HomeX, ai.HomeY)
	}
}

// nearestPlayer finds the player closest (Manhattan distance) to (x, y).
// Returns nil, 0 if no players are in the room.
func (room *MapRoom) nearestPlayer(x, y int) (*player.PlayerSession, int) {
	var nearest *player.PlayerSession
	minDist := 999999
	for _, p := range room.players {
		px, py, _ := p.Position()
		dist := abs(px-x) + abs(py-y)
		if dist < minDist {
			minDist = dist
			nearest = p
		}
	}
	return nearest, minDist
}

// moveNPCToward moves an NPC one tile toward the target (x, y).
func (room *MapRoom) moveNPCToward(npc *NPCRuntime, targetX, targetY int) {
	dx := targetX - npc.X
	dy := targetY - npc.Y
	if dx == 0 && dy == 0 {
		return
	}
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

// moveNPCFlee moves an NPC one tile away from the target (x, y).
func (room *MapRoom) moveNPCFlee(npc *NPCRuntime, targetX, targetY int) {
	dx := targetX - npc.X
	dy := targetY - npc.Y
	// Move in the opposite direction of the target.
	if abs(dx) >= abs(dy) {
		if dx > 0 {
			if !room.tryMoveNPC(npc, dirLeft) && dy != 0 {
				if dy > 0 {
					room.tryMoveNPC(npc, dirUp)
				} else {
					room.tryMoveNPC(npc, dirDown)
				}
			}
		} else {
			if !room.tryMoveNPC(npc, dirRight) && dy != 0 {
				if dy > 0 {
					room.tryMoveNPC(npc, dirUp)
				} else {
					room.tryMoveNPC(npc, dirDown)
				}
			}
		}
	} else {
		if dy > 0 {
			if !room.tryMoveNPC(npc, dirUp) && dx != 0 {
				if dx > 0 {
					room.tryMoveNPC(npc, dirLeft)
				} else {
					room.tryMoveNPC(npc, dirRight)
				}
			}
		} else {
			if !room.tryMoveNPC(npc, dirDown) && dx != 0 {
				if dx > 0 {
					room.tryMoveNPC(npc, dirLeft)
				} else {
					room.tryMoveNPC(npc, dirRight)
				}
			}
		}
	}
}

// broadcastNPCBalloon sends an npc_effect message to all players in the room
// to display a balloon animation above the NPC (code 213).
// Uses the existing npc_effect protocol: {code:213, params:[event_id, balloon_id, false]}.
func (room *MapRoom) broadcastNPCBalloon(eventID, balloonID int) {
	payload, _ := json.Marshal(map[string]interface{}{
		"code":   213,
		"params": []interface{}{eventID, balloonID, false},
	})
	pkt, _ := json.Marshal(&player.Packet{Type: "npc_effect", Payload: payload})
	room.broadcastRaw(pkt)
}
