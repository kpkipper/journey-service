package testutil

import (
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/kpkipper/journey-service/database"
	"gorm.io/gorm"
)

// NewTestDB opens an isolated in-memory SQLite database (one per test) and migrates all models.
func NewTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := strings.ReplaceAll(t.Name(), "/", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(1)", name)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	return db
}
