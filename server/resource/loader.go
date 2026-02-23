package resource

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
}

// SkillDamage holds the damage formula and element from RMMV Skills.json.
type SkillDamage struct {
	Formula   string `json:"formula"`
	ElementID int    `json:"elementId"`
	Type      int    `json:"type"` // 0=none,1=HP dmg,2=MP dmg,3=HP rec,4=MP rec
}

type Skill struct {
	ID        int         `json:"id"`
	Name      string      `json:"name"`
	MPCost    int         `json:"mpCost"`
	IconIndex int         `json:"iconIndex"`
	Damage    SkillDamage `json:"damage"`
}

type Item struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Price     int    `json:"price"`
	Consumable bool  `json:"consumable"`
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
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Price   int    `json:"price"`
	Params  []int  `json:"params"`
	WtypeID int    `json:"wtypeId"`
}

type Armor struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Price   int    `json:"price"`
	Params  []int  `json:"params"`
	EtypeID int    `json:"etypeId"` // 1=shield,2=helmet,3=body,4=accessory
	AtypeID int    `json:"atypeId"`
}

type Enemy struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	HP     int    `json:"mhp"`
	MP     int    `json:"mmp"`
	Atk    int    `json:"atk"`
	Def    int    `json:"def"`
	Mat    int    `json:"mat"`
	Mdf    int    `json:"mdf"`
	Agi    int    `json:"agi"`
	Luk    int    `json:"luk"`
	Exp       int          `json:"exp"`
	Gold      int          `json:"gold"`
	DropItems []EnemyDrop  `json:"dropItems"`
}

// EnemyDrop represents one entry in the RMMV enemy drop table.
type EnemyDrop struct {
	Kind        int `json:"kind"`        // 1=Item 2=Weapon 3=Armor
	DataID      int `json:"dataId"`      // ID within Items/Weapons/Armors.json
	Denominator int `json:"denominator"` // Drop probability = 1/denominator
}

type Troop struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Members []struct {
		EnemyID int `json:"enemyId"`
		X       int `json:"x"`
		Y       int `json:"y"`
	} `json:"members"`
}

type State struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	IconIndex int    `json:"iconIndex"`
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

// MapData represents an RMMV Map*.json file.
type MapData struct {
	ID          int         `json:"id"` // set after load from filename
	DisplayName string      `json:"displayName"`
	Width       int         `json:"width"`
	Height      int         `json:"height"`
	Data        []int       `json:"data"` // tileId array: [layer * height * width + y * width + x]
	TilesetID   int         `json:"tilesetId"`
	Events      []*MapEvent `json:"events"` // nil entries are possible (RMMV uses 1-based IDs)
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

// CanPass reports whether movement in the given RPG Maker direction (2/4/6/8) is allowed at (x,y).
func (pm *PassabilityMap) CanPass(x, y, dir int) bool {
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
func (pm *PassabilityMap) RegionAt(x, y int) int {
	if pm.regions == nil || x < 0 || x >= pm.Width || y < 0 || y >= pm.Height {
		return 0
	}
	return pm.regions[y][x]
}

// RegionRestrictions stores the region-based movement restriction config
// parsed from the YEP_RegionRestrictions plugin.
type RegionRestrictions struct {
	EventRestrict []int // regions that block event/NPC movement
	AllRestrict   []int // regions that block all movement
	EventAllow    []int // regions that always allow event movement
	AllAllow      []int // regions that always allow all movement
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
	return nil
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
	return err
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
			rl.extractRegions(pm, md)
			rl.Passability[mapID] = pm
			continue
		}

		pm := newPassabilityMap(md.Width, md.Height)
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
					// Iterate layers from bottom (0) to top (3), matching RMMV's
					// layeredTiles() order in checkPassage. The FIRST non-star
					// tile encountered determines passability for that direction.
					for layer := 0; layer < tileLayers; layer++ {
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
