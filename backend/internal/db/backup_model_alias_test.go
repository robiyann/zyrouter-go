package db

import "testing"

func TestModelAliasMetadataSurvivesBackupRoundTrip(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := NewRepo(database)
	connectionID := "conn-1"
	if err := repo.SetModelAliasRecord("fast-chat", "openai", "gpt-4.1", &connectionID, []string{"chat", "tools"}, 1); err != nil {
		t.Fatal(err)
	}

	backup, err := repo.ExportDB()
	if err != nil || len(backup.ModelAliasRecords) != 1 {
		t.Fatalf("alias metadata was not exported: records=%+v err=%v", backup.ModelAliasRecords, err)
	}
	if err := repo.DeleteModelAlias("fast-chat"); err != nil {
		t.Fatal(err)
	}
	if err := repo.ImportDB(backup); err != nil {
		t.Fatal(err)
	}
	restored, err := repo.GetModelAliasRecord("fast-chat")
	if err != nil || restored == nil || restored.Provider != "openai" || restored.UpstreamModel != "gpt-4.1" || restored.ConnectionID == nil || *restored.ConnectionID != connectionID || len(restored.Capabilities) != 2 {
		t.Fatalf("alias metadata was not restored: %+v err=%v", restored, err)
	}
}
