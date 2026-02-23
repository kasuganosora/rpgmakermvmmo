package quest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kasuganosora/rpgmakermvmmo/server/game/player"
	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ObjectiveType categorizes a quest objective.
type ObjectiveType string

const (
	ObjectiveKill    ObjectiveType = "kill"
	ObjectiveCollect ObjectiveType = "collect"
	ObjectiveGoto    ObjectiveType = "goto"
	ObjectiveTalk    ObjectiveType = "talk"
)

// Objective describes one requirement within a quest.
type Objective struct {
	Type     ObjectiveType `json:"type"`
	TargetID int           `json:"target_id"`
	Count    int           `json:"count"`
	Label    string        `json:"label,omitempty"`
}

// RewardItem is a single item reward entry.
type RewardItem struct {
	ItemType int `json:"item_type"`
	ItemID   int `json:"item_id"`
	Qty      int `json:"qty"`
}

// QuestDef is a quest definition.
type QuestDef struct {
	ID          int          `json:"id"`
	Name        string       `json:"name"`
	Objectives  []Objective  `json:"objectives"`
	RewardExp   int          `json:"reward_exp"`
	RewardGold  int64        `json:"reward_gold"`
	RewardItems []RewardItem `json:"reward_items"`
}

// Service handles all quest operations.
type Service struct {
	db     *gorm.DB
	defs   map[int]*QuestDef
	logger *zap.Logger
}

// NewService creates a new quest Service with the given quest definitions.
func NewService(db *gorm.DB, defs map[int]*QuestDef, logger *zap.Logger) *Service {
	if defs == nil {
		defs = make(map[int]*QuestDef)
	}
	return &Service{db: db, defs: defs, logger: logger}
}

// AcceptQuest adds a quest to the player's active quest list.
func (svc *Service) AcceptQuest(ctx context.Context, s *player.PlayerSession, questID int) error {
	def, ok := svc.defs[questID]
	if !ok {
		return nil
	}

	var existing model.QuestProgress
	if err := svc.db.WithContext(ctx).Where("char_id = ? AND quest_id = ?", s.CharID, questID).
		First(&existing).Error; err == nil {
		return nil // already accepted
	}

	emptyProgress, _ := json.Marshal(map[string]int{})
	now := time.Now()
	progress := &model.QuestProgress{
		CharID:     s.CharID,
		QuestID:    questID,
		Progress:   datatypes.JSON(emptyProgress),
		Status:     0,
		AcceptedAt: now,
	}
	if err := svc.db.WithContext(ctx).Create(progress).Error; err != nil {
		return err
	}
	svc.sendUpdate(s, def, map[string]int{}, false)
	return nil
}

// OnMonsterKill updates kill-type quest objectives.
func (svc *Service) OnMonsterKill(ctx context.Context, s *player.PlayerSession, monsterID int) {
	svc.updateObjective(ctx, s, ObjectiveKill, monsterID)
}

// OnItemGain updates collect-type quest objectives.
func (svc *Service) OnItemGain(ctx context.Context, s *player.PlayerSession, itemID int) {
	svc.updateObjective(ctx, s, ObjectiveCollect, itemID)
}

// OnMapEnter updates goto-type quest objectives.
func (svc *Service) OnMapEnter(ctx context.Context, s *player.PlayerSession, mapID int) {
	svc.updateObjective(ctx, s, ObjectiveGoto, mapID)
}

func (svc *Service) updateObjective(ctx context.Context, s *player.PlayerSession, objType ObjectiveType, targetID int) {
	var quests []model.QuestProgress
	svc.db.WithContext(ctx).Where("char_id = ? AND status = 0", s.CharID).Find(&quests)

	for i := range quests {
		qp := &quests[i]
		def, ok := svc.defs[qp.QuestID]
		if !ok {
			continue
		}
		progress := make(map[string]int)
		_ = json.Unmarshal(qp.Progress, &progress)

		changed := false
		for j, obj := range def.Objectives {
			if obj.Type != objType || obj.TargetID != targetID {
				continue
			}
			key := progressKey(obj, j)
			current := progress[key]
			if current >= obj.Count {
				continue
			}
			progress[key] = current + 1
			changed = true
		}
		if !changed {
			continue
		}

		progressJSON, _ := json.Marshal(progress)
		qp.Progress = datatypes.JSON(progressJSON)

		completed := true
		for j, obj := range def.Objectives {
			if progress[progressKey(obj, j)] < obj.Count {
				completed = false
				break
			}
		}
		if completed {
			qp.Status = 1
		}
		svc.db.WithContext(ctx).Save(qp)
		svc.sendUpdate(s, def, progress, completed)
		if completed {
			svc.grantRewards(ctx, s, def)
		}
	}
}

func progressKey(obj Objective, idx int) string {
	return fmt.Sprintf("%s_%d_%d", obj.Type, obj.TargetID, idx)
}

func (svc *Service) sendUpdate(s *player.PlayerSession, def *QuestDef, progress map[string]int, completed bool) {
	objectives := make([]map[string]interface{}, len(def.Objectives))
	for i, obj := range def.Objectives {
		objectives[i] = map[string]interface{}{
			"type":      obj.Type,
			"target_id": obj.TargetID,
			"label":     obj.Label,
			"current":   progress[progressKey(obj, i)],
			"required":  obj.Count,
		}
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"quest_id":   def.ID,
		"name":       def.Name,
		"objectives": objectives,
		"completed":  completed,
	})
	s.Send(&player.Packet{Type: "quest_update", Payload: payload})
}

func (svc *Service) grantRewards(ctx context.Context, s *player.PlayerSession, def *QuestDef) {
	svc.logger.Info("quest completed",
		zap.Int64("char_id", s.CharID),
		zap.Int("quest_id", def.ID))
	go func() {
		svc.db.Model(&model.Character{}).Where("id = ?", s.CharID).
			Updates(map[string]interface{}{
				"exp":  gorm.Expr("exp + ?", def.RewardExp),
				"gold": gorm.Expr("gold + ?", def.RewardGold),
			})
	}()
}
