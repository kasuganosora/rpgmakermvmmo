package resource

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ---- RMMV Data Structures ----

type SystemData struct {
	GameTitle   string `json:"gameTitle"`
	CurrencyUnit string `json:"currencyUnit"`
	StartMapID  int    `json:"startMapId"`
	StartX      int    `json:"startX"`
	StartY      int    `json:"startY"`
}

type Actor struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	ClassID      int    `json:"classId"`
	InitialLevel int    `json:"initialLevel"`
	MaxLevel     int    `json:"maxLevel"`
	CharacterName string `json:"characterName"`
	FaceName     string `json:"faceName"`
}

// ClassLearning represents a skill learned at a certain level.
type ClassLearning struct {
	Level   int    `json:"level"`
	SkillID int    `json:"skillId"`
	Note    string `json:"note"`
}

// Class params is a 2D array: [param_id][level] = value
// params[0] = max HP, [1] = max MP, [2] = ATK, [3] = DEF, [4] = MAT, [5] = MDF, [6] = AGI, [7] = LUK
type Class struct {
	ID        int              `json:"id"`
	Name      string           `json:"name"`
	Params    [][]int          `json:"params"`
	Learnings []ClassLearning  `json:"learnings"`
	Traits    []Trait          `json:"traits"`
}

// SkillDamage holds the damage formula and element from RMMV Skills.json.
type SkillDamage struct {
	Formula   string `json:"formula"`
	ElementID int    `json:"elementId"`
	Type      int    `json:"type"`     // 0=none,1=HP dmg,2=MP dmg,3=HP rec,4=MP rec
	Critical  bool   `json:"critical"` // can crit
	Variance  int    `json:"variance"` // damage variance %
}

type Skill struct {
	ID          int           `json:"id"`
	Name        string        `json:"name"`
	MPCost      int           `json:"mpCost"`
	TPCost      int           `json:"tpCost"`
	IconIndex   int           `json:"iconIndex"`
	Scope       int           `json:"scope"`       // 1-11 target range
	HitType     int           `json:"hitType"`     // 0=certain, 1=physical, 2=magical
	SuccessRate int           `json:"successRate"` // 0-100
	Speed       int           `json:"speed"`       // action speed modifier
	Repeats     int           `json:"repeats"`     // repeat count
	TPGain      int           `json:"tpGain"`      // TP gain on use
	Damage      SkillDamage   `json:"damage"`
	Effects     []SkillEffect `json:"effects"`
	Note        string                 `json:"note"`
	ParsedMeta  map[string]interface{} `json:"-"`
}

type Item struct {
	ID          int           `json:"id"`
	Name        string        `json:"name"`
	Price       int           `json:"price"`
	Consumable  bool          `json:"consumable"`
	Scope       int           `json:"scope"`
	HitType     int           `json:"hitType"`
	SuccessRate int           `json:"successRate"`
	Speed       int           `json:"speed"`
	Damage      SkillDamage   `json:"damage"`
	Effects     []SkillEffect `json:"effects"`
	Note        string                 `json:"note"`
	ParsedMeta  map[string]interface{} `json:"-"`
}

// EquipStats extracts the stat bonuses from a weapon or armor Params array.
// RMMV params order: [0]=maxHP,[1]=maxMP,[2]=atk,[3]=def,[4]=mat,[5]=mdf,[6]=agi,[7]=luk
type EquipStats struct {
	MaxHP, MaxMP     int
	Atk, Def         int
	Mat, Mdf         int
	Agi, Luk         int
}

// EquipStatsFromParams builds EquipStats from an RMMV params array.
func EquipStatsFromParams(params []int) EquipStats {
	get := func(i int) int {
		if i < len(params) {
			return params[i]
		}
		return 0
	}
	return EquipStats{
		MaxHP: get(0), MaxMP: get(1),
		Atk: get(2), Def: get(3),
		Mat: get(4), Mdf: get(5),
		Agi: get(6), Luk: get(7),
	}
}

type Weapon struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Price   int     `json:"price"`
	Params  []int   `json:"params"`
	WtypeID int     `json:"wtypeId"`
	Traits  []Trait `json:"traits"`
	Note       string                 `json:"note"`
	ParsedMeta map[string]interface{} `json:"-"`
}

type Armor struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Price   int     `json:"price"`
	Params  []int   `json:"params"`
	EtypeID int     `json:"etypeId"` // 1=shield,2=helmet,3=body,4=accessory
	AtypeID int     `json:"atypeId"`
	Traits  []Trait `json:"traits"`
	Note       string                 `json:"note"`
	ParsedMeta map[string]interface{} `json:"-"`
}

// Trait represents an RMMV trait entry (used by actors, enemies, classes, equipment, states).
type Trait struct {
	Code   int     `json:"code"`   // 11=element rate, 13=state rate, 21=param, 22=xparam, etc.
	DataID int     `json:"dataId"`
	Value  float64 `json:"value"`
}

// SkillEffect represents an effect attached to a skill or item.
type SkillEffect struct {
	Code   int     `json:"code"`   // 11=add state, 12=remove state, 21=add buff, etc.
	DataID int     `json:"dataId"`
	Value1 float64 `json:"value1"`
	Value2 float64 `json:"value2"`
}

// EnemyAction represents one entry in an RMMV enemy's action pattern table.
type EnemyAction struct {
	SkillID         int     `json:"skillId"`
	ConditionType   int     `json:"conditionType"`   // 0=always, 1=turn, 2=hp, 3=mp, etc.
	ConditionParam1 float64 `json:"conditionParam1"` // float: turn uses int values, hp/mp uses 0.0-1.0
	ConditionParam2 float64 `json:"conditionParam2"`
	Rating          int     `json:"rating"` // AI weight (1-9, 5=standard)
}

type Enemy struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Note        string `json:"note"`        // Note tags for AI config, etc.
	BattlerName string `json:"battlerName"` // enemy graphic name
	HP          int    // maxHP — populated from params[0]
	MP          int    // maxMP — populated from params[1]
	Atk         int    // populated from params[2]
	Def         int    // populated from params[3]
	Mat         int    // populated from params[4]
	Mdf         int    // populated from params[5]
	Agi         int    // populated from params[6]
	Luk         int    // populated from params[7]
	Exp         int           `json:"exp"`
	Gold        int           `json:"gold"`
	DropItems   []EnemyDrop   `json:"dropItems"`
	Actions     []EnemyAction `json:"actions"`
	Traits      []Trait       `json:"traits"`
}

// UnmarshalJSON implements custom unmarshaling for Enemy.
// RMMV stores enemy stats in a "params" array [maxHP, maxMP, atk, def, mat, mdf, agi, luk]
// rather than individual named fields.
func (e *Enemy) UnmarshalJSON(data []byte) error {
	type EnemyAlias Enemy
	aux := &struct {
		*EnemyAlias
		Params []int `json:"params"`
	}{EnemyAlias: (*EnemyAlias)(e)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if len(aux.Params) >= 8 {
		e.HP = aux.Params[0]
		e.MP = aux.Params[1]
		e.Atk = aux.Params[2]
		e.Def = aux.Params[3]
		e.Mat = aux.Params[4]
		e.Mdf = aux.Params[5]
		e.Agi = aux.Params[6]
		e.Luk = aux.Params[7]
	}
	return nil
}

// EnemyDrop represents one entry in the RMMV enemy drop table.
type EnemyDrop struct {
	Kind        int `json:"kind"`        // 1=Item 2=Weapon 3=Armor
	DataID      int `json:"dataId"`      // ID within Items/Weapons/Armors.json
	Denominator int `json:"denominator"` // Drop probability = 1/denominator
}

// TroopPageConditions holds activation conditions for a troop battle event page.
type TroopPageConditions struct {
	TurnValid    bool `json:"turnValid"`
	TurnA        int  `json:"turnA"`
	TurnB        int  `json:"turnB"`
	EnemyValid   bool `json:"enemyValid"`
	EnemyIndex   int  `json:"enemyIndex"`
	EnemyHp      int  `json:"enemyHp"` // percentage 0-100
	ActorValid   bool `json:"actorValid"`
	ActorId      int  `json:"actorId"`
	ActorHp      int  `json:"actorHp"` // percentage 0-100
	SwitchValid  bool `json:"switchValid"`
	SwitchId     int  `json:"switchId"`
	TurnEnding   bool `json:"turnEnding"`
}

// TroopPage is a battle event page on a troop.
type TroopPage struct {
	Conditions TroopPageConditions `json:"conditions"`
	List       []EventCommand      `json:"list"`
	Span       int                 `json:"span"` // 0=battle, 1=turn, 2=moment
}

type Troop struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Members []struct {
		EnemyID int `json:"enemyId"`
		X       int `json:"x"`
		Y       int `json:"y"`
	} `json:"members"`
	Pages []TroopPage `json:"pages"`
}

type State struct {
	ID                int     `json:"id"`
	Name              string  `json:"name"`
	IconIndex         int     `json:"iconIndex"`
	Restriction       int     `json:"restriction"`       // 0=none, 1=attack enemy, 2=attack anyone, 4=cannot move
	Priority          int     `json:"priority"`
	AutoRemovalTiming int     `json:"autoRemovalTiming"` // 0=none, 1=action end, 2=turn end
	MinTurns          int     `json:"minTurns"`
	MaxTurns          int     `json:"maxTurns"`
	RemoveAtBattleEnd bool    `json:"removeAtBattleEnd"`
	RemoveByDamage    bool    `json:"removeByDamage"`
	ChanceByDamage    int     `json:"chanceByDamage"`  // percentage 0-100
	Traits            []Trait `json:"traits"`
	Note              string  `json:"note"`
}

type Animation struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// EventCommand is a single RMMV event command.
// Parameters is []interface{} because RMMV mixes int/string/bool.
type EventCommand struct {
	Code       int           `json:"code"`
	Indent     int           `json:"indent"`
	Parameters []interface{} `json:"parameters"`
}

// EventPageConditions holds the activation conditions for an event page.
type EventPageConditions struct {
	Switch1Valid    bool   `json:"switch1Valid"`
	Switch1ID       int    `json:"switch1Id"`
	Switch2Valid    bool   `json:"switch2Valid"`
	Switch2ID       int    `json:"switch2Id"`
	VariableValid   bool   `json:"variableValid"`
	VariableID      int    `json:"variableId"`
	VariableValue   int    `json:"variableValue"`
	SelfSwitchValid bool   `json:"selfSwitchValid"`
	SelfSwitchCh    string `json:"selfSwitchCh"`
	ActorValid      bool   `json:"actorValid"`
	ActorID         int    `json:"actorId"`
	ItemValid       bool   `json:"itemValid"`
	ItemID          int    `json:"itemId"`
}

// EventImage holds the character sprite image for an event page.
type EventImage struct {
	TileID         int    `json:"tileId"`
	CharacterName  string `json:"characterName"`
	CharacterIndex int    `json:"characterIndex"`
	Direction      int    `json:"direction"`
	Pattern        int    `json:"pattern"`
}

// MoveCommand is a single command in a move route.
type MoveCommand struct {
	Code       int           `json:"code"`
	Parameters []interface{} `json:"parameters"`
}

// MoveRoute defines a custom movement path for an event.
type MoveRoute struct {
	List      []*MoveCommand `json:"list"`
	Repeat    bool           `json:"repeat"`
	Skippable bool           `json:"skippable"`
	Wait      bool           `json:"wait"`
}

// EventPage is one page of an event's command list.
// Trigger: 0=ActionButton, 1=PlayerTouch, 2=EventTouch, 3=Autorun, 4=Parallel
type EventPage struct {
	Conditions    EventPageConditions `json:"conditions"`
	Image         EventImage          `json:"image"`
	Trigger       int                 `json:"trigger"`
	List          []*EventCommand     `json:"list"`
	MoveType      int                 `json:"moveType"`      // 0=fixed,1=random,2=approach,3=custom
	MoveSpeed     int                 `json:"moveSpeed"`     // 1-6
	MoveFrequency int                 `json:"moveFrequency"` // 1-5
	MoveRoute     *MoveRoute          `json:"moveRoute"`
	PriorityType  int                 `json:"priorityType"`  // 0=below,1=same,2=above
	StepAnime     bool                `json:"stepAnime"`
	DirectionFix  bool                `json:"directionFix"`
	Through       bool                `json:"through"`
	WalkAnime     bool                `json:"walkAnime"`
}

// MapEvent is an event object placed on a map.
type MapEvent struct {
	ID    int          `json:"id"`
	Name  string       `json:"name"`
	Note  string       `json:"note"`
	X     int          `json:"x"`
	Y     int          `json:"y"`
	Pages []*EventPage `json:"pages"`
	// OriginalPages stores the event's original pages before TemplateEvent resolution.
	// Used by TE_CALL_ORIGIN_EVENT to call back to the source event's commands.
	// Nil if the event is not a template event.
	OriginalPages []*EventPage `json:"-"`
}

// AudioFile mirrors RMMV's audio descriptor (bgm/bgs/me/se).
type AudioFile struct {
	Name   string `json:"name"`
	Pan    int    `json:"pan"`
	Pitch  int    `json:"pitch"`
	Volume int    `json:"volume"`
}

// MapData represents an RMMV Map*.json file.
type MapData struct {
	ID          int         `json:"id"` // set after load from filename
	DisplayName string      `json:"displayName"`
	Width       int         `json:"width"`
	Height      int         `json:"height"`
	ScrollType  int         `json:"scrollType"` // 0=none, 1=vertical loop, 2=horizontal loop, 3=both
	Data        []int       `json:"data"`       // tileId array: [layer * height * width + y * width + x]
	TilesetID   int         `json:"tilesetId"`
	Events      []*MapEvent `json:"events"` // nil entries are possible (RMMV uses 1-based IDs)
	AutoplayBgm bool        `json:"autoplayBgm"`
	Bgm         *AudioFile  `json:"bgm"`
	AutoplayBgs      bool        `json:"autoplayBgs"`
	Bgs              *AudioFile  `json:"bgs"`
	Battleback1Name  string      `json:"battleback1Name"`
	Battleback2Name  string      `json:"battleback2Name"`
	Note             string      `json:"note"` // map note field for meta tags like <RandomPos>

	// RegionTriggerIndex maps regionID → []eventID for YEP_EventRegionTrigger.
	// Built lazily by BuildRegionTriggerIndex(); nil until first call.
	RegionTriggerIndex map[int][]int `json:"-"`

	// BattlebackOverrides maps regionID → [bb1, bb2] for region-specific battlebacks.
	// Parsed from map Note tags: <Region N Battleback1: file> / <Region N Battleback2: file>
	// Built lazily by BuildBattlebackOverrides(); nil until first call.
	BattlebackOverrides map[int][2]string `json:"-"`
}

// IsLoopHorizontal returns true if the map wraps horizontally (scrollType 2 or 3).
func (md *MapData) IsLoopHorizontal() bool {
	return md.ScrollType == 2 || md.ScrollType == 3
}

// IsLoopVertical returns true if the map wraps vertically (scrollType 1 or 3).
func (md *MapData) IsLoopVertical() bool {
	return md.ScrollType == 1 || md.ScrollType == 3
}

// RoundX wraps x coordinate for horizontal looping maps.
func (md *MapData) RoundX(x int) int {
	if md.IsLoopHorizontal() {
		return ((x % md.Width) + md.Width) % md.Width
	}
	return x
}

// RoundY wraps y coordinate for vertical looping maps.
func (md *MapData) RoundY(y int) int {
	if md.IsLoopVertical() {
		return ((y % md.Height) + md.Height) % md.Height
	}
	return y
}

// TransferTarget holds the destination of a map transfer event.
type TransferTarget struct {
	MapID int
	X     int
	Y     int
	Dir   int
}

// FindTransferAt checks if there is a player-touch (trigger 1 or 2) transfer event
// at the given (x, y) coordinate. Returns the target if found.
func (md *MapData) FindTransferAt(x, y int) *TransferTarget {
	for _, ev := range md.Events {
		if ev == nil || ev.X != x || ev.Y != y {
			continue
		}
		for _, page := range ev.Pages {
			if page == nil {
				continue
			}
			// Only trigger on player touch (1) or event touch (2).
			if page.Trigger != 1 && page.Trigger != 2 {
				continue
			}
			for _, cmd := range page.List {
				if cmd == nil || cmd.Code != 201 {
					continue
				}
				// RMMV Transfer command params: [mode, mapID, x, y, dir, fadeType]
				if len(cmd.Parameters) < 4 {
					continue
				}
				return &TransferTarget{
					MapID: paramIntP(cmd.Parameters, 1),
					X:     paramIntP(cmd.Parameters, 2),
					Y:     paramIntP(cmd.Parameters, 3),
					Dir:   paramIntP(cmd.Parameters, 4),
				}
			}
		}
	}
	return nil
}

// BuildRegionTriggerIndex scans all event pages for YEP_EventRegionTrigger comment tags
// (code 108/408 with "<Region Trigger: N>" or "<Region Triggers: N,N,...>") and builds
// RegionTriggerIndex: regionID → []eventID.
// Safe to call multiple times; rebuilds the index on each call.
func (md *MapData) BuildRegionTriggerIndex() {
	idx := make(map[int][]int)
	reSingle := regexp.MustCompile(`(?i)<Region\s+Triggers?:\s*(\d+)>`)
	reMulti := regexp.MustCompile(`(?i)<Region\s+Triggers?:\s*([\d,\s]+)>`)
	for _, ev := range md.Events {
		if ev == nil {
			continue
		}
		seen := make(map[int]bool) // deduplicate per event
		for _, page := range ev.Pages {
			if page == nil {
				continue
			}
			for _, cmd := range page.List {
				if cmd == nil || (cmd.Code != 108 && cmd.Code != 408) {
					continue
				}
				if len(cmd.Parameters) == 0 {
					continue
				}
				text, _ := cmd.Parameters[0].(string)
				if text == "" {
					continue
				}
				// Try multi first (superset of single).
				if m := reMulti.FindStringSubmatch(text); m != nil {
					for _, part := range regexp.MustCompile(`\d+`).FindAllString(m[1], -1) {
						var n int
						fmt.Sscanf(part, "%d", &n)
						if n > 0 && !seen[n] {
							seen[n] = true
							idx[n] = append(idx[n], ev.ID)
						}
					}
				} else if m := reSingle.FindStringSubmatch(text); m != nil {
					var n int
					fmt.Sscanf(m[1], "%d", &n)
					if n > 0 && !seen[n] {
						seen[n] = true
						idx[n] = append(idx[n], ev.ID)
					}
				}
			}
		}
	}
	md.RegionTriggerIndex = idx
}

// BuildBattlebackOverrides parses region-specific battleback Note tags from the map Note field.
// Supported formats:
//
//	<Region N Battleback1: filename>
//	<Region N Battleback2: filename>
//
// Fills BattlebackOverrides: regionID → [bb1, bb2] (empty string = no override for that slot).
func (md *MapData) BuildBattlebackOverrides() {
	reBB1 := regexp.MustCompile(`(?i)<Region\s+(\d+)\s+Battleback1:\s*([^>]+)>`)
	reBB2 := regexp.MustCompile(`(?i)<Region\s+(\d+)\s+Battleback2:\s*([^>]+)>`)
	idx := make(map[int][2]string)
	for _, m := range reBB1.FindAllStringSubmatch(md.Note, -1) {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		if n > 0 {
			entry := idx[n]
			entry[0] = strings.TrimSpace(m[2])
			idx[n] = entry
		}
	}
	for _, m := range reBB2.FindAllStringSubmatch(md.Note, -1) {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		if n > 0 {
			entry := idx[n]
			entry[1] = strings.TrimSpace(m[2])
			idx[n] = entry
		}
	}
	md.BattlebackOverrides = idx
}

// BattlebackAt returns the battleback filenames for the given region ID.
// Falls back to the map-level default if no region override is found.
// Returns (bb1, bb2) where empty string means "use engine default".
func (md *MapData) BattlebackAt(regionID int) (bb1, bb2 string) {
	if md.BattlebackOverrides == nil {
		md.BuildBattlebackOverrides()
	}
	if regionID > 0 {
		if entry, ok := md.BattlebackOverrides[regionID]; ok {
			r1 := entry[0]
			r2 := entry[1]
			if r1 == "" {
				r1 = md.Battleback1Name
			}
			if r2 == "" {
				r2 = md.Battleback2Name
			}
			return r1, r2
		}
	}
	return md.Battleback1Name, md.Battleback2Name
}

// paramIntP extracts an int from a []interface{} at the given index (JSON numbers are float64).
func paramIntP(params []interface{}, idx int) int {
	if idx >= len(params) {
		return 0
	}
	switch v := params[idx].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

type MapInfo struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	ParentID int    `json:"parentId"`
}

type CommonEvent struct {
	ID       int             `json:"id"`
	Name     string          `json:"name"`
	Trigger  int             `json:"trigger"`
	SwitchID int             `json:"switchId"`
	List     []*EventCommand `json:"list"`
}

type Tileset struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Flags    []int  `json:"flags"` // passability flags per tileId
}

// PassabilityMap stores passability for each (x, y) in 4 directions,
// plus region IDs from RMMV's layer 5.
// dir: 0=down(2), 1=left(4), 2=right(6), 3=up(8)
type PassabilityMap struct {
	Width  int
	Height int
	// data[y][x][dir]
	data [][][4]bool
	// regions[y][x] = region ID (RMMV layer 5); nil if map has < 6 layers.
	regions [][]int
	// Map wrapping (RMMV scrollType). Set from MapData.ScrollType during buildPassability.
	loopH bool // horizontal loop (scrollType 2 or 3)
	loopV bool // vertical loop (scrollType 1 or 3)
}

func newPassabilityMap(w, h int) *PassabilityMap {
	pm := &PassabilityMap{Width: w, Height: h}
	pm.data = make([][][4]bool, h)
	for y := range pm.data {
		pm.data[y] = make([][4]bool, w)
		for x := range pm.data[y] {
			// default: all passable
			pm.data[y][x] = [4]bool{true, true, true, true}
		}
	}
	return pm
}

// NewPassabilityMap creates a PassabilityMap with all tiles passable by default.
// Exported for use by other packages (e.g., tests).
func NewPassabilityMap(w, h int) *PassabilityMap {
	return newPassabilityMap(w, h)
}

// SetPass sets the passability for a specific RMMV direction (2/4/6/8) at (x, y).
func (pm *PassabilityMap) SetPass(x, y, dir int, passable bool) {
	if x < 0 || x >= pm.Width || y < 0 || y >= pm.Height {
		return
	}
	switch dir {
	case 2:
		pm.data[y][x][0] = passable
	case 4:
		pm.data[y][x][1] = passable
	case 6:
		pm.data[y][x][2] = passable
	case 8:
		pm.data[y][x][3] = passable
	}
}

// SetRegion sets the region ID at (x, y), initializing the region layer if needed.
func (pm *PassabilityMap) SetRegion(x, y, regionID int) {
	if x < 0 || x >= pm.Width || y < 0 || y >= pm.Height {
		return
	}
	if pm.regions == nil {
		pm.regions = make([][]int, pm.Height)
		for i := range pm.regions {
			pm.regions[i] = make([]int, pm.Width)
		}
	}
	pm.regions[y][x] = regionID
}

// IsLoopH returns true if this map loops horizontally.
func (pm *PassabilityMap) IsLoopH() bool { return pm.loopH }

// IsLoopV returns true if this map loops vertically.
func (pm *PassabilityMap) IsLoopV() bool { return pm.loopV }

// RoundX wraps x coordinate for horizontal looping maps.
func (pm *PassabilityMap) RoundX(x int) int {
	if pm.loopH {
		return ((x % pm.Width) + pm.Width) % pm.Width
	}
	return x
}

// RoundY wraps y coordinate for vertical looping maps.
func (pm *PassabilityMap) RoundY(y int) int {
	if pm.loopV {
		return ((y % pm.Height) + pm.Height) % pm.Height
	}
	return y
}

// IsValid checks if (x, y) is within map bounds, accounting for wrapping.
// On looping axes, the coordinate is always valid (it wraps).
func (pm *PassabilityMap) IsValid(x, y int) bool {
	xOK := pm.loopH || (x >= 0 && x < pm.Width)
	yOK := pm.loopV || (y >= 0 && y < pm.Height)
	return xOK && yOK
}

// CanPass reports whether movement in the given RPG Maker direction (2/4/6/8) is allowed at (x,y).
// Supports map wrapping: coordinates are rounded for looping maps.
func (pm *PassabilityMap) CanPass(x, y, dir int) bool {
	x = pm.RoundX(x)
	y = pm.RoundY(y)
	if x < 0 || x >= pm.Width || y < 0 || y >= pm.Height {
		return false
	}
	switch dir {
	case 2:
		return pm.data[y][x][0]
	case 4:
		return pm.data[y][x][1]
	case 6:
		return pm.data[y][x][2]
	case 8:
		return pm.data[y][x][3]
	}
	return false
}

// RegionAt returns the region ID at (x, y), or 0 if out of bounds / no region data.
// Supports map wrapping: coordinates are rounded for looping maps.
func (pm *PassabilityMap) RegionAt(x, y int) int {
	x = pm.RoundX(x)
	y = pm.RoundY(y)
	if pm.regions == nil || x < 0 || x >= pm.Width || y < 0 || y >= pm.Height {
		return 0
	}
	return pm.regions[y][x]
}

// RegionRestrictions stores the region-based movement restriction config
// parsed from the YEP_RegionRestrictions plugin.
type RegionRestrictions struct {
	PlayerRestrict []int // regions that block player movement
	PlayerAllow    []int // regions that always allow player movement
	EventRestrict  []int // regions that block event/NPC movement
	AllRestrict    []int // regions that block all movement
	EventAllow     []int // regions that always allow event movement
	AllAllow       []int // regions that always allow all movement
}

// IsPlayerRestricted returns true if the given region blocks player movement.
func (rr *RegionRestrictions) IsPlayerRestricted(regionID int) bool {
	if regionID == 0 {
		return false
	}
	for _, r := range rr.PlayerRestrict {
		if r == regionID {
			return true
		}
	}
	for _, r := range rr.AllRestrict {
		if r == regionID {
			return true
		}
	}
	return false
}

// IsPlayerAllowed returns true if the given region always allows player movement
// (overrides tile passability).
func (rr *RegionRestrictions) IsPlayerAllowed(regionID int) bool {
	if regionID == 0 {
		return false
	}
	for _, r := range rr.PlayerAllow {
		if r == regionID {
			return true
		}
	}
	for _, r := range rr.AllAllow {
		if r == regionID {
			return true
		}
	}
	return false
}

// IsEventRestricted returns true if the given region blocks NPC/event movement.
func (rr *RegionRestrictions) IsEventRestricted(regionID int) bool {
	if regionID == 0 {
		return false
	}
	for _, r := range rr.EventRestrict {
		if r == regionID {
			return true
		}
	}
	for _, r := range rr.AllRestrict {
		if r == regionID {
			return true
		}
	}
	return false
}

// IsEventAllowed returns true if the given region always allows NPC/event movement.
func (rr *RegionRestrictions) IsEventAllowed(regionID int) bool {
	if regionID == 0 {
		return false
	}
	for _, r := range rr.EventAllow {
		if r == regionID {
			return true
		}
	}
	for _, r := range rr.AllAllow {
		if r == regionID {
			return true
		}
	}
	return false
}

// ---- ResourceLoader ----

// ResourceLoader reads and holds all RMMV data files.
type ResourceLoader struct {
	DataPath     string
	ImgPath      string
	System       *SystemData
	Actors       []*Actor
	Classes      []*Class
	Skills       []*Skill
	Items        []*Item
	Weapons      []*Weapon
	Armors       []*Armor
	Enemies      []*Enemy
	Troops       []*Troop
	States       []*State
	Animations   []*Animation
	Maps         map[int]*MapData
	MapInfos     []*MapInfo
	CommonEvents []*CommonEvent
	Tilesets     []*Tileset
	Passability  map[int]*PassabilityMap

	// IncomingTransfers maps destMapID → list of arrival positions.
	// Built by scanning all maps for transfer commands (code 201) and recording
	// the destination coordinates grouped by destination map ID.
	IncomingTransfers map[int][]EntryPoint

	// RegionRestr holds region-based movement restriction config from
	// the YEP_RegionRestrictions plugin. Nil if plugin is not active.
	RegionRestr *RegionRestrictions

	// CPStarPassFix is true when the CP_Star_Passability_Fix plugin is active.
	// Changes star tile (flag & 0x10) behavior: star tiles CAN block passage
	// if their direction bit is set.
	CPStarPassFix bool

	// CommonEventsByName maps CE name → CE ID for name-based lookups.
	// Built during loadCommonEvents() to support CallCommon plugin commands.
	CommonEventsByName map[string]int

	// TagSkillList holds AddSkillEffectBase data from TagSkillList.json.
	// Maps skill tag index (21-122) to base/add variable mappings.
	// Used by CulSkillEffect plugin to compute base stat values from equipment effects.
	TagSkillList map[int]*TagSkillEntry

	// Pre-built data arrays as Go slices for fast injection into Goja VMs.
	// Built once at load time; each element is map[string]interface{} or nil.
	PrebuiltArmors  []interface{}
	PrebuiltWeapons []interface{}
	PrebuiltSkills  []interface{}
	PrebuiltItems   []interface{}

	// InitState holds client initialization data loaded from InitState.json.
	// Sent to client via map_init to replace hardcoded projectb-specific init.
	// nil if file doesn't exist (optional).
	InitState json.RawMessage

	// MMOConfig holds game-specific configuration loaded from MMOConfig.json.
	// Replaces all hardcoded projectb-specific values (plugin commands,
	// broadcast whitelists, time period formula, etc.).
	// nil if file doesn't exist (framework runs with built-in defaults).
	MMOConfig *MMOConfig
}

// ServerExecPlugin defines a plugin command that runs server-side via Goja VM.
type ServerExecPlugin struct {
	ScriptFile       string `json:"scriptFile"`       // path relative to game root
	Timeout          int    `json:"timeout"`           // execution timeout in ms
	InjectActors     bool   `json:"injectActors"`      // inject $gameActors
	InjectDataArrays bool   `json:"injectDataArrays"`  // inject $dataArmors etc.
	InjectPlayerVars bool   `json:"injectPlayerVars"`  // inject __playerLevel, __gold, __classId
	TagSkillRange    []int  `json:"tagSkillListRange"` // [start, end] for TagSkillList post-processing

	// LoadedScript holds the file contents, loaded at startup.
	LoadedScript string `json:"-"`
}

// TimePeriodRange maps an hour threshold to a period value.
type TimePeriodRange struct {
	MaxHour int `json:"maxHour"`
	Period  int `json:"period"`
}

// TimePeriodConfig defines how to compute the time period variable from the hour variable.
type TimePeriodConfig struct {
	HourVar   int               `json:"hourVar"`
	PeriodVar int               `json:"periodVar"`
	Ranges    []TimePeriodRange `json:"ranges"`
}

// MMOConfig holds game-specific configuration that varies per RMMV project.
type MMOConfig struct {
	BlockedPluginCmds  []string                      `json:"blockedPluginCmds"`
	ServerExecPlugins  map[string]*ServerExecPlugin   `json:"serverExecPlugins"`
	BroadcastVariables []int                          `json:"broadcastVariables"`
	BroadcastSwitches  []int                          `json:"broadcastSwitches"`
	// AlwaysSendSwitches lists switch IDs that are always forwarded to the client
	// even when the server-side value is unchanged. Use for switches the client may
	// reset independently (e.g. via event_end safety nets).
	AlwaysSendSwitches []int                          `json:"alwaysSendSwitches"`
	TimePeriod         *TimePeriodConfig              `json:"timePeriod"`
	SafeScriptPrefixes []string                       `json:"safeScriptPrefixes"`
	SafeScreenMethods  []string                       `json:"safeScreenMethods"`
	Battle             *BattleMMOConfig               `json:"battle"`
	MonsterSpawns      []MonsterSpawnConfig           `json:"monsterSpawns"`
	MonsterAIProfiles  map[string]*MonsterAIProfile   `json:"monsterAIProfiles"`
}

// MonsterSpawnConfig defines where and how monsters spawn on a map.
type MonsterSpawnConfig struct {
	MapID      int    `json:"mapId"`
	EnemyID    int    `json:"enemyId"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	MaxCount   int    `json:"maxCount"`
	RespawnSec int    `json:"respawnSec"`
	AIOverride  string `json:"aiOverride"`  // optional: override enemy's Note AI profile
	GroupID     string `json:"groupId"`     // group identifier — same groupId monsters form a group
	GroupType   string `json:"groupType"`   // "assist" | "linked" | "pack" (default: "assist")
	AssistRange int    `json:"assistRange"` // assist call range in tiles (default: 5, assist mode only)
}

// MonsterAIProfile defines a custom AI behavior profile.
type MonsterAIProfile struct {
	AggroRange         int `json:"aggroRange"`
	LeashRange         int `json:"leashRange"`
	AttackRange        int `json:"attackRange"`
	AttackCooldownTicks int `json:"attackCooldownTicks"`
	MoveIntervalTicks  int `json:"moveIntervalTicks"`
	WanderRadius       int `json:"wanderRadius"`
	FleeHPPercent      int `json:"fleeHPPercent"`
}

// BattleMMOConfig holds battle-specific configuration (YEP plugin integration + real-time combat).
type BattleMMOConfig struct {
	// BaseTroopID mirrors YEP_BaseTroopEvents: the troop whose event pages are
	// merged into every battle's TroopEventRunner at initialization time.
	// Set to 0 (default) to disable.
	BaseTroopID int `json:"baseTroopId"`

	// CombatMode controls which combat systems are active:
	//   "turnbased" — only RunBattle (field attacks disabled)
	//   "realtime"  — only field attacks (RunBattle disabled)
	//   "hybrid"    — both active (default, backward-compatible)
	CombatMode string `json:"combatMode"`

	// RealtimeGCDMs is the global cooldown in milliseconds between player attacks.
	// Default 1000 (1 second). Only applies when field attacks are enabled.
	RealtimeGCDMs int `json:"realtimeGCDMs"`

	// RealtimeAttackRange is the max Manhattan distance for field attacks (default 1 = melee).
	RealtimeAttackRange int `json:"realtimeAttackRange"`

	// EnforceSkillCosts deducts MP/TP when using skills in field combat.
	EnforceSkillCosts bool `json:"enforceSkillCosts"`

	// DeathReviveMapID/X/Y is the respawn point after player death.
	// If DeathReviveMapID is 0, revive on the same map at current position.
	DeathReviveMapID int `json:"deathReviveMapId"`
	DeathReviveX     int `json:"deathReviveX"`
	DeathReviveY     int `json:"deathReviveY"`

	// DeathPenaltyExpPct is the percentage of current-level exp lost on death (0 = none).
	DeathPenaltyExpPct int `json:"deathPenaltyExpPct"`
}

// GetCombatMode returns the effective combat mode, defaulting to "hybrid".
func (c *BattleMMOConfig) GetCombatMode() string {
	if c == nil || c.CombatMode == "" {
		return "hybrid"
	}
	return c.CombatMode
}

// GetGCDMs returns the effective GCD in milliseconds, defaulting to 1000.
func (c *BattleMMOConfig) GetGCDMs() int {
	if c == nil || c.RealtimeGCDMs <= 0 {
		return 1000
	}
	return c.RealtimeGCDMs
}

// GetAttackRange returns the effective attack range, defaulting to 1.
func (c *BattleMMOConfig) GetAttackRange() int {
	if c == nil || c.RealtimeAttackRange <= 0 {
		return 1
	}
	return c.RealtimeAttackRange
}

// BlockedPluginCmdSet returns the blocked plugin commands as a set for O(1) lookup.
func (c *MMOConfig) BlockedPluginCmdSet() map[string]bool {
	m := make(map[string]bool, len(c.BlockedPluginCmds))
	for _, cmd := range c.BlockedPluginCmds {
		m[cmd] = true
	}
	return m
}

// BroadcastVarSet returns broadcast variable IDs as a set.
func (c *MMOConfig) BroadcastVarSet() map[int]bool {
	m := make(map[int]bool, len(c.BroadcastVariables))
	for _, id := range c.BroadcastVariables {
		m[id] = true
	}
	return m
}

// BroadcastSwitchSet returns broadcast switch IDs as a set.
func (c *MMOConfig) BroadcastSwitchSet() map[int]bool {
	m := make(map[int]bool, len(c.BroadcastSwitches))
	for _, id := range c.BroadcastSwitches {
		m[id] = true
	}
	return m
}

// AlwaysSendSwitchSet returns always-send switch IDs as a set.
func (c *MMOConfig) AlwaysSendSwitchSet() map[int]bool {
	m := make(map[int]bool, len(c.AlwaysSendSwitches))
	for _, id := range c.AlwaysSendSwitches {
		m[id] = true
	}
	return m
}

// SafeScreenMethodSet returns safe $gameScreen methods as a set.
func (c *MMOConfig) SafeScreenMethodSet() map[string]bool {
	m := make(map[string]bool, len(c.SafeScreenMethods))
	for _, method := range c.SafeScreenMethods {
		m[method] = true
	}
	return m
}

// ComputeTimePeriod returns the period value for a given hour.
// Returns 0 if no time period config is set.
func (c *MMOConfig) ComputeTimePeriod(hour int) int {
	if c.TimePeriod == nil {
		return 0
	}
	for _, r := range c.TimePeriod.Ranges {
		if hour < r.MaxHour {
			return r.Period
		}
	}
	return 0
}

// TagSkillEntry represents a single entry in TagSkillList.json.
// AddSkillEffectBase logic: v[BaseVar] = BaseNum + v[AddVar]
type TagSkillEntry struct {
	BaseVar int `json:"BaseVar"` // target variable ID
	AddVar  int `json:"AddVar"`  // source accumulator variable ID
	BaseNum int `json:"BaseNum"` // base value before skill effects
}

// EntryPoint is a position where players arrive when transferring to a map.
type EntryPoint struct {
	X, Y int
}

// NewLoader creates a ResourceLoader for the given RMMV data directory.
func NewLoader(dataPath, imgPath string) *ResourceLoader {
	return &ResourceLoader{
		DataPath: dataPath,
		ImgPath:  imgPath,
		Maps:     make(map[int]*MapData),
		Passability: make(map[int]*PassabilityMap),
	}
}

// Load reads all RMMV data files and pre-computes derived data.
func (rl *ResourceLoader) Load() error {
	loaders := []func() error{
		rl.loadSystem,
		rl.loadActors,
		rl.loadClasses,
		rl.loadSkills,
		rl.loadItems,
		rl.loadWeapons,
		rl.loadArmors,
		rl.loadEnemies,
		rl.loadTroops,
		rl.loadStates,
		rl.loadAnimations,
		rl.loadMapInfos,
		rl.loadTilesets,
		rl.loadCommonEvents,
		rl.loadMaps,
	}
	for _, fn := range loaders {
		if err := fn(); err != nil {
			return err
		}
	}
	// Apply plugin adapters after maps are loaded but before derived data.
	// This resolves template events, etc. based on detected RMMV plugins.
	if err := rl.applyPluginAdapters(); err != nil {
		return err
	}
	rl.buildPassability()
	rl.buildIncomingTransfers()
	rl.loadTagSkillList() // optional, ignore errors
	rl.preParseAllMeta()
	rl.prebuildDataArrays()
	rl.loadInitState()   // optional, ignore errors
	rl.loadMMOConfig()   // optional, ignore errors
	return nil
}

// loadInitState loads InitState.json as raw JSON for client-side initialization.
// The file is optional; missing file is not an error.
func (rl *ResourceLoader) loadInitState() {
	path := filepath.Join(rl.DataPath, "InitState.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return // file not found is fine
	}
	// Validate it's valid JSON
	if json.Valid(data) {
		rl.InitState = json.RawMessage(data)
	}
}

// loadMMOConfig loads MMOConfig.json and its referenced plugin script files.
// The file is optional; missing file is not an error (framework uses defaults).
func (rl *ResourceLoader) loadMMOConfig() {
	path := filepath.Join(rl.DataPath, "MMOConfig.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return // file not found is fine
	}
	var cfg MMOConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return
	}
	// Load script files for each server-exec plugin.
	gameRoot := filepath.Dir(rl.DataPath) // parent of data/ is the game root
	for name, plugin := range cfg.ServerExecPlugins {
		if plugin == nil || plugin.ScriptFile == "" {
			continue
		}
		scriptPath := filepath.Join(gameRoot, filepath.FromSlash(plugin.ScriptFile))
		scriptData, err := os.ReadFile(scriptPath)
		if err != nil {
			// Script file missing — remove plugin from config so it won't be called.
			delete(cfg.ServerExecPlugins, name)
			continue
		}
		plugin.LoadedScript = string(scriptData)
	}
	rl.MMOConfig = &cfg
}

// metaTagRe matches RMMV meta tags including kaeru.js semicolon extension.
var metaTagRe = regexp.MustCompile(`<([^<>:;]+)([:;]?)([^>]*)>`)

// ParseMetaGo parses RMMV Note field meta tags into a Go map.
// Format: <key> → true, <key:value> → string, <key;json> → JSON-parsed value.
func ParseMetaGo(note string) map[string]interface{} {
	if note == "" {
		return nil
	}
	meta := make(map[string]interface{})
	for _, match := range metaTagRe.FindAllStringSubmatch(note, -1) {
		key := match[1]
		switch match[2] {
		case ":":
			meta[key] = match[3]
		case ";":
			var parsed interface{}
			if err := json.Unmarshal([]byte(match[3]), &parsed); err == nil {
				meta[key] = parsed
			} else {
				meta[key] = match[3]
			}
		default:
			meta[key] = true
		}
	}
	return meta
}

// preParseAllMeta pre-parses Note fields into ParsedMeta for all data arrays.
func (rl *ResourceLoader) preParseAllMeta() {
	for _, a := range rl.Armors {
		if a != nil {
			a.ParsedMeta = ParseMetaGo(a.Note)
		}
	}
	for _, w := range rl.Weapons {
		if w != nil {
			w.ParsedMeta = ParseMetaGo(w.Note)
		}
	}
	for _, s := range rl.Skills {
		if s != nil {
			s.ParsedMeta = ParseMetaGo(s.Note)
		}
	}
	for _, item := range rl.Items {
		if item != nil {
			item.ParsedMeta = ParseMetaGo(item.Note)
		}
	}
}

// prebuildDataArrays builds Go slices for fast injection into Goja VMs.
func (rl *ResourceLoader) prebuildDataArrays() {
	if rl.Armors != nil {
		arr := make([]interface{}, len(rl.Armors))
		for i, a := range rl.Armors {
			if a == nil {
				continue
			}
			m := map[string]interface{}{
				"id": a.ID, "name": a.Name, "price": a.Price,
				"etypeId": a.EtypeID, "atypeId": a.AtypeID, "params": a.Params,
			}
			if a.ParsedMeta != nil {
				m["meta"] = a.ParsedMeta
			} else {
				m["meta"] = map[string]interface{}{}
			}
			arr[i] = m
		}
		rl.PrebuiltArmors = arr
	}
	if rl.Weapons != nil {
		arr := make([]interface{}, len(rl.Weapons))
		for i, w := range rl.Weapons {
			if w == nil {
				continue
			}
			m := map[string]interface{}{
				"id": w.ID, "name": w.Name, "price": w.Price,
				"wtypeId": w.WtypeID, "params": w.Params,
			}
			if w.ParsedMeta != nil {
				m["meta"] = w.ParsedMeta
			} else {
				m["meta"] = map[string]interface{}{}
			}
			arr[i] = m
		}
		rl.PrebuiltWeapons = arr
	}
	if rl.Skills != nil {
		arr := make([]interface{}, len(rl.Skills))
		for i, sk := range rl.Skills {
			if sk == nil {
				continue
			}
			m := map[string]interface{}{
				"id": sk.ID, "name": sk.Name, "iconIndex": sk.IconIndex,
				"mpCost": sk.MPCost, "tpCost": sk.TPCost, "scope": sk.Scope,
			}
			if sk.ParsedMeta != nil {
				m["meta"] = sk.ParsedMeta
			} else {
				m["meta"] = map[string]interface{}{}
			}
			arr[i] = m
		}
		rl.PrebuiltSkills = arr
	}
	if rl.Items != nil {
		arr := make([]interface{}, len(rl.Items))
		for i, item := range rl.Items {
			if item == nil {
				continue
			}
			m := map[string]interface{}{
				"id": item.ID, "name": item.Name, "price": item.Price,
			}
			if item.ParsedMeta != nil {
				m["meta"] = item.ParsedMeta
			} else {
				m["meta"] = map[string]interface{}{}
			}
			arr[i] = m
		}
		rl.PrebuiltItems = arr
	}
}

func (rl *ResourceLoader) path(file string) string {
	return filepath.Join(rl.DataPath, file)
}

func loadJSONArray[T any](path string) ([]*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("resource: read %s: %w", path, err)
	}
	var arr []*T
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, fmt.Errorf("resource: parse %s: %w", path, err)
	}
	return arr, nil
}

func loadJSONObject[T any](path string, out *T) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("resource: read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("resource: parse %s: %w", path, err)
	}
	return nil
}

// loadTagSkillList loads TagSkillList.json (optional custom data file).
// Used by CulSkillEffect's AddSkillEffectBase to compute base stat values.
func (rl *ResourceLoader) loadTagSkillList() {
	p := rl.path("TagSkillList.json")
	data, err := os.ReadFile(p)
	if err != nil {
		return // file doesn't exist — not an error
	}
	var raw map[string]*TagSkillEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	rl.TagSkillList = make(map[int]*TagSkillEntry, len(raw))
	for k, v := range raw {
		var id int
		if _, err := fmt.Sscanf(k, "%d", &id); err == nil && v != nil {
			rl.TagSkillList[id] = v
		}
	}
}

func (rl *ResourceLoader) loadSystem() error {
	rl.System = &SystemData{}
	return loadJSONObject(rl.path("System.json"), rl.System)
}

func (rl *ResourceLoader) loadActors() error {
	var err error
	rl.Actors, err = loadJSONArray[Actor](rl.path("Actors.json"))
	return err
}

func (rl *ResourceLoader) loadClasses() error {
	var err error
	rl.Classes, err = loadJSONArray[Class](rl.path("Classes.json"))
	return err
}

func (rl *ResourceLoader) loadSkills() error {
	var err error
	rl.Skills, err = loadJSONArray[Skill](rl.path("Skills.json"))
	return err
}

func (rl *ResourceLoader) loadItems() error {
	var err error
	rl.Items, err = loadJSONArray[Item](rl.path("Items.json"))
	return err
}

func (rl *ResourceLoader) loadWeapons() error {
	var err error
	rl.Weapons, err = loadJSONArray[Weapon](rl.path("Weapons.json"))
	return err
}

func (rl *ResourceLoader) loadArmors() error {
	var err error
	rl.Armors, err = loadJSONArray[Armor](rl.path("Armors.json"))
	return err
}

func (rl *ResourceLoader) loadEnemies() error {
	var err error
	rl.Enemies, err = loadJSONArray[Enemy](rl.path("Enemies.json"))
	return err
}

func (rl *ResourceLoader) loadTroops() error {
	var err error
	rl.Troops, err = loadJSONArray[Troop](rl.path("Troops.json"))
	return err
}

func (rl *ResourceLoader) loadStates() error {
	var err error
	rl.States, err = loadJSONArray[State](rl.path("States.json"))
	return err
}

func (rl *ResourceLoader) loadAnimations() error {
	var err error
	rl.Animations, err = loadJSONArray[Animation](rl.path("Animations.json"))
	return err
}

func (rl *ResourceLoader) loadMapInfos() error {
	var err error
	rl.MapInfos, err = loadJSONArray[MapInfo](rl.path("MapInfos.json"))
	return err
}

func (rl *ResourceLoader) loadTilesets() error {
	var err error
	rl.Tilesets, err = loadJSONArray[Tileset](rl.path("Tilesets.json"))
	return err
}

func (rl *ResourceLoader) loadCommonEvents() error {
	var err error
	rl.CommonEvents, err = loadJSONArray[CommonEvent](rl.path("CommonEvents.json"))
	if err != nil {
		return err
	}
	rl.buildCommonEventIndex()
	return nil
}

// buildCommonEventIndex builds the CommonEventsByName lookup map.
func (rl *ResourceLoader) buildCommonEventIndex() {
	rl.CommonEventsByName = make(map[string]int, len(rl.CommonEvents))
	for i, ce := range rl.CommonEvents {
		if ce != nil && ce.Name != "" {
			rl.CommonEventsByName[ce.Name] = i
		}
	}
}

// FindCommonEventByName returns the CE ID for the given name, or 0 if not found.
func (rl *ResourceLoader) FindCommonEventByName(name string) int {
	if rl.CommonEventsByName == nil {
		return 0
	}
	return rl.CommonEventsByName[name]
}

// FindCommonEventByPrefix returns the CE ID for the first CE whose name starts
// with the given prefix (matching MPP_CallCommonByName CCT behavior), or 0.
func (rl *ResourceLoader) FindCommonEventByPrefix(prefix string) int {
	// MPP_CallCommonByName searches from last to first, so we do the same.
	for i := len(rl.CommonEvents) - 1; i > 0; i-- {
		ce := rl.CommonEvents[i]
		if ce != nil && len(ce.Name) >= len(prefix) && ce.Name[:len(prefix)] == prefix {
			return i
		}
	}
	return 0
}

var mapFileRegex = regexp.MustCompile(`^Map(\d+)\.json$`)

func (rl *ResourceLoader) loadMaps() error {
	entries, err := os.ReadDir(rl.DataPath)
	if err != nil {
		return fmt.Errorf("resource: readdir %s: %w", rl.DataPath, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := mapFileRegex.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		var mapID int
		fmt.Sscanf(m[1], "%d", &mapID)

		md := &MapData{ID: mapID}
		if err := loadJSONObject(filepath.Join(rl.DataPath, e.Name()), md); err != nil {
			return err
		}
		rl.Maps[mapID] = md
	}
	return nil
}

// buildPassability pre-computes passability for each map using tile flags.
// Matches RMMV's Game_Map.checkPassage logic: iterate tiles from top layer to
// bottom, skip star tiles (flag & 0x10), and use the FIRST non-star tile to
// determine passability per direction.
//
// If CPStarPassFix is true (CP_Star_Passability_Fix plugin active), star tiles
// CAN block passage when their direction bit is set.
//
// Also extracts region IDs from layer 5 of the map data.
// BuildPassabilityExported is an exported wrapper for buildPassability, for use in tests.
func (rl *ResourceLoader) BuildPassabilityExported() { rl.buildPassability() }

func (rl *ResourceLoader) buildPassability() {
	// Build a map from tilesetID → tileset for quick lookup.
	tilesetMap := make(map[int]*Tileset)
	for _, ts := range rl.Tilesets {
		if ts != nil {
			tilesetMap[ts.ID] = ts
		}
	}

	cpStarFix := rl.CPStarPassFix

	for mapID, md := range rl.Maps {
		ts, ok := tilesetMap[md.TilesetID]
		if !ok || len(ts.Flags) == 0 {
			// No tileset data – mark everything passable.
			pm := newPassabilityMap(md.Width, md.Height)
			pm.loopH = md.IsLoopHorizontal()
			pm.loopV = md.IsLoopVertical()
			rl.extractRegions(pm, md)
			rl.Passability[mapID] = pm
			continue
		}

		pm := newPassabilityMap(md.Width, md.Height)
		pm.loopH = md.IsLoopHorizontal()
		pm.loopV = md.IsLoopVertical()
		layers := len(md.Data) / (md.Width * md.Height)

		// RMMV direction bits: bit0=down(2), bit1=left(4), bit2=right(6), bit3=up(8)
		dirBits := [4]int{0x01, 0x02, 0x04, 0x08}

		// Only check tile layers 0-3, matching RMMV's layeredTiles().
		// Layer 4 = shadows, layer 5 = regions — NOT real tiles.
		tileLayers := layers
		if tileLayers > 4 {
			tileLayers = 4
		}

		for y := 0; y < md.Height; y++ {
			for x := 0; x < md.Width; x++ {
				// Check each direction independently, matching RMMV's checkPassage.
				for di, bit := range dirBits {
					passable := false
					// Iterate layers from top (3) to bottom (0), matching RMMV's
					// layeredTiles() which returns [z=3, z=2, z=1, z=0].
					// The FIRST non-star tile encountered determines passability.
					for layer := tileLayers - 1; layer >= 0; layer-- {
						tileID := md.Data[layer*md.Height*md.Width+y*md.Width+x]
						if tileID < 0 || tileID >= len(ts.Flags) {
							continue
						}
						flag := ts.Flags[tileID]
						if flag&0x10 != 0 {
							// Star tile.
							if cpStarFix {
								// CP_Star_Passability_Fix: star tiles CAN block.
								// If direction bit is clear → passable, check next layer.
								// If direction bit is set → BLOCKED.
								if (flag & bit) == 0 {
									continue
								}
								passable = false
								break
							}
							// Base RMMV: star tiles are always skipped.
							continue
						}
						// First non-star tile determines passability.
						passable = (flag & bit) == 0
						break
					}
					pm.data[y][x][di] = passable
				}
			}
		}
		rl.extractRegions(pm, md)
		rl.Passability[mapID] = pm
	}
}

// extractRegions reads region IDs from layer 5 of the map data into the PassabilityMap.
func (rl *ResourceLoader) extractRegions(pm *PassabilityMap, md *MapData) {
	if md.Width <= 0 || md.Height <= 0 || len(md.Data) == 0 {
		return
	}
	layers := len(md.Data) / (md.Width * md.Height)
	if layers < 6 {
		return // no region layer
	}
	pm.regions = make([][]int, md.Height)
	for y := 0; y < md.Height; y++ {
		pm.regions[y] = make([]int, md.Width)
		for x := 0; x < md.Width; x++ {
			pm.regions[y][x] = md.Data[5*md.Height*md.Width+y*md.Width+x]
		}
	}
}

// buildIncomingTransfers scans all maps for transfer commands (code 201) and
// records the destination coordinates grouped by destination map ID. This builds
// a reverse index: for each map, where do players arrive from other maps?
func (rl *ResourceLoader) buildIncomingTransfers() {
	rl.IncomingTransfers = make(map[int][]EntryPoint)
	seen := make(map[int]map[[2]int]bool) // destMapID → set of (x,y) already added
	for _, md := range rl.Maps {
		if md == nil {
			continue
		}
		for _, ev := range md.Events {
			if ev == nil {
				continue
			}
			for _, page := range ev.Pages {
				if page == nil {
					continue
				}
				for _, cmd := range page.List {
					if cmd == nil || cmd.Code != 201 || len(cmd.Parameters) < 5 {
						continue
					}
					mode := paramIntP(cmd.Parameters, 0)
					if mode != 0 {
						continue // skip variable-based transfers
					}
					destMap := paramIntP(cmd.Parameters, 1)
					destX := paramIntP(cmd.Parameters, 2)
					destY := paramIntP(cmd.Parameters, 3)
					if destMap <= 0 {
						continue
					}
					if seen[destMap] == nil {
						seen[destMap] = make(map[[2]int]bool)
					}
					key := [2]int{destX, destY}
					if seen[destMap][key] {
						continue
					}
					seen[destMap][key] = true
					rl.IncomingTransfers[destMap] = append(rl.IncomingTransfers[destMap], EntryPoint{X: destX, Y: destY})
				}
			}
		}
	}
}

// ClassByID returns the Class with the given ID, or nil.
func (rl *ResourceLoader) ClassByID(id int) *Class {
	for _, c := range rl.Classes {
		if c != nil && c.ID == id {
			return c
		}
	}
	return nil
}

// SkillsForLevel returns skill IDs a class learns at or below the given level.
func (rl *ResourceLoader) SkillsForLevel(classID, level int) []int {
	cls := rl.ClassByID(classID)
	if cls == nil {
		return nil
	}
	var ids []int
	for _, l := range cls.Learnings {
		if l.Level <= level {
			ids = append(ids, l.SkillID)
		}
	}
	return ids
}

// SkillByID returns the Skill with the given ID, or nil.
func (rl *ResourceLoader) SkillByID(id int) *Skill {
	for _, s := range rl.Skills {
		if s != nil && s.ID == id {
			return s
		}
	}
	return nil
}

// ValidWalkName checks that the given walk character sheet name exists in img/characters/.
func (rl *ResourceLoader) ValidWalkName(name string) bool {
	if rl.ImgPath == "" {
		return true // no img path configured, skip check
	}
	path := filepath.Join(rl.ImgPath, "characters", name+".png")
	_, err := os.Stat(path)
	return err == nil
}

// ValidFaceName checks that the given face sheet name exists in img/faces/.
// If the faces directory doesn't exist, falls back to checking img/characters/.
func (rl *ResourceLoader) ValidFaceName(name string) bool {
	if rl.ImgPath == "" {
		return true
	}
	facesDir := filepath.Join(rl.ImgPath, "faces")
	if _, err := os.Stat(facesDir); os.IsNotExist(err) {
		// No faces directory — accept character sprite names instead
		return rl.ValidWalkName(name)
	}
	path := filepath.Join(facesDir, name+".png")
	_, err := os.Stat(path)
	return err == nil
}
