package admin

import (
	"encoding/json"
	"net/http"
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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
	users, err := h.repo.ListUsers()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]map[string]any, 0, len(users))
	for _, user := range users {
		activeKey, _ := h.repo.GetActiveUserApiKey(user.ID)
		result = append(result, map[string]any{"id": user.ID, "telegramUsername": user.TelegramUsername, "displayName": user.DisplayName, "accountTypeId": user.AccountTypeID, "isActive": user.IsActive, "verifiedAt": user.VerifiedAt, "createdAt": user.CreatedAt})
		result[len(result)-1]["hasActiveKey"] = activeKey != nil
		if activeKey != nil {
			result[len(result)-1]["keyPrefix"] = activeKey.Key
			result[len(result)-1]["keyCreatedAt"] = activeKey.CreatedAt
		}
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"users": result})
}

func (h *AdminHandler) HandleRevokeUserKey(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.RevokeUserApiKey(chi.URLParam(r, "id")); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to revoke user api key")
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *AdminHandler) HandleUpdateUserAccountType(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccountTypeID string `json:"accountTypeId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.AccountTypeID) == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "accountTypeId is required")
		return
	}
	if err := h.repo.UpdateUserAccountType(chi.URLParam(r, "id"), strings.TrimSpace(body.AccountTypeID)); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
