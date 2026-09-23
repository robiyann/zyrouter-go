package admin

import (
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
	snapshot, err := h.quotaService.Snapshot(r.Context(), force)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("fetch quota: %v", err))
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, snapshot)
}
