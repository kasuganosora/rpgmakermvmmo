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
