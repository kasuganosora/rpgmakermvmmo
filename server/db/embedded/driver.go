package embedded

import (
	"context"
	"fmt"
	"os"

	sqlexecapi "github.com/kasuganosora/sqlexec/pkg/api"
	sqlexecgorm "github.com/kasuganosora/sqlexec/pkg/api/gorm"
	"github.com/kasuganosora/sqlexec/pkg/resource/domain"
	"github.com/kasuganosora/sqlexec/pkg/resource/memory"
	sqlexecxml "github.com/kasuganosora/sqlexec/pkg/resource/xml"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type EngineType int

const (
	EngineXML    EngineType = iota
	EngineMemory EngineType = iota
)

// Open creates a GORM *DB backed by the sqlexec engine.
// For EngineXML, dataPath specifies the XML storage directory (created if absent).
// For EngineMemory, dataPath is ignored.
func Open(dataPath string, eng EngineType) (*gorm.DB, error) {
	db, err := sqlexecapi.NewDB(&sqlexecapi.DBConfig{
		DebugMode:            false,
		UseEnhancedOptimizer: true,
	})
	if err != nil {
		return nil, fmt.Errorf("embedded: NewDB: %w", err)
	}

	var ds domain.DataSource
	switch eng {
	case EngineMemory:
		cfg := &domain.DataSourceConfig{
			Type:     domain.DataSourceTypeMemory,
			Name:     "default",
			Writable: true,
		}
		ds = memory.NewMVCCDataSource(cfg)

	case EngineXML:
		if err := os.MkdirAll(dataPath, 0755); err != nil {
			return nil, fmt.Errorf("embedded: create XML data dir %q: %w", dataPath, err)
		}
		cfg := &domain.DataSourceConfig{
			Type:     domain.DataSourceTypeXML,
			Name:     "default",
			Writable: true,
			Database: dataPath,
		}
		factory := sqlexecxml.NewXMLFactory()
		ds, err = factory.Create(cfg)
		if err != nil {
			return nil, fmt.Errorf("embedded: XMLFactory.Create: %w", err)
		}

	default:
		return nil, fmt.Errorf("embedded: unknown engine type %d", eng)
	}

	// Connect the datasource before use.
	if err := ds.Connect(context.Background()); err != nil {
		return nil, fmt.Errorf("embedded: datasource Connect: %w", err)
	}

	if err := db.RegisterDataSource("default", ds); err != nil {
		return nil, fmt.Errorf("embedded: RegisterDataSource: %w", err)
	}

	dialector := sqlexecgorm.NewDialector(db.Session())
	return gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}
