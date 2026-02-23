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
