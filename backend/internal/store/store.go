// Package store opens the GORM handle and migrates the schema.
// PostgreSQL is the production driver; SQLite (pure-Go, no cgo) exists so the
// API can be developed and smoke-tested on a machine without a database server.
package store

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/ysnb/oj/internal/config"
	"github.com/ysnb/oj/internal/external"
	"github.com/ysnb/oj/internal/model"
	"github.com/ysnb/oj/internal/public"
)

// Open connects to the configured database and migrates all entities.
func Open(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.Database.Driver {
	case "postgres":
		dialector = postgres.Open(cfg.Database.DSN)
	case "sqlite":
		dialector = sqlite.Open(cfg.Database.DSN)
	default:
		return nil, fmt.Errorf("unsupported db driver %q", cfg.Database.Driver)
	}
	logLevel := logger.Warn
	if cfg.Mode == "dev" {
		logLevel = logger.Silent
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	if err := db.AutoMigrate(migrationTargets()...); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return db, nil
}

func migrationTargets() []any {
	return []any{
		&model.User{},
		&model.InvitationCode{},
		&model.Problem{},
		&model.TestCase{},
		&model.ProblemSolution{},
		&model.Submission{},
		&model.Contest{},
		&model.ContestProblem{},
		&model.ContestUserFlag{},
		&model.ContestRegistration{},
		&model.ContestNotice{},
		&model.JudgeDaemon{},
		&model.ProblemList{},
		&model.ProblemListItem{},
		&model.Team{},
		&model.TeamMember{},
		&model.TeamNotice{},
		&model.TeamList{},
		&public.ApiKeyRecord{},
		&external.Record{},
		&external.Binding{},
	}
}
