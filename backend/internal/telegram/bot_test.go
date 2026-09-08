package telegram

import (
	"testing"
)

func TestExtractChallengeCode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/start 12345-abcde", "12345-abcde"},
		{"/start", ""},
		{"/verify 67890-fghij", "67890-fghij"},
		{"/verify", ""},
		{"  ZY-ABCD-1234  ", "ZY-ABCD-1234"},
		{"", ""},
		{"   ", ""},
	}

	for _, tc := range tests {
		got := ExtractChallengeCode(tc.input)
		if got != tc.expected {
			t.Errorf("ExtractChallengeCode(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}
