package database

import (
	"io/fs"
	"strings"
	"testing"
)

func TestMigrationsDoNotDropExistingTables(t *testing.T) {
	files, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected embedded migrations")
	}
	combined := strings.Builder{}
	for _, name := range files {
		content, err := migrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		upper := strings.ToUpper(string(content))
		if strings.Contains(upper, "DROP TABLE") || strings.Contains(upper, "TRUNCATE TABLE") {
			t.Fatalf("destructive SQL found in %s", name)
		}
		combined.Write(content)
	}
	if !strings.Contains(combined.String(), "CREATE TABLE IF NOT EXISTS ai_user") {
		t.Fatal("expected safe ai_user create-if-missing migration")
	}
}
