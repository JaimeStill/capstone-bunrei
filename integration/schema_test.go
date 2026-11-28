//go:build integration

package integration_test

import (
	"context"
	"testing"

	"github.com/JaimeStill/capstone-bunrei/integration"
)

func TestSchema_StartupAppliesTheLibraryAndBunreiSets(t *testing.T) {
	s := integration.Start(t, integration.Options{})
	assertReady(t, s.Client())

	db := integration.Database(t)
	tables := []string{
		"docproc_schema_version", "docproc_entry", "docproc_item",
		"docinfer_schema_version", "docinfer_cache",
		"messaging_schema_version", "messaging_outbox", "messaging_inbox",
		"schema_version", "upload", "upload_chunk", "submission", "completed_upload",
	}
	for _, table := range tables {
		var exists bool
		if err := db.QueryRowContext(context.Background(), "SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); err != nil {
			t.Fatalf("look up %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %s does not exist after startup", table)
		}
	}

	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
}
