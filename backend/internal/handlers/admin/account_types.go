package admin

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/models"
)

func (h *AdminHandler) HandleGetAccountTypes(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListAccountTypes(false)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"accountTypes": items})
}

func (h *AdminHandler) HandleUpsertAccountType(w http.ResponseWriter, r *http.Request) {
	var body struct {
		models.AccountType
		IsActive *int `json:"isActive"`
	}
	if err := decodeJSON(r, &body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	item := body.AccountType
	if body.IsActive != nil {
		item.IsActive = *body.IsActive
	} else if r.Method == http.MethodPost {
		item.IsActive = 1
	} else {
		// On PUT, preserve existing isActive if omitted
		if existing, err := h.repo.GetAccountType(item.ID); err == nil && existing != nil {
			item.IsActive = existing.IsActive
		} else {
			item.IsActive = 1
		}
	}
	item.ID = strings.TrimSpace(item.ID)
	if item.ID == "" {
		item.ID = strings.TrimSpace(chi.URLParam(r, "id"))
	}
	item.Name = strings.TrimSpace(item.Name)
	if item.ID == "" || item.Name == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "id and name are required")
		return
	}
	if item.QuotaMode == "" {
		item.QuotaMode = "unlimited"
	}
	if err := h.repo.UpsertAccountType(&item); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	itemPtr, _ := h.repo.GetAccountType(item.ID)
	handlerutil.WriteJSON(w, http.StatusOK, itemPtr)
}

func (h *AdminHandler) HandleDeleteAccountType(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.DeleteAccountType(chi.URLParam(r, "id")); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *AdminHandler) HandleGetAccountTypeModels(w http.ResponseWriter, r *http.Request) {
	aliases, err := h.repo.GetAccountTypeModels(chi.URLParam(r, "id"))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"aliases": aliases})
}

func (h *AdminHandler) HandleSetAccountTypeModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Aliases []string `json:"aliases"`
	}
	if err := decodeJSON(r, &body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	aliases, err := h.repo.GetModelAliases()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, alias := range body.Aliases {
		if _, ok := aliases[alias]; !ok {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "unknown model alias: "+alias)
			return
		}
	}
	if err := h.repo.SetAccountTypeModels(chi.URLParam(r, "id"), body.Aliases); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"aliases": body.Aliases})
}

func (h *AdminHandler) HandleGetUsers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	search := r.URL.Query().Get("q")
	status := r.URL.Query().Get("status")
	accountTypeID := r.URL.Query().Get("accountTypeId")
	rows, total, err := h.repo.ListUsersPage(page, pageSize, search, status, accountTypeID)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		user := row.User
		result = append(result, map[string]any{"id": user.ID, "telegramUserId": user.TelegramUserID, "telegramUsername": user.TelegramUsername, "displayName": user.DisplayName, "accountTypeId": user.AccountTypeID, "isActive": user.IsActive, "isBanned": user.IsActive != 1, "verifiedAt": user.VerifiedAt, "createdAt": user.CreatedAt, "hasActiveKey": row.HasActiveKey, "keyPrefix": row.KeyPrefix, "keyCreatedAt": row.KeyCreatedAt})
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"users": result, "page": page, "pageSize": pageSize, "total": total, "totalPages": totalPages, "q": search, "status": status, "accountTypeId": accountTypeID})
}

func (h *AdminHandler) HandleRevokeUserKey(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.RevokeUserApiKey(chi.URLParam(r, "id")); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to revoke user api key")
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *AdminHandler) HandleSetUserBan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Banned *bool `json:"banned"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Banned == nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "banned is required")
		return
	}
	telegramUserID := strings.TrimSpace(chi.URLParam(r, "telegramUserId"))
	if telegramUserID == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "telegram user id is required")
		return
	}
	if err := h.repo.SetUserBannedByTelegramID(telegramUserID, *body.Banned); err != nil {
		if err == sql.ErrNoRows {
			handlerutil.WriteJSONError(w, http.StatusNotFound, "telegram user not found")
			return
		}
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to update user ban state")
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"telegramUserId": telegramUserID,
		"banned":         *body.Banned,
	})
}

func (h *AdminHandler) HandleUpdateUserAccountType(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccountTypeID string `json:"accountTypeId"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.AccountTypeID) == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "accountTypeId is required")
		return
	}
	if err := h.repo.UpdateUserAccountType(chi.URLParam(r, "id"), strings.TrimSpace(body.AccountTypeID)); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
