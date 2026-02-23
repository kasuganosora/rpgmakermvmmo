package model

import "gorm.io/gorm"

// allModels lists every model to be auto-migrated.
var allModels = []interface{}{
	&Account{},
	&Character{},
	&Inventory{},
	&CharSkill{},
	&QuestProgress{},
	&Friendship{},
	&Guild{},
	&GuildMember{},
	&Mail{},
	&AuditLog{},
}

// AutoMigrate creates or updates all tables in the given database.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(allModels...)
}
