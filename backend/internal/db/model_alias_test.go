package db

import "testing"

func TestModelAliasRegistryStoresOnePublicMapping(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := NewRepo(database)
	connectionID := "conn-google-1"
	if err := repo.SetModelAliasRecord("fast-gemini", "google", "gemini-2.5-pro", &connectionID, []string{"chat", "vision"}, 1); err != nil {
		t.Fatal(err)
	}

	target, err := repo.GetModelAlias("fast-gemini")
	if err != nil || target != "google/gemini-2.5-pro" {
		t.Fatalf("unexpected alias target: %q err=%v", target, err)
	}
	records, err := repo.GetModelAliasRecords()
	if err != nil || len(records) != 1 || records[0].ConnectionID == nil || *records[0].ConnectionID != connectionID || len(records[0].Capabilities) != 2 {
		t.Fatalf("unexpected alias records: %+v err=%v", records, err)
	}

	if err := repo.SetModelAliasRecord("fast-gemini", "google", "gemini-2.5-pro", nil, nil, 0); err != nil {
		t.Fatal(err)
	}
	target, err = repo.GetModelAlias("fast-gemini")
	if err != nil || target != "" {
		t.Fatalf("disabled alias is still active: %q err=%v", target, err)
	}
	aliases, err := repo.GetModelAliases()
	if err != nil || len(aliases) != 0 {
		t.Fatalf("disabled alias leaked into public map: %+v err=%v", aliases, err)
	}
	if err := repo.SetModelAlias("bad/alias", "google/model"); err == nil {
		t.Fatal("provider-prefixed public alias was accepted")
	}
}
