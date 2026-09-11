package db

import (
	"encoding/json"
	"os"
	"testing"
)

func TestUpdateProviderConnectionOAuthTokensPreservesConcurrentFields(t *testing.T) {
	file, err := os.CreateTemp("", "oauth-token-update-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	defer os.Remove(file.Name())

	database, err := OpenDatabase(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	_, err = database.Exec(`INSERT INTO providerConnections
		(id, provider, authType, isActive, data, createdAt, updatedAt)
		VALUES ('ag-token', 'antigravity', 'oauth', 1, ?, '2030-01-01T00:00:00Z', '2030-01-01T00:00:00Z')`,
		`{"accessToken":"old","refreshToken":"refresh","modelLock_gemini-3.8-flash-high":"2030-01-02T00:00:00Z","backoffLevel":3}`)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewRepo(database).UpdateProviderConnectionOAuthTokens("ag-token", "new", "", "2030-01-03T00:00:00Z"); err != nil {
		t.Fatal(err)
	}

	var raw string
	if err := database.QueryRow("SELECT data FROM providerConnections WHERE id = 'ag-token'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatal(err)
	}
	if data["accessToken"] != "new" || data["refreshToken"] != "refresh" || data["modelLock_gemini-3.8-flash-high"] == nil || data["backoffLevel"] != float64(3) {
		t.Fatalf("token update overwrote concurrent fields: %s", raw)
	}
}
