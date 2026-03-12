package world

import "github.com/kasuganosora/rpgmakermvmmo/server/resource"

// checkEventTilePassage checks event tile passability at (x, y) for the given direction.
// In RMMV, allTiles() returns tile events (events with tileId > 0 && priorityType == 0)
// before layer tiles. If an event tile is a non-star tile, it determines passability,
// overriding the static passability map.
//
// Returns:
//   - passable: whether movement is allowed (only meaningful when decided == true)
//   - decided: true if an event tile determined passability (caller should use passable);
//     false means no event tiles affected the result and the caller should fall through
//     to the static passability map.
//
// Must be called while room.mu is held (at least RLock).
func (room *MapRoom) checkEventTilePassage(x, y, dir int) (passable, decided bool) {
	flags := room.getTilesetFlags()
	if flags == nil {
		return false, false
	}

	cpStarFix := room.res != nil && room.res.CPStarPassFix

	// RMMV direction bit for checkPassage: (1 << (d/2 - 1)) & 0x0f
	bit := dirToBit(dir)
	if bit == 0 {
		return false, false
	}

	// Check all NPCs at position (x, y) that are tile events.
	// RMMV isTile(): this._tileId > 0 && this._priorityType === 0
	for _, npc := range room.npcs {
		if npc.ActivePage == nil {
			continue
		}
		if npc.X != x || npc.Y != y {
			continue
		}
		tileID := npc.ActivePage.Image.TileID
		if tileID <= 0 || npc.ActivePage.PriorityType != 0 {
			continue
		}
		// This NPC is a tile event at (x, y). Check its tileset flags.
		if tileID >= len(flags) {
			continue
		}
		flag := flags[tileID]
		if flag&0x10 != 0 {
			// Star tile.
			if cpStarFix {
				if (flag & bit) == 0 {
					continue // passable star with CP fix → check next
				}
				return false, true // blocked star with CP fix
			}
			continue // base RMMV: star tiles skipped
		}
		// Non-star tile: determines passability.
		if (flag & bit) == 0 {
			return true, true // passable
		}
		return false, true // impassable
	}

	return false, false
}

// getTilesetFlags returns the tileset flags for the current map room's tileset, or nil.
func (room *MapRoom) getTilesetFlags() []int {
	if room.res == nil {
		return nil
	}
	md, ok := room.res.Maps[room.MapID]
	if !ok {
		return nil
	}
	for _, ts := range room.res.Tilesets {
		if ts != nil && ts.ID == md.TilesetID {
			return ts.Flags
		}
	}
	return nil
}

// tilesetForMapData returns the Tileset struct for a map, or nil.
func (room *MapRoom) tilesetForMapData(md *resource.MapData) *resource.Tileset {
	if room.res == nil || md == nil {
		return nil
	}
	for _, ts := range room.res.Tilesets {
		if ts != nil && ts.ID == md.TilesetID {
			return ts
		}
	}
	return nil
}

// dirToBit converts an RMMV direction (2/4/6/8) to its passability bit.
// Matches RMMV: (1 << (d/2 - 1)) & 0x0f
func dirToBit(dir int) int {
	switch dir {
	case 2:
		return 0x01
	case 4:
		return 0x02
	case 6:
		return 0x04
	case 8:
		return 0x08
	}
	return 0
}

// CheckPassage checks passability at (x, y) for the given direction,
// combining event tile passability with the static passability map.
// This matches RMMV's checkPassage(x, y, bit) with allTiles() which includes
// tile events before layer tiles.
//
// Must be called while room.mu is held (at least RLock).
func (room *MapRoom) CheckPassage(x, y, dir int) bool {
	// First check event tiles (highest priority in RMMV's allTiles).
	if passable, decided := room.checkEventTilePassage(x, y, dir); decided {
		return passable
	}
	// Fall through to static passability map.
	if room.passMap != nil {
		return room.passMap.CanPass(x, y, dir)
	}
	return true // no passability data → assume passable
}

// IsPassable checks passability with event tiles, acquiring the read lock internally.
// Safe to call from external packages (e.g., ws handlers).
func (room *MapRoom) IsPassable(x, y, dir int) bool {
	room.mu.RLock()
	defer room.mu.RUnlock()
	return room.CheckPassage(x, y, dir)
}

// CheckEventTileOnly checks only event tile passability at (x, y) for the given direction.
// Returns (passable, decided). If decided is true, an event tile determined passability.
// If decided is false, no event tile was relevant and the caller should use static passability.
// Acquires the read lock internally. Safe to call from external packages.
func (room *MapRoom) CheckEventTileOnly(x, y, dir int) (passable, decided bool) {
	room.mu.RLock()
	defer room.mu.RUnlock()
	return room.checkEventTilePassage(x, y, dir)
}
