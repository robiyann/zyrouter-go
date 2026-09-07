package admin

import (
	"net/http"
	"strconv"

	"zyrouter/backend/internal/handlerutil"
)

// HandleGetAuthLogs returns compact dashboard access/security events only.
// Passwords, cookies, API keys, and request bodies are never stored here.
func (h *AdminHandler) HandleGetAuthLogs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	logs, err := h.repo.ListAuthLogs(limit, offset)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to load auth logs")
		return
	}
	count, err := h.repo.CountAuthLogs()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to count auth logs")
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"logs": logs, "total": count, "limit": limit, "offset": offset})
}
