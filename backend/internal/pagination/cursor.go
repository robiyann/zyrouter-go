package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// Cursor is an opaque keyset position. Timestamp and ID together provide a
// deterministic boundary even when multiple rows share the same timestamp.
type Cursor struct {
	Timestamp string `json:"t"`
	ID        int64  `json:"i"`
}

func Encode(timestamp string, id int64) string {
	payload, _ := json.Marshal(Cursor{Timestamp: timestamp, ID: id})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func Decode(raw string) (Cursor, error) {
	if strings.TrimSpace(raw) == "" {
		return Cursor{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return Cursor{}, fmt.Errorf("invalid cursor")
	}
	var cursor Cursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || strings.TrimSpace(cursor.Timestamp) == "" || cursor.ID < 1 {
		return Cursor{}, fmt.Errorf("invalid cursor")
	}
	return cursor, nil
}
