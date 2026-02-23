package rest_test

import (
	"testing"

	"github.com/kasuganosora/rpgmakermvmmo/server/model"
	"github.com/kasuganosora/rpgmakermvmmo/server/testutil"
)

func TestCountDebug(t *testing.T) {
	db := testutil.SetupTestDB(t)

	acc := &model.Account{Username: "countdebug", PasswordHash: "y", Status: 1}
	db.Create(acc)
	t.Logf("acc.ID = %d", acc.ID)

	char := &model.Character{AccountID: acc.ID, Name: "C1", ClassID: 1, HP: 100, MP: 50, MaxHP: 100, MaxMP: 50}
	err := db.Create(char).Error
	t.Logf("create char err: %v, id: %d", err, char.ID)

	var cnt int64
	err = db.Model(&model.Character{}).Where("account_id = ?", acc.ID).Count(&cnt).Error
	t.Logf("Count with Where int64: cnt=%d err=%v", cnt, err)

	err = db.Model(&model.Character{}).Count(&cnt).Error
	t.Logf("Count without Where: cnt=%d err=%v", cnt, err)

	// Try Find-based count
	var chars []model.Character
	err = db.Where("account_id = ?", acc.ID).Find(&chars).Error
	t.Logf("Find chars: len=%d err=%v", len(chars), err)
}
