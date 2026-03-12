package ai

// ThreatTable tracks damage-based aggro for one monster.
type ThreatTable struct {
	entries map[int64]int // charID → cumulative threat
}

// NewThreatTable creates an empty threat table.
func NewThreatTable() *ThreatTable {
	return &ThreatTable{entries: make(map[int64]int)}
}

// AddThreat accumulates threat from damage dealt.
func (tt *ThreatTable) AddThreat(charID int64, amount int) {
	if amount <= 0 {
		return
	}
	tt.entries[charID] += amount
}

// TopThreat returns the charID with the highest threat, or 0 if empty.
func (tt *ThreatTable) TopThreat() int64 {
	var topID int64
	topVal := 0
	for id, v := range tt.entries {
		if v > topVal {
			topVal = v
			topID = id
		}
	}
	return topID
}

// Remove removes a player from the table (e.g. disconnected or left room).
func (tt *ThreatTable) Remove(charID int64) {
	delete(tt.entries, charID)
}

// Decay reduces all threat values by a percentage (0-100).
func (tt *ThreatTable) Decay(percent int) {
	if percent <= 0 {
		return
	}
	for id, v := range tt.entries {
		reduction := v * percent / 100
		if reduction < 1 {
			reduction = 1
		}
		v -= reduction
		if v <= 0 {
			delete(tt.entries, id)
		} else {
			tt.entries[id] = v
		}
	}
}

// Clear empties the threat table (e.g. monster returned to spawn).
func (tt *ThreatTable) Clear() {
	for k := range tt.entries {
		delete(tt.entries, k)
	}
}

// Len returns the number of entries in the table.
func (tt *ThreatTable) Len() int {
	return len(tt.entries)
}
