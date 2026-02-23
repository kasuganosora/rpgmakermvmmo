package quest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func nopLogger() *zap.Logger { l, _ := zap.NewDevelopment(); return l }

// newSession creates a minimal PlayerSession for testing (no WebSocket).
func newSession(charID int64) *player.PlayerSession {
	return &player.PlayerSession{
		CharID:   charID,
		CharName: "TestPlayer",
		SendChan: make(chan []byte, 256),
		Done:     make(chan struct{}),
	}
}

func testDefs() map[int]*QuestDef {
	return map[int]*QuestDef{
		1: {
			ID:   1,
			Name: "Slay Slimes",
			Objectives: []Objective{
				{Type: ObjectiveKill, TargetID: 10, Count: 3, Label: "Kill Slimes"},
			},
			RewardExp:  100,
			RewardGold: 50,
		},
		2: {
			ID:   2,
			Name: "Collect Herbs",
			Objectives: []Objective{
				{Type: ObjectiveCollect, TargetID: 5, Count: 2, Label: "Collect Herb"},
			},
			RewardExp: 50,
		},
		3: {
			ID:   3,
			Name: "Explore Cave",
			Objectives: []Objective{
				{Type: ObjectiveGoto, TargetID: 7, Count: 1, Label: "Enter Cave"},
			},
		},
	}
}

func TestNewService_NilDefs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db, nil, nopLogger())
	require.NotNil(t, svc)
	assert.NotNil(t, svc.defs)
}

func TestAcceptQuest_UnknownQuestID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db, testDefs(), nopLogger())
	s := newSession(1)
	err := svc.AcceptQuest(context.Background(), s, 999)
	assert.NoError(t, err)
}

func TestAcceptQuest_Success(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db, testDefs(), nopLogger())
	s := newSession(10)

	err := svc.AcceptQuest(context.Background(), s, 1)
	require.NoError(t, err)

	var qp model.QuestProgress
	require.NoError(t, db.Where("char_id = ? AND quest_id = ?", int64(10), 1).First(&qp).Error)
	assert.Equal(t, 0, qp.Status)

	// Player should receive a quest_update packet
	select {
	case msg := <-s.SendChan:
		var pkt player.Packet
		require.NoError(t, json.Unmarshal(msg, &pkt))
		assert.Equal(t, "quest_update", pkt.Type)
	default:
		t.Fatal("expected quest_update packet")
	}
}

func TestAcceptQuest_AlreadyAccepted(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db, testDefs(), nopLogger())
	s := newSession(10)

	require.NoError(t, svc.AcceptQuest(context.Background(), s, 1))
	<-s.SendChan

	// Accept again → idempotent, no duplicate
	err := svc.AcceptQuest(context.Background(), s, 1)
	assert.NoError(t, err)

	// Use Find instead of Count — sqlexec does not implement COUNT(*) correctly.
	var rows []model.QuestProgress
	db.Where("char_id = ? AND quest_id = ?", int64(10), 1).Find(&rows)
	assert.Len(t, rows, 1)
}

func TestOnMonsterKill_IncreasesProgress(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db, testDefs(), nopLogger())
	s := newSession(20)

	require.NoError(t, svc.AcceptQuest(context.Background(), s, 1))
	<-s.SendChan

	svc.OnMonsterKill(context.Background(), s, 10)

	var qp model.QuestProgress
	require.NoError(t, db.Where("char_id = ? AND quest_id = ?", int64(20), 1).First(&qp).Error)

	progress := make(map[string]int)
	require.NoError(t, json.Unmarshal(qp.Progress, &progress))
	key := progressKey(testDefs()[1].Objectives[0], 0)
	assert.Equal(t, 1, progress[key])
	assert.Equal(t, 0, qp.Status) // not yet completed
}

func TestOnMonsterKill_Completion(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db, testDefs(), nopLogger())
	s := newSession(30)

	char := &model.Character{ID: 30, Name: "Hero", AccountID: 1, HP: 100, MaxHP: 100}
	db.Create(char)

	require.NoError(t, svc.AcceptQuest(context.Background(), s, 1))
	<-s.SendChan

	for i := 0; i < 3; i++ {
		svc.OnMonsterKill(context.Background(), s, 10)
		select {
		case <-s.SendChan:
		default:
		}
	}

	var qp model.QuestProgress
	require.NoError(t, db.Where("char_id = ? AND quest_id = ?", int64(30), 1).First(&qp).Error)
	assert.Equal(t, 1, qp.Status) // completed
}

func TestOnMonsterKill_WrongMonster(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db, testDefs(), nopLogger())
	s := newSession(40)

	require.NoError(t, svc.AcceptQuest(context.Background(), s, 1))
	<-s.SendChan

	svc.OnMonsterKill(context.Background(), s, 99) // wrong ID

	var qp model.QuestProgress
	require.NoError(t, db.Where("char_id = ? AND quest_id = ?", int64(40), 1).First(&qp).Error)

	progress := make(map[string]int)
	json.Unmarshal(qp.Progress, &progress)
	key := progressKey(testDefs()[1].Objectives[0], 0)
	assert.Equal(t, 0, progress[key])
}

func TestOnItemGain_Progress(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db, testDefs(), nopLogger())
	s := newSession(50)

	require.NoError(t, svc.AcceptQuest(context.Background(), s, 2))
	<-s.SendChan

	svc.OnItemGain(context.Background(), s, 5)

	var qp model.QuestProgress
	require.NoError(t, db.Where("char_id = ? AND quest_id = ?", int64(50), 2).First(&qp).Error)

	progress := make(map[string]int)
	json.Unmarshal(qp.Progress, &progress)
	key := progressKey(testDefs()[2].Objectives[0], 0)
	assert.Equal(t, 1, progress[key])
}

func TestOnMapEnter_Completion(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db, testDefs(), nopLogger())
	s := newSession(60)

	require.NoError(t, svc.AcceptQuest(context.Background(), s, 3))
	<-s.SendChan

	svc.OnMapEnter(context.Background(), s, 7)

	var qp model.QuestProgress
	require.NoError(t, db.Where("char_id = ? AND quest_id = ?", int64(60), 3).First(&qp).Error)
	assert.Equal(t, 1, qp.Status) // completed (count=1, needed=1)
}

func TestOnMapEnter_WrongMap(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewService(db, testDefs(), nopLogger())
	s := newSession(70)

	require.NoError(t, svc.AcceptQuest(context.Background(), s, 3))
	<-s.SendChan

	svc.OnMapEnter(context.Background(), s, 99) // wrong map

	var qp model.QuestProgress
	require.NoError(t, db.Where("char_id = ? AND quest_id = ?", int64(70), 3).First(&qp).Error)
	assert.Equal(t, 0, qp.Status)
}

func TestProgressKey_Format(t *testing.T) {
	obj := Objective{Type: ObjectiveKill, TargetID: 10, Count: 3}
	key := progressKey(obj, 0)
	assert.Equal(t, "kill_10_0", key)
}

func TestObjectiveTypes(t *testing.T) {
	assert.Equal(t, ObjectiveType("kill"), ObjectiveKill)
	assert.Equal(t, ObjectiveType("collect"), ObjectiveCollect)
	assert.Equal(t, ObjectiveType("goto"), ObjectiveGoto)
	assert.Equal(t, ObjectiveType("talk"), ObjectiveTalk)
}
