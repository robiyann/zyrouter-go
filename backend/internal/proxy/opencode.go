package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultOpenCodeUA = "opencode/1.18.30"
	Base62Chars       = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

var (
	OpenCodeSessionRegex = regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)
	OpenCodeRequestRegex = regexp.MustCompile(`^msg_[0-9a-f]{12}[0-9A-Za-z]{14}$`)
	opencodeUaRegex      = regexp.MustCompile(`(?i)opencode/(\d+)\.(\d+)(?:\.(\d+))?`)

	opencodeMu            sync.Mutex
	lastOpencodeTimestamp int64
	opencodeCounter       int64

	ocSessionsMu sync.RWMutex
	ocSessions   = make(map[string]string)
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

func generateOpenCodeID(descending bool) string {
	opencodeMu.Lock()
	now := time.Now().UnixMilli()
	if now != lastOpencodeTimestamp {
		lastOpencodeTimestamp = now
		opencodeCounter = 0
	}
	opencodeCounter++
	cnt := opencodeCounter
	opencodeMu.Unlock()

	current := now*0x1000 + cnt
	value := current
	if descending {
		value = ^current
	}
	var timeBytes [6]byte
	for i := 0; i < 6; i++ {
		timeBytes[i] = byte((value >> uint(40-8*i)) & 0xff)
	}
	timeHex := hex.EncodeToString(timeBytes[:])
	return timeHex + generateRandomBase62(14)
}

// GenerateOpenCodeSessionID generates a canonical descending session identifier.
func GenerateOpenCodeSessionID() string {
	return "ses_" + generateOpenCodeID(true)
}

// GenerateOpenCodeRequestID generates a canonical request identifier.
func GenerateOpenCodeRequestID() string {
	return "msg_" + generateOpenCodeID(false)
}

// GetOrCreateOpenCodeSession retrieves or caches a session ID for the given key.
func GetOrCreateOpenCodeSession(key string) string {
	if key == "" {
		return GenerateOpenCodeSessionID()
	}
	ocSessionsMu.RLock()
	if s, ok := ocSessions[key]; ok {
		ocSessionsMu.RUnlock()
		return s
	}
	ocSessionsMu.RUnlock()

	s := GenerateOpenCodeSessionID()
	ocSessionsMu.Lock()
	if len(ocSessions) >= 1000 {
		for k := range ocSessions {
			delete(ocSessions, k)
			break
		}
	}
	ocSessions[key] = s
	ocSessionsMu.Unlock()
	return s
}

// TranslateOpenCodeSessionID converts an incoming session ID to OpenCode canonical format.
func TranslateOpenCodeSessionID(raw string) string {
	raw = strings.TrimSpace(raw)
	if OpenCodeSessionRegex.MatchString(raw) {
		return raw
	}
	return GetOrCreateOpenCodeSession(raw)
}

// HasValidOpenCodeVersion checks if a User-Agent string contains opencode >= 1.17.0.
func HasValidOpenCodeVersion(ua string) bool {
	matches := opencodeUaRegex.FindStringSubmatch(ua)
	if len(matches) < 3 {
		return false
	}
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return false
	}
	return major > 1 || (major == 1 && minor >= 17)
}

// BuildOpenCodeHeaders generates the mandatory headers for OpenCode upstream.
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

	sesHdr := ""
	if rawHeaders != nil {
		for k, v := range rawHeaders {
			if strings.EqualFold(k, "x-opencode-session") {
				sesHdr = strings.TrimSpace(v)
				break
			}
		}
	}

	var session string
	if sesHdr != "" && OpenCodeSessionRegex.MatchString(sesHdr) {
		session = sesHdr
	} else if sessionID != "" {
		session = TranslateOpenCodeSessionID(sessionID)
	} else {
		session = GenerateOpenCodeSessionID()
	}

	reqHdr := ""
	if rawHeaders != nil {
		for k, v := range rawHeaders {
			if strings.EqualFold(k, "x-opencode-request") {
				reqHdr = strings.TrimSpace(v)
				break
			}
		}
	}
	var reqId string
	if reqHdr != "" && OpenCodeRequestRegex.MatchString(reqHdr) {
		reqId = reqHdr
	} else {
		reqId = GenerateOpenCodeRequestID()
	}

	clientHdr := "desktop"
	if rawHeaders != nil {
		for k, v := range rawHeaders {
			if strings.EqualFold(k, "x-opencode-client") && strings.TrimSpace(v) != "" {
				clientHdr = strings.TrimSpace(v)
				break
			}
		}
	}

	projHdr := "global"
	if rawHeaders != nil {
		for k, v := range rawHeaders {
			if strings.EqualFold(k, "x-opencode-project") && strings.TrimSpace(v) != "" {
				projHdr = strings.TrimSpace(v)
				break
			}
		}
	}

	auth := "Bearer public"
	if rawHeaders != nil {
		for k, v := range rawHeaders {
			if strings.EqualFold(k, "authorization") && strings.TrimSpace(v) != "" {
				auth = strings.TrimSpace(v)
				break
			}
		}
	}

	res := map[string]string{
		"Content-Type":       "application/json",
		"Authorization":      auth,
		"anthropic-version":  "2023-06-01",
		"User-Agent":         ua,
		"x-opencode-client":  clientHdr,
		"x-opencode-session": session,
		"x-opencode-request": reqId,
		"x-opencode-project": projHdr,
	}

	if isStream {
		res["Accept"] = "text/event-stream"
	} else {
		res["Accept"] = "*/*"
	}

	for k, v := range rawHeaders {
		kl := strings.ToLower(k)
		if kl == "user-agent" || kl == "x-opencode-session" || kl == "x-opencode-request" || kl == "x-opencode-client" || kl == "x-opencode-project" || kl == "authorization" || kl == "content-type" || kl == "accept" || kl == "anthropic-version" {
			continue
		}
		res[k] = v
	}
	return res
}
