package admin

import (
	"encoding/json"
	"fmt"
	"net/http"

	"zyrouter/backend/internal/antigravityquota"
	"zyrouter/backend/internal/handlerutil"
)

func (h *AdminHandler) HandleGetQuota(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("refresh") == "true" || r.URL.Query().Get("force") == "true" || r.URL.Query().Get("refresh") == "1"
	if h.quotaService == nil {
		h.quotaService = antigravityquota.NewService(h.repo)
	}

	var snapshot *antigravityquota.Snapshot
	var err error

	if force {
		snapshot, err = h.quotaService.Snapshot(r.Context(), true)
	} else {
		cached := h.quotaService.GetCachedSnapshot()
		if cached != nil {
			snapshot = cached
		} else {
			snapshot, err = h.quotaService.Snapshot(r.Context(), false)
		}
	}

	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("fetch quota: %v", err))
		return
	}

	resp := map[string]any{
		"fetchedAt":           snapshot.FetchedAt,
		"accounts":            snapshot.Accounts,
		"autoRefreshInterval": h.quotaService.GetAutoRefreshInterval(),
	}
	handlerutil.WriteJSON(w, http.StatusOK, resp)
}

func (h *AdminHandler) HandleUpdateQuotaSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AutoRefreshInterval int `json:"autoRefreshInterval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if h.quotaService == nil {
		h.quotaService = antigravityquota.NewService(h.repo)
	}

	h.quotaService.SetAutoRefreshInterval(r.Context(), body.AutoRefreshInterval)

	if settings, err := h.repo.GetSettings(); err == nil && settings != nil {
		settings.QuotaAutoRefreshInterval = body.AutoRefreshInterval
		_ = h.repo.UpdateSettingsData(settings)
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":              "ok",
		"autoRefreshInterval": h.quotaService.GetAutoRefreshInterval(),
	})
}
