package proxy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultOpenCodeUA = "opencode/1.18.31"
	Base62Chars       = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

var (
	OpenCodeSessionRegex = regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)
	OpenCodeRequestRegex = regexp.MustCompile(`^msg_[0-9a-f]{12}[0-9A-Za-z]{14}$`)
	opencodeUaRegex      = regexp.MustCompile(`(?i)opencode/(\d+)\.(\d+)(?:\.(\d+))?`)

	opencodeMu            sync.Mutex
	lastOpencodeTimestamp int64
	opencodeCounter       int64
)

func generateRandomBase62(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	res := make([]byte, n)
	for i := 0; i < n; i++ {
		res[i] = Base62Chars[int(b[i])%len(Base62Chars)]
	}
	return string(res)
}

// GenerateOpenCodeSessionID generates a canonical descending session identifier.
func GenerateOpenCodeSessionID() string {
	opencodeMu.Lock()
	now := time.Now().UnixMilli()
	if now != lastOpencodeTimestamp {
		lastOpencodeTimestamp = now
		opencodeCounter = 0
	}
	opencodeCounter++
	cnt := opencodeCounter
	opencodeMu.Unlock()

	current := uint64(now)*0x1000 + uint64(cnt)
	value := ^current
	timeHex := fmt.Sprintf("%012x", value&0xffffffffffff)
	return "ses_" + timeHex + generateRandomBase62(14)
}

// GenerateOpenCodeRequestID generates a canonical request identifier.
func GenerateOpenCodeRequestID() string {
	now := time.Now().UnixMilli()
	current := uint64(now)*0x1000 + 1
	timeHex := fmt.Sprintf("%012x", current&0xffffffffffff)
	return "msg_" + timeHex + generateRandomBase62(14)
}

// TranslateOpenCodeSessionID converts any input session string into a valid canonical format.
func TranslateOpenCodeSessionID(raw string) string {
	raw = strings.TrimSpace(raw)
	if OpenCodeSessionRegex.MatchString(raw) {
		return raw
	}
	if raw == "" {
		return GenerateOpenCodeSessionID()
	}
	h := sha256.Sum256([]byte("opencode:generic:" + raw))
	timeHex := hex.EncodeToString(h[:6])
	res := make([]byte, 14)
	for i := 6; i < 20; i++ {
		res[i-6] = Base62Chars[int(h[i])%len(Base62Chars)]
	}
	return "ses_" + timeHex + string(res)
}

// HasValidOpenCodeVersion checks if User-Agent has opencode/version >= 1.17.0.
func HasValidOpenCodeVersion(ua string) bool {
	matches := opencodeUaRegex.FindStringSubmatch(ua)
	if len(matches) < 3 {
		return false
	}
	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	return major > 1 || (major == 1 && minor >= 17)
}

// BuildOpenCodeHeaders generates the official OpenCode fingerprint headers to prevent 403 FreeTierError / 429 rate limits.
func BuildOpenCodeHeaders(rawHeaders map[string]string, sessionID string, isStream bool) map[string]string {
	ua := DefaultOpenCodeUA
	if rawHeaders != nil {
		for k, v := range rawHeaders {
			if strings.EqualFold(k, "user-agent") {
				if HasValidOpenCodeVersion(v) {
					ua = v
				}
				break
			}
		}
	}

	session := TranslateOpenCodeSessionID(sessionID)
	if rawHeaders != nil {
		for k, v := range rawHeaders {
			if strings.EqualFold(k, "x-opencode-session") {
				session = TranslateOpenCodeSessionID(v)
				break
			}
		}
	}

	res := map[string]string{
		"Content-Type":       "application/json",
		"Authorization":      "Bearer public",
		"User-Agent":         ua,
		"x-opencode-client":  "desktop",
		"x-opencode-session": session,
		"x-opencode-request": GenerateOpenCodeRequestID(),
		"x-opencode-project": "global",
	}
	if isStream {
		res["Accept"] = "text/event-stream"
	} else {
		res["Accept"] = "*/*"
	}
	for k, v := range rawHeaders {
		kl := strings.ToLower(k)
		if kl == "user-agent" || kl == "x-opencode-session" || kl == "x-opencode-request" || kl == "x-opencode-client" || kl == "x-opencode-project" || kl == "authorization" || kl == "content-type" || kl == "accept" {
			continue
		}
		res[k] = v
	}
	return res
}
