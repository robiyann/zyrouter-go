package user

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/middleware"
	"zyrouter/backend/internal/models"
)

type Handler struct{ Repo *db.Repo }

func NewHandler(repo *db.Repo) *Handler { return &Handler{Repo: repo} }

func (h *Handler) StartVerification(w http.ResponseWriter, r *http.Request) {
	id, expires, err := h.Repo.CreateVerificationChallenge(5 * time.Minute)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to create verification challenge")
		return
	}
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
	status, user, err := h.Repo.GetVerificationStatus(chi.URLParam(r, "id"))
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
		response["sessionToken"] = session
		response["warning"] = "Store this session token securely. It will not be shown again."
		http.SetCookie(w, &http.Cookie{
			Name:     "user_session",
			Value:    session,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400,
		})
	}
	handlerutil.WriteJSON(w, http.StatusOK, response)
}

// TelegramWebhook receives only server-to-server bot updates. Configure Telegram
// with TELEGRAM_WEBHOOK_SECRET and reject every request without the exact secret.
func (h *Handler) TelegramWebhook(w http.ResponseWriter, r *http.Request) {
	secret := strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_SECRET"))
	if secret == "" || r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != secret {
		handlerutil.WriteJSONError(w, http.StatusUnauthorized, "invalid telegram webhook secret")
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
	user, err := h.Repo.VerifyChallenge(parts[1], fmtInt64(update.Message.From.ID), update.Message.From.Username, name, "user")
	if err != nil {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "rejected", "reason": err.Error()})
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"status": "verified", "userId": user.ID})
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

func (h *Handler) GenerateKey(w http.ResponseWriter, r *http.Request) { h.rotateKey(w, r, false) }
func (h *Handler) RotateKey(w http.ResponseWriter, r *http.Request)   { h.rotateKey(w, r, true) }

func (h *Handler) rotateKey(w http.ResponseWriter, r *http.Request, rotate bool) {
	user := middleware.GetAuthenticatedUser(r)
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
