package model

// GameSwitch stores a global RMMV switch (ON/OFF) state.
type GameSwitch struct {
	SwitchID int  `gorm:"primaryKey" json:"switch_id"`
	Value    bool `json:"value"`
}

func (GameSwitch) TableName() string { return "game_switches" }

// GameVariable stores a global RMMV variable (integer) state.
type GameVariable struct {
	VariableID int `gorm:"primaryKey" json:"variable_id"`
	Value      int `json:"value"`
}

func (GameVariable) TableName() string { return "game_variables" }

// GameSelfSwitch stores a per-event self-switch state.
type GameSelfSwitch struct {
	MapID   int    `gorm:"primaryKey" json:"map_id"`
	EventID int    `gorm:"primaryKey" json:"event_id"`
	Ch      string `gorm:"primaryKey;size:1" json:"ch"` // "A","B","C","D"
	Value   bool   `json:"value"`
}

func (GameSelfSwitch) TableName() string { return "game_self_switches" }

// ---- Per-character state (keyed by CharID) ----

// CharSwitch stores a per-character RMMV switch state.
type CharSwitch struct {
	CharID   int64 `gorm:"primaryKey" json:"char_id"`
	SwitchID int   `gorm:"primaryKey" json:"switch_id"`
	Value    bool  `json:"value"`
}

func (CharSwitch) TableName() string { return "char_switches" }

// CharVariable stores a per-character RMMV variable state.
type CharVariable struct {
	CharID     int64 `gorm:"primaryKey" json:"char_id"`
	VariableID int   `gorm:"primaryKey" json:"variable_id"`
	Value      int   `json:"value"`
}

func (CharVariable) TableName() string { return "char_variables" }

// CharSelfSwitch stores a per-character self-switch state.
type CharSelfSwitch struct {
	CharID  int64  `gorm:"primaryKey" json:"char_id"`
	MapID   int    `gorm:"primaryKey" json:"map_id"`
	EventID int    `gorm:"primaryKey" json:"event_id"`
	Ch      string `gorm:"primaryKey;size:1" json:"ch"`
	Value   bool   `json:"value"`
}

func (CharSelfSwitch) TableName() string { return "char_self_switches" }

// ========================================================================
// Self Variable Support (TemplateEvent.js extension)
// TemplateEvent.js uses self-variables with numeric indices (13-17 for RandomPos)
// ========================================================================

// GameSelfVariable stores a global self-variable state (TemplateEvent.js extension).
// Key format: [map_id, event_id, index] where index can be any integer (not just A/B/C/D).
type GameSelfVariable struct {
	MapID   int `gorm:"primaryKey" json:"map_id"`
	EventID int `gorm:"primaryKey" json:"event_id"`
	Index   int `gorm:"primaryKey" json:"index"` // e.g., 13=X, 14=Y, 15=Dir, 16=Day, 17=Seed
	Value   int `json:"value"`
}

func (GameSelfVariable) TableName() string { return "game_self_variables" }

// CharSelfVariable stores a per-character self-variable state.
type CharSelfVariable struct {
	CharID  int64 `gorm:"primaryKey" json:"char_id"`
	MapID   int   `gorm:"primaryKey" json:"map_id"`
	EventID int   `gorm:"primaryKey" json:"event_id"`
	Index   int   `gorm:"primaryKey" json:"index"`
	Value   int   `json:"value"`
}

func (CharSelfVariable) TableName() string { return "char_self_variables" }
