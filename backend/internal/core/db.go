package core

import (
	glebarezsqlite "github.com/glebarez/sqlite"
	gormmysql "gorm.io/driver/mysql"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// OpenDB opens the configured database: sqlite (pure-Go, local dev), mysql
// or postgres (production). The DSN formats are the usual ones, e.g.
//
//	sqlite:   /data/mailez.db
//	mysql:    user:pass@tcp(host:3306)/mailez?charset=utf8mb4&parseTime=True&loc=Local
//	postgres: postgres://user:pass@host:5432/mailez?sslmode=disable
//
// SingularTable keeps table names aligned with the model names on all
// engines.
func OpenDB(driver, dsn, logLevel string) (*gorm.DB, error) {
	level := logger.Warn
	if logLevel == "debug" || logLevel == "trace" {
		level = logger.Info
	}
	var dialector gorm.Dialector
	switch driver {
	case "mysql":
		dialector = gormmysql.Open(dsn)
	case "postgres":
		dialector = gormpostgres.Open(dsn)
	default:
		// WAL + a generous busy timeout: the control plane runs background
		// writers (outbox flush, notifier, reminder sweeps, uploads cleanup)
		// next to API reads; the driver defaults (journal_mode=DELETE,
		// busy_timeout=5000) surface sporadic "database is locked" 500s
		// once a write burst outlives the 5s window.
		dsn = dsn + "_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=synchronous(NORMAL)"
		dialector = glebarezsqlite.Open(dsn)
	}
	return gorm.Open(dialector, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
		Logger:         logger.Default.LogMode(level),
	})
}
