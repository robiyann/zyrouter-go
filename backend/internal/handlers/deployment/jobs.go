package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"zyrouter/backend/internal/handlerutil"
)

type vercelJob struct {
	ID              string    `json:"id"`
	Status          string    `json:"status"`
	Total           int       `json:"total"`
	Completed       int       `json:"completed"`
	Failed          int       `json:"failed"`
	CurrentProject  string    `json:"currentProject,omitempty"`
	LastError       string    `json:"lastError,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	FinishedAt      time.Time `json:"finishedAt,omitempty"`
	DelayMode       string    `json:"delayMode"`
	DelayMinSeconds int       `json:"delayMinSeconds"`
	DelayMaxSeconds int       `json:"delayMaxSeconds"`
	token           string
	cancel          context.CancelFunc
	subscribers     map[chan vercelJob]struct{}
}

type vercelJobManager struct {
	mu   sync.RWMutex
	jobs map[string]*vercelJob
}

func newVercelJobManager() *vercelJobManager {
	return &vercelJobManager{jobs: make(map[string]*vercelJob)}
}

func (m *vercelJobManager) snapshot(job *vercelJob) vercelJob {
	copy := *job
	copy.token = ""
	copy.cancel = nil
	copy.subscribers = nil
	return copy
}

func (m *vercelJobManager) publish(job *vercelJob) {
	snapshot := m.snapshot(job)
	for sub := range job.subscribers {
		select {
		case sub <- snapshot:
		default:
		}
	}
}

func (m *vercelJobManager) update(job *vercelJob, fn func(*vercelJob)) {
	m.mu.Lock()
	fn(job)
	m.publish(job)
	m.mu.Unlock()
}

func (h *Handler) HandleCreateVercelDeployJob(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token           string `json:"token"`
		VercelToken     string `json:"vercelToken"`
		ProjectName     string `json:"projectName"`
		Count           int    `json:"count"`
		DelayMode       string `json:"delayMode"`
		DelayMinSeconds int    `json:"delayMinSeconds"`
		DelayMaxSeconds int    `json:"delayMaxSeconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		token = strings.TrimSpace(input.VercelToken)
	}
	if token == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "Vercel API token is required")
		return
	}
	if input.Count <= 0 {
		input.Count = 1
	}
	if input.Count > 50 {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "maximum 50 deployments per job")
		return
	}
	if input.DelayMode != "random" {
		input.DelayMode = "fixed"
	}
	if input.DelayMinSeconds < 0 {
		input.DelayMinSeconds = 0
	}
	if input.DelayMaxSeconds < input.DelayMinSeconds {
		input.DelayMaxSeconds = input.DelayMinSeconds
	}
	if input.DelayMaxSeconds > 3600 {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "maximum delay is 3600 seconds")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	job := &vercelJob{
		ID: input.ProjectName, Status: "queued", Total: input.Count,
		CreatedAt: time.Now().UTC(), DelayMode: input.DelayMode,
		DelayMinSeconds: input.DelayMinSeconds, DelayMaxSeconds: input.DelayMaxSeconds,
		token: token, cancel: cancel, subscribers: make(map[chan vercelJob]struct{}),
	}
	if job.ID == "" {
		job.ID = fmt.Sprintf("vercel-%d", time.Now().UnixNano())
	}
	h.jobs.mu.Lock()
	// Job IDs are never derived from the token and are safe to expose to the UI.
	if _, exists := h.jobs.jobs[job.ID]; exists {
		job.ID = fmt.Sprintf("%s-%d", job.ID, time.Now().UnixNano()%100000)
	}
	h.jobs.jobs[job.ID] = job
	h.jobs.mu.Unlock()
	go h.runVercelJob(ctx, job, input.ProjectName)

	h.jobs.mu.RLock()
	response := h.jobs.snapshot(job)
	h.jobs.mu.RUnlock()
	handlerutil.WriteJSON(w, http.StatusAccepted, response)
}

func (h *Handler) runVercelJob(ctx context.Context, job *vercelJob, requestedName string) {
	h.jobs.update(job, func(j *vercelJob) { j.Status = "running" })
	defer func() {
		h.jobs.update(job, func(j *vercelJob) {
			if j.Status == "running" {
				j.Status = "completed"
			}
			j.FinishedAt = time.Now().UTC()
			j.token = ""
		})
	}()

	usedNames := make(map[string]struct{}, job.Total)
	for i := 0; i < job.Total; i++ {
		select {
		case <-ctx.Done():
			h.jobs.update(job, func(j *vercelJob) { j.Status = "cancelled" })
			return
		default:
		}

		name := requestedName
		if name == "" || job.Total > 1 {
			name = humanProjectName(usedNames)
			usedNames[name] = struct{}{}
		}
		h.jobs.update(job, func(j *vercelJob) { j.CurrentProject = name })

		body, _ := json.Marshal(map[string]string{"vercelToken": job.token, "projectName": name})
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/proxy-pools/vercel-deploy", strings.NewReader(string(body)))
		rec := httptest.NewRecorder()
		h.HandleVercelDeploy(rec, req)
		if rec.Code >= 200 && rec.Code < 300 {
			h.jobs.update(job, func(j *vercelJob) { j.Completed++ })
		} else {
			h.jobs.update(job, func(j *vercelJob) {
				j.Failed++
				j.LastError = compactJobError(rec.Body.String())
			})
		}

		if i+1 < job.Total {
			delay := job.DelayMinSeconds
			if job.DelayMode == "random" && job.DelayMaxSeconds > delay {
				delay += rand.Intn(job.DelayMaxSeconds - delay + 1)
			}
			timer := time.NewTimer(time.Duration(delay) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

func compactJobError(raw string) string {
	if len(raw) > 500 {
		return raw[:500] + "..."
	}
	return raw
}

var projectAdjectives = []string{"quiet", "silver", "bright", "north", "calm", "swift", "hidden", "gentle", "clear", "urban"}
var projectNouns = []string{"river", "orbit", "maple", "station", "harbor", "meadow", "lantern", "summit", "garden", "bridge"}
var projectQualifiers = []string{"blue", "cedar", "morning", "field", "stone", "willow", "coast", "pine", "hollow", "sunny"}

func humanProjectName(used map[string]struct{}) string {
	// Use random dictionary words rather than sequence numbers. The local set
	// avoids duplicate names inside one batch; Vercel still handles external
	// name collisions.
	for attempts := 0; attempts < 100; attempts++ {
		name := fmt.Sprintf("%s-%s-%s",
			projectAdjectives[rand.Intn(len(projectAdjectives))],
			projectNouns[rand.Intn(len(projectNouns))],
			projectQualifiers[rand.Intn(len(projectQualifiers))])
		if _, exists := used[name]; !exists {
			return name
		}
	}
	return fmt.Sprintf("%s-%s-%s", projectAdjectives[rand.Intn(len(projectAdjectives))], projectNouns[rand.Intn(len(projectNouns))], projectQualifiers[rand.Intn(len(projectQualifiers))])
}

func (h *Handler) HandleGetVercelDeployJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.jobs.mu.RLock()
	job := h.jobs.jobs[id]
	if job == nil {
		h.jobs.mu.RUnlock()
		handlerutil.WriteJSONError(w, http.StatusNotFound, "deployment job not found")
		return
	}
	response := h.jobs.snapshot(job)
	h.jobs.mu.RUnlock()
	handlerutil.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) HandleGetVercelDeployJobs(w http.ResponseWriter, r *http.Request) {
	h.jobs.mu.RLock()
	jobs := make([]vercelJob, 0, len(h.jobs.jobs))
	for _, job := range h.jobs.jobs {
		jobs = append(jobs, h.jobs.snapshot(job))
	}
	h.jobs.mu.RUnlock()
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (h *Handler) HandleCancelVercelDeployJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.jobs.mu.RLock()
	job := h.jobs.jobs[id]
	h.jobs.mu.RUnlock()
	if job == nil {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "deployment job not found")
		return
	}
	if job.cancel != nil {
		job.cancel()
	}
	handlerutil.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "cancelling"})
}

func (h *Handler) HandleVercelDeployJobStream(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.jobs.mu.Lock()
	job := h.jobs.jobs[id]
	if job == nil {
		h.jobs.mu.Unlock()
		handlerutil.WriteJSONError(w, http.StatusNotFound, "deployment job not found")
		return
	}
	updates := make(chan vercelJob, 8)
	job.subscribers[updates] = struct{}{}
	initial := h.jobs.snapshot(job)
	h.jobs.mu.Unlock()
	defer func() {
		h.jobs.mu.Lock()
		delete(job.subscribers, updates)
		close(updates)
		h.jobs.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	write := func(update vercelJob) bool {
		payload, _ := json.Marshal(update)
		_, err := fmt.Fprintf(w, "event: progress\ndata: %s\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
		return err == nil
	}
	if !write(initial) {
		return
	}
	for {
		select {
		case update := <-updates:
			if !write(update) || update.Status == "completed" || update.Status == "failed" || update.Status == "cancelled" {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}
