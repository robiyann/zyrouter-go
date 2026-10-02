package handlerutil

import "testing"

func TestSanitizeClientIdentity(t *testing.T) {
	if got := SanitizeClientIdentity("zy_secret-client-key", "key-1"); got != "zy_secr...-key" {
		t.Fatalf("masked key = %q", got)
	}
	for _, identity := range []string{"@alice", "123456", "Dashboard Admin", "Local Loopback Client"} {
		if got := SanitizeClientIdentity(identity, "key-1"); got != identity {
			t.Fatalf("identity %q was changed to %q", identity, got)
		}
	}
}
