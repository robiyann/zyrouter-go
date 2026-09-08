package user

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"zyrouter/backend/internal/clientstream"
	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/middleware"
	"zyrouter/backend/internal/models"
)

type Handler struct{ Repo *db.Repo }

const verificationBrowserCookie = "verification_browser"

func edgeSecretAllowed(r *http.Request) bool {
	expected := strings.TrimSpace(os.Getenv("CF_EDGE_SHARED_SECRET"))
	if expected == "" {
		return true
	}
	provided := strings.TrimSpace(r.Header.Get("X-Zyrouter-Edge-Secret"))
	if provided == "" || len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

type verificationRateState struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

var verificationRates = verificationRateState{hits: make(map[string][]time.Time)}

func verificationClientKey(r *http.Request) string {
	if cookie, err := r.Cookie(verificationBrowserCookie); err == nil && strings.TrimSpace(cookie.Value) != "" {
		return "browser:" + db.HashUserSecret(cookie.Value)
	}
	if value := strings.TrimSpace(r.Header.Get("X-Real-IP")); value != "" {
		return value
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func allowVerificationRequest(r *http.Request, limit int, window time.Duration) bool {
	now := time.Now()
	key := verificationClientKey(r)
	verificationRates.mu.Lock()
	defer verificationRates.mu.Unlock()
	items := verificationRates.hits[key]
	cutoff := now.Add(-window)
	kept := items[:0]
	for _, item := range items {
		if item.After(cutoff) {
			kept = append(kept, item)
		}
	}
	if len(kept) >= limit {
		verificationRates.hits[key] = kept
		return false
	}
	verificationRates.hits[key] = append(kept, now)
	return true
}

func NewHandler(repo *db.Repo) *Handler { return &Handler{Repo: repo} }

func (h *Handler) StartVerification(w http.ResponseWriter, r *http.Request) {
	if !edgeSecretAllowed(r) {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "edge_gateway_required")
		return
	}
	if !middleware.BrowserOriginAllowed(r) {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "csrf_origin_forbidden")
		return
	}
	if cookie, cookieErr := r.Cookie(verificationBrowserCookie); cookieErr == nil && strings.TrimSpace(cookie.Value) != "" {
		if existingID, expires, lookupErr := h.Repo.GetActiveVerificationChallenge(db.HashUserSecret(cookie.Value)); lookupErr == nil && existingID != "" {
			bot := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_USERNAME"))
			deepLink := ""
			if bot != "" {
				deepLink = "https://t.me/" + strings.TrimPrefix(bot, "@") + "?start=" + existingID
			}
			handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"challengeId": existingID, "status": "pending", "expiresAt": expires.Format(time.RFC3339), "telegramDeepLink": deepLink, "reused": true})
			return
		}
	}
	if !allowVerificationRequest(r, 5, 10*time.Minute) {
		handlerutil.WriteJSONError(w, http.StatusTooManyRequests, "verification_start_rate_limited")
		return
	}
	browserKey, err := randomToken(32)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to create verification browser binding")
		return
	}
	id, expires, err := h.Repo.CreateVerificationChallengeForBrowser(10*time.Minute, db.HashUserSecret(browserKey))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to create verification challenge")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: verificationBrowserCookie, Value: browserKey, Path: "/", HttpOnly: true,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteLaxMode, MaxAge: 600,
	})
	bot := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_USERNAME"))
	deepLink := ""
	if bot != "" {
		deepLink = "https://t.me/" + strings.TrimPrefix(bot, "@") + "?start=" + id
	}
	handlerutil.WriteJSON(w, http.StatusCreated, map[string]any{
		"challengeId": id, "status": "pending", "expiresAt": expires.Format(time.RFC3339),
		"telegramDeepLink": deepLink,
		"message":          "Open the Telegram bot and approve this verification request.",
	})
}

func (h *Handler) VerificationStatus(w http.ResponseWriter, r *http.Request) {
	if !edgeSecretAllowed(r) {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "edge_gateway_required")
		return
	}
	if !allowVerificationRequest(r, 90, time.Minute) {
		handlerutil.WriteJSONError(w, http.StatusTooManyRequests, "verification_poll_rate_limited")
		return
	}
	challengeID := chi.URLParam(r, "id")
	browserCookie, cookieErr := r.Cookie(verificationBrowserCookie)
	storedBrowserKey, browserErr := h.Repo.GetVerificationBrowserKey(challengeID)
	if browserErr != nil || cookieErr != nil || storedBrowserKey == "" || db.HashUserSecret(browserCookie.Value) != storedBrowserKey {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "verification challenge is not bound to this browser")
		return
	}
	status, user, err := h.Repo.GetVerificationStatus(challengeID)
	if errors.Is(err, sql.ErrNoRows) {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "verification challenge not found")
		return
	}
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	response := map[string]any{"status": status}
	if user != nil {
		session, err := randomToken(32)
		if err != nil || h.Repo.CreateUserSession(user.ID, session, 24*time.Hour) != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to create user session")
			return
		}
		response["user"] = sanitizeUser(user)
		http.SetCookie(w, &http.Cookie{
			Name:     "user_session",
			Value:    session,
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400,
		})
	}
	handlerutil.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) CompleteVerification(w http.ResponseWriter, r *http.Request) {
	if !edgeSecretAllowed(r) {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "edge_gateway_required")
		return
	}
	if !middleware.BrowserOriginAllowed(r) {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "csrf_origin_forbidden")
		return
	}
	if !allowVerificationRequest(r, 10, 5*time.Minute) {
		handlerutil.WriteJSONError(w, http.StatusTooManyRequests, "verification_complete_rate_limited")
		return
	}
	var body struct {
		ChallengeID      string `json:"challengeId"`
		ConfirmationCode string `json:"confirmationCode"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.ChallengeID) == "" || strings.TrimSpace(body.ConfirmationCode) == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "challengeId and confirmationCode are required")
		return
	}
	cookie, err := r.Cookie(verificationBrowserCookie)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "verification browser binding is missing")
		return
	}
	user, err := h.Repo.CompleteVerification(body.ChallengeID, cookie.Value, body.ConfirmationCode, "user")
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusForbidden, err.Error())
		return
	}
	session, err := randomToken(32)
	if err != nil || h.Repo.CreateUserSession(user.ID, session, 24*time.Hour) != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to create user session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "user_session", Value: session, Path: "/", HttpOnly: true, Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"), SameSite: http.SameSiteLaxMode, MaxAge: 86400})
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"status": "verified", "user": sanitizeUser(user), "redirect": "/#dashboard"})
}

// TelegramWebhook receives only server-to-server bot updates. Configure Telegram
// with TELEGRAM_WEBHOOK_SECRET and reject every request without the exact secret.
func (h *Handler) TelegramWebhook(w http.ResponseWriter, r *http.Request) {
	if !edgeSecretAllowed(r) {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "edge_gateway_required")
		return
	}
	secret := strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_SECRET"))
	if secret == "" || r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != secret {
		handlerutil.WriteJSONError(w, http.StatusUnauthorized, "invalid telegram webhook secret")
		return
	}
	if !allowVerificationRequest(r, 120, time.Minute) {
		handlerutil.WriteJSONError(w, http.StatusTooManyRequests, "telegram_webhook_rate_limited")
		return
	}
	var update struct {
		Message *struct {
			From *struct {
				ID                            int64 `json:"id"`
				Username, FirstName, LastName string
			} `json:"from"`
			Text string `json:"text"`
		} `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil || update.Message == nil || update.Message.From == nil {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	parts := strings.Fields(update.Message.Text)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "/start") {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	name := strings.TrimSpace(update.Message.From.FirstName + " " + update.Message.From.LastName)
	confirmationCode, err := h.Repo.MarkChallengeTelegramVerified(parts[1], fmtInt64(update.Message.From.ID), update.Message.From.Username, name)
	if err != nil {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "rejected", "reason": err.Error()})
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"status": "telegram_verified", "confirmationCode": confirmationCode})
}

func (h *Handler) Profile(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetAuthenticatedUser(r)
	typ, err := h.Repo.GetAccountType(user.AccountTypeID)
	if err != nil || typ == nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "account type unavailable")
		return
	}
	aliases, err := h.Repo.GetAccountTypeModels(typ.ID)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to load model permissions")
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"user": sanitizeUser(user), "accountType": typ, "allowedAliases": aliases})
}

func (h *Handler) Features(w http.ResponseWriter, r *http.Request) {
	settings, err := h.Repo.GetUserSettings(middleware.GetAuthenticatedUser(r).ID)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to load feature settings")
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, settings)
}

func (h *Handler) UpdateFeatures(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetAuthenticatedUser(r)
	typ, err := h.Repo.GetAccountType(user.AccountTypeID)
	if err != nil || typ == nil || typ.IsActive != 1 {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "account type is inactive")
		return
	}
	var settings models.UserFeatureSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if settings.RTKEnabled != nil && *settings.RTKEnabled && !typ.AllowRTK {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "RTK is not enabled for this account type")
		return
	}
	if settings.CavemanEnabled != nil && *settings.CavemanEnabled && !typ.AllowCaveman {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "Caveman is not enabled for this account type")
		return
	}
	if settings.PonytailEnabled != nil && *settings.PonytailEnabled && !typ.AllowPonytail {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "Ponytail is not enabled for this account type")
		return
	}
	if err := h.Repo.SaveUserSettings(user.ID, &settings); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to save feature settings")
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, &settings)
}

func (h *Handler) GetKey(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetAuthenticatedUser(r)
	key, err := h.Repo.GetActiveUserApiKey(user.ID)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to load api key")
		return
	}
	if key == nil {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"key": nil})
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"id": key.ID, "keyPrefix": key.Key, "isActive": key.IsActive, "createdAt": key.CreatedAt})
}

func (h *Handler) Usage(w http.ResponseWriter, r *http.Request) {
	usage, err := h.Repo.GetUserUsage(middleware.GetAuthenticatedUser(r).ID)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to load usage")
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, usage)
}

func (h *Handler) Logs(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetAuthenticatedUser(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	logs, err := h.Repo.GetUserUsageLogs(user.ID, limit, offset)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to load request logs")
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, logs)
}

// LogsStream emits only sanitized lifecycle events belonging to the authenticated user.
func (h *Handler) LogsStream(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetAuthenticatedUser(r)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "streaming is unavailable")
		return
	}
	writeEvent := func(eventName string, value any) bool {
		payload, err := json.Marshal(value)
		if err != nil {
			return false
		}
		if _, err := w.Write([]byte("event: " + eventName + "\ndata: " + string(payload) + "\n\n")); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	subject := "user:" + user.ID
	ch, unsubscribe := clientstream.Get().Subscribe(subject)
	defer unsubscribe()
	// Subscribe before taking the snapshot so an event cannot land in the gap
	// between history delivery and live delivery. The browser deduplicates IDs.
	writeEvent("snapshot", map[string]any{"items": clientstream.Get().Snapshot(subject)})
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case frame, ok := <-ch:
			if !ok {
				return
			}
			if _, err := w.Write([]byte("event: request\ndata: " + string(frame) + "\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": heartbeat\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *Handler) GenerateKey(w http.ResponseWriter, r *http.Request) { h.rotateKey(w, r, false) }
func (h *Handler) RotateKey(w http.ResponseWriter, r *http.Request)   { h.rotateKey(w, r, true) }

func (h *Handler) rotateKey(w http.ResponseWriter, r *http.Request, rotate bool) {
	user := middleware.GetAuthenticatedUser(r)
	// Account type is always resolved from the authenticated user record. The
	// client cannot select administrator or another tier in the request body.
	typ, err := h.Repo.GetAccountType(user.AccountTypeID)
	if err != nil || typ == nil || typ.IsActive != 1 {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "account type is inactive")
		return
	}
	if !rotate {
		old, err := h.Repo.GetActiveUserApiKey(user.ID)
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to inspect current api key")
			return
		}
		if old != nil {
			handlerutil.WriteJSONError(w, http.StatusConflict, "user already has an active api key; use rotate instead")
			return
		}
	}
	raw, err := randomToken(32)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to generate api key")
		return
	}
	raw = "zy_" + raw
	id := "key_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if rotate {
		err = h.Repo.RotateUserApiKey(user.ID, typ.ID, id, raw, "User API Key")
	} else {
		err = h.Repo.CreateUserApiKey(user.ID, typ.ID, id, raw, "User API Key")
	}
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to save api key")
		return
	}
	handlerutil.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "key": raw, "accountTypeId": typ.ID, "warning": "Store this key securely. It will not be shown again."})
}

func (h *Handler) RevokeKey(w http.ResponseWriter, r *http.Request) {
	if err := h.Repo.RevokeUserApiKey(middleware.GetAuthenticatedUser(r).ID); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to revoke api key")
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	token := middleware.ExtractAuthToken(r)
	if token != "" {
		_ = h.Repo.RevokeUserSession(token)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "user_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func sanitizeUser(user *models.User) map[string]any {
	return map[string]any{"id": user.ID, "telegramUsername": user.TelegramUsername, "displayName": user.DisplayName, "accountTypeId": user.AccountTypeID, "isActive": user.IsActive, "verifiedAt": user.VerifiedAt}
}

func randomToken(size int) (string, error) {
	b := make([]byte, size)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
func fmtInt64(v int64) string { return strconv.FormatInt(v, 10) }
