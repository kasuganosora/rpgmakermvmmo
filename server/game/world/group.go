package world

// MonsterGroup represents a group of related monsters that assist each other.
type MonsterGroup struct {
	GroupID   string
	GroupType string                  // "assist" | "linked" | "pack"
	Members   map[int]*MonsterRuntime // spawnIndex → monster
	LeaderID  int                     // pack mode: spawnIndex of the leader (-1 = none)
}

// GroupManager tracks monster groups within a MapRoom.
type GroupManager struct {
	groups  map[string]*MonsterGroup // groupID → group
	reverse map[int]string           // spawnIndex → groupID (fast reverse lookup)
}

// NewGroupManager creates an empty GroupManager.
func NewGroupManager() *GroupManager {
	return &GroupManager{
		groups:  make(map[string]*MonsterGroup),
		reverse: make(map[int]string),
	}
}

// Register adds a monster to a group. If the group doesn't exist, it is created.
// The first monster registered to a pack group becomes the leader.
func (gm *GroupManager) Register(spawnIndex int, groupID, groupType string, m *MonsterRuntime) {
	if groupID == "" {
		return
	}
	if groupType == "" {
		groupType = "assist"
	}
	group, ok := gm.groups[groupID]
	if !ok {
		group = &MonsterGroup{
			GroupID:   groupID,
			GroupType: groupType,
			Members:   make(map[int]*MonsterRuntime),
			LeaderID:  -1,
		}
		gm.groups[groupID] = group
	}
	group.Members[spawnIndex] = m
	gm.reverse[spawnIndex] = groupID
	// First member of a pack becomes the leader.
	if group.GroupType == "pack" && group.LeaderID == -1 {
		group.LeaderID = spawnIndex
	}
}

// Unregister removes a monster from its group (e.g. on death or despawn).
// Handles pack leader succession and empty group cleanup.
func (gm *GroupManager) Unregister(spawnIndex int) {
	gid, ok := gm.reverse[spawnIndex]
	if !ok {
		return
	}
	group := gm.groups[gid]
	if group == nil {
		delete(gm.reverse, spawnIndex)
		return
	}
	delete(group.Members, spawnIndex)
	delete(gm.reverse, spawnIndex)

	// Pack leader succession: pick the member with highest HP.
	if group.GroupType == "pack" && group.LeaderID == spawnIndex {
		group.LeaderID = -1
		bestHP := 0
		for idx, mem := range group.Members {
			if mem.HP > bestHP {
				bestHP = mem.HP
				group.LeaderID = idx
			}
		}
	}

	// Clean up empty groups.
	if len(group.Members) == 0 {
		delete(gm.groups, gid)
	}
}

// OnMemberDamaged triggers group assist behavior when a group member takes damage.
func (gm *GroupManager) OnMemberDamaged(spawnIndex int, attackerCharID int64) {
	gid, ok := gm.reverse[spawnIndex]
	if !ok {
		return
	}
	group := gm.groups[gid]
	if group == nil {
		return
	}

	switch group.GroupType {
	case "linked":
		// All group members immediately gain threat on the attacker.
		for _, m := range group.Members {
			if m.HP > 0 && m.Threat != nil && m.Threat.Len() == 0 {
				m.Threat.AddThreat(attackerCharID, 1)
			}
		}
	case "pack":
		// Only the leader gains threat; followers will follow via FollowLeaderTarget BT node.
		if leader, ok := group.Members[group.LeaderID]; ok && leader.HP > 0 {
			if leader.Threat != nil && leader.Threat.Len() == 0 {
				leader.Threat.AddThreat(attackerCharID, 1)
			}
		}
	default: // "assist"
		// Range-based: only same-group monsters within assistRange gain threat.
		victim := group.Members[spawnIndex]
		if victim == nil {
			return
		}
		assistRange := 5 // default
		if victim.SpawnCfg != nil && victim.SpawnCfg.AssistRange > 0 {
			assistRange = victim.SpawnCfg.AssistRange
		}
		vx, vy := victim.Position()
		for idx, m := range group.Members {
			if idx == spawnIndex {
				continue // skip the victim itself
			}
			if m.HP > 0 && m.Threat != nil && m.Threat.Len() == 0 {
				mx, my := m.Position()
				dx := mx - vx
				if dx < 0 {
					dx = -dx
				}
				dy := my - vy
				if dy < 0 {
					dy = -dy
				}
				if dx+dy <= assistRange {
					m.Threat.AddThreat(attackerCharID, 1)
				}
			}
		}
	}
}

// GetGroup returns the MonsterGroup for a groupID, or nil.
func (gm *GroupManager) GetGroup(groupID string) *MonsterGroup {
	return gm.groups[groupID]
}

// ClearGroupThreats clears all threat tables for members of the given group.
// Used for linked-mode group leash: when one returns to spawn, all disengage.
func (gm *GroupManager) ClearGroupThreats(groupID string) {
	group := gm.groups[groupID]
	if group == nil {
		return
	}
	for _, m := range group.Members {
		if m.Threat != nil {
			m.Threat.Clear()
		}
		m.SetTarget(0)
	}
}
