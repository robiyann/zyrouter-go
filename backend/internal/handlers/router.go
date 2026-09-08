package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"zyrouter/backend/internal/constants"
	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlers/admin"
	"zyrouter/backend/internal/handlers/chat"
	clienthandlers "zyrouter/backend/internal/handlers/client"
	"zyrouter/backend/internal/handlers/deployment"
	"zyrouter/backend/internal/handlers/oauth"
	"zyrouter/backend/internal/handlers/shared"
	userhandlers "zyrouter/backend/internal/handlers/user"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/middleware"
)

// Re-export TokenSaverConfig for root compatibility
type TokenSaverConfig = shared.TokenSaverConfig

// NewTokenSaverConfig re-exports shared.NewTokenSaverConfig.
func NewTokenSaverConfig(rtk, caveman, ponytail bool) *TokenSaverConfig {
	return shared.NewTokenSaverConfig(rtk, caveman, ponytail)
}

// SetupRoutes mounts all domain handlers on the provided router.
func SetupRoutes(r interface {
	Get(pattern string, handlerFn http.HandlerFunc)
	Post(pattern string, handlerFn http.HandlerFunc)
	Put(pattern string, handlerFn http.HandlerFunc)
	Delete(pattern string, handlerFn http.HandlerFunc)
	HandleFunc(pattern string, handlerFn http.HandlerFunc)
}, repo *db.Repo, ts *TokenSaverConfig) {
	chatH := chat.NewChatHandler(repo, ts)
	deploymentH := deployment.NewHandler(repo)
	oauthH := oauth.NewOAuthHandler(repo)
	adminH := admin.NewAdminHandler(repo)
	adminH.SetChatTester(chatH)

	// Admin CRUD & Governance Domain
	r.Get("/api/keys", adminH.HandleGetKeys)
	r.Get("/api/keys/{id}/reveal", adminH.HandleRevealKey)
	r.Post("/api/keys", adminH.HandleCreateKey)
	r.Put("/api/keys/{id}", adminH.HandleUpdateKey)
	r.Delete("/api/keys/{id}", adminH.HandleDeleteKey)

	r.Get("/api/providers", adminH.HandleGetProviders)
	r.Post("/api/providers", adminH.HandleCreateProvider)
	r.Get("/api/providers/{id}", adminH.HandleGetProvider)
	r.Put("/api/providers/{id}", adminH.HandleUpdateProvider)
	r.Delete("/api/providers/{id}", adminH.HandleDeleteProvider)
	r.Get("/api/providers/{id}/models", adminH.HandleFetchProviderConnectionModels)
	r.Post("/api/providers/{id}/test-model", adminH.HandleTestProviderModel)
	r.Get("/api/combos", adminH.HandleGetCombos)
	r.Post("/api/combos", adminH.HandleCreateCombo)
	r.Put("/api/combos/{id}", adminH.HandleUpdateCombo)
	r.Delete("/api/combos/{id}", adminH.HandleDeleteCombo)

	r.Get("/api/settings", adminH.HandleGetSettings)
	r.Put("/api/settings", adminH.HandleUpdateSettings)
	r.Post("/api/settings", adminH.HandleUpdateSettings)
	r.Get("/api/settings/database", adminH.HandleExportDatabase)
	r.Post("/api/settings/database", adminH.HandleImportDatabase)
	r.Get("/api/proxy-pools", adminH.HandleGetProxyPools)
	r.Get("/api/audit-logs/files", adminH.HandleListAuditFiles)
	r.Get("/api/audit-logs/files/{filename}", adminH.HandleDownloadAuditFile)
	r.Get("/api/auth-logs", adminH.HandleGetAuthLogs)
	r.Post("/api/proxy-pools", adminH.HandleCreateProxyPool)
	r.Delete("/api/proxy-pools/{id}", adminH.HandleDeleteProxyPool)
	r.Post("/api/proxy-pools/{id}/test", adminH.HandleTestProxyPool)
	r.Get("/api/model-aliases", adminH.HandleGetModelAliases)
	r.Post("/api/model-aliases", adminH.HandleSetModelAlias)
	r.Put("/api/model-aliases/{alias}", adminH.HandleSetModelAlias)
	r.Delete("/api/model-aliases/{alias}", adminH.HandleDeleteModelAlias)
	r.Post("/api/model-aliases/{alias}/test", adminH.HandleTestModelAlias)

	r.Get("/api/custom-models", adminH.HandleGetCustomModels)
	r.Post("/api/custom-models", adminH.HandleAddCustomModel)
	r.Delete("/api/custom-models", adminH.HandleDeleteCustomModel)
	r.Delete("/api/custom-models/{key}", adminH.HandleDeleteCustomModel)
	r.Get("/api/models/custom", adminH.HandleGetCustomModels)
	r.Post("/api/models/custom", adminH.HandleAddCustomModel)
	r.Delete("/api/models/custom", adminH.HandleDeleteCustomModel)

	r.Get("/api/provider-nodes", adminH.HandleGetProviderNodes)
	r.Post("/api/provider-nodes", adminH.HandleCreateProviderNode)
	r.Put("/api/provider-nodes/{id}", adminH.HandleUpdateProviderNode)
	r.Delete("/api/provider-nodes/{id}", adminH.HandleDeleteProviderNode)
	r.Post("/api/provider-nodes/validate", adminH.HandleValidateProviderNode)
	r.Get("/api/provider-prefixes", adminH.HandleGetProviderPrefixes)
	r.Post("/api/provider-prefixes", adminH.HandleSetProviderPrefix)
	r.Put("/api/provider-prefixes", adminH.HandleSetProviderPrefix)
	r.Delete("/api/provider-prefixes/{provider}", adminH.HandleDeleteProviderPrefix)
	r.Delete("/api/provider-prefixes", adminH.HandleDeleteProviderPrefix)

	// Chat, Version & Models Domain
	r.Get("/version", chatH.HandleVersion)
	r.Get("/api/version", chatH.HandleVersion)
	r.Get("/api/version/check", chatH.HandleCheckUpdate)
	r.Post("/api/version/update", chatH.HandleTriggerUpdate)
	r.Get("/models", chatH.HandleModels)
	r.Get("/v1/models", chatH.HandleModels)
	r.Get("/models/info", chatH.HandleModelsInfo)
	r.Get("/v1/models/info", chatH.HandleModelsInfo)
	r.Get("/models/{kind}", chatH.HandleModelsByKind)
	r.Get("/v1/models/{kind}", chatH.HandleModelsByKind)
	r.Post("/chat/completions", chatH.HandleChatCompletions)
	r.Post("/v1/chat/completions", chatH.HandleChatCompletions)
	r.Post("/responses", chatH.HandleResponses)
	r.Post("/v1/responses", chatH.HandleResponses)
	r.Post("/messages", chatH.HandleMessages)
	r.Post("/v1/messages", chatH.HandleMessages)
	r.Post("/messages/count_tokens", chatH.HandleCountTokens)
	r.Post("/v1/messages/count_tokens", chatH.HandleCountTokens)
	r.Post("/api/chat", chatH.HandleOllamaChat)

	// Proxy Pool Deploy Domain
	r.Post("/api/proxy-pools/vercel-deploy", deploymentH.HandleVercelDeploy)
	r.Post("/api/proxy-pools/vercel-deploy/jobs", deploymentH.HandleCreateVercelDeployJob)
	r.Get("/api/proxy-pools/vercel-deploy/jobs", deploymentH.HandleGetVercelDeployJobs)
	r.Get("/api/proxy-pools/vercel-deploy/jobs/{id}", deploymentH.HandleGetVercelDeployJob)
	r.Get("/api/proxy-pools/vercel-deploy/jobs/{id}/stream", deploymentH.HandleVercelDeployJobStream)
	r.Post("/api/proxy-pools/vercel-deploy/jobs/{id}/cancel", deploymentH.HandleCancelVercelDeployJob)
	r.Post("/api/proxy-pools/deno-deploy", deploymentH.HandleDenoDeploy)
	r.Post("/api/proxy-pools/cloudflare-deploy", deploymentH.HandleCloudflareDeploy)
	r.Post("/proxy-pools/vercel-deploy", deploymentH.HandleVercelDeploy)
	r.Post("/proxy-pools/deno-deploy", deploymentH.HandleDenoDeploy)
	r.Post("/proxy-pools/cloudflare-deploy", deploymentH.HandleCloudflareDeploy)
	// OAuth & Import Tokens Domain
	r.Post("/api/oauth/{provider}/import", oauthH.HandleOAuthImport)
	r.Get("/api/oauth/{provider}/authorize", oauthH.HandleOAuthAuthorize)
	r.Post("/api/oauth/{provider}/exchange", oauthH.HandleOAuthExchange)
	r.Post("/api/oauth/github/device-code", oauthH.HandleGitHubDeviceCode)
	r.Post("/api/oauth/github/poll", oauthH.HandleGitHubPoll)
	r.Get("/api/oauth/cursor/auto-import", oauthH.HandleCursorAutoImport)
	r.Get("/api/oauth/kiro/social-authorize", oauthH.HandleOAuthKiroSocialAuthorize)
	r.Post("/api/oauth/kiro/social-exchange", oauthH.HandleOAuthKiroSocialExchange)
	r.Post("/api/oauth/codex/bulk-import", oauthH.HandleOAuthCodexBulkImport)

	// Live Console Logs Domain (dashboard "Monitor Console Log")
	r.Get("/translator/console-logs", HandleConsoleLogsGet)
	r.Delete("/translator/console-logs", HandleConsoleLogsDelete)
	r.Get("/translator/console-logs/stream", HandleConsoleLogsStream)

	// Usage Real-time SSE Stream & Stats Domain (dashboard topology animation + recent requests)
	r.Get("/usage/stream", HandleUsageStream(repo))
	r.Get("/api/usage/stream", HandleUsageStream(repo))
	r.Get("/usage/stats", HandleUsageStats(repo))
	r.Get("/api/usage/stats", HandleUsageStats(repo))

	// Debug Tracing Domain (p50/p95 latency per provider+model)
	r.Get("/debug/traces", HandleDebugTraces)
}

// SetupServerRouter mounts public endpoints (/health, /api/hello) and
// API-key protected routes (all engine + admin routes) on the chi router.
func SetupServerRouter(r chi.Router, repo *db.Repo, ts *TokenSaverConfig) {
	r.Use(middleware.CORS)

	chatH := chat.NewChatHandler(repo, ts)
	adminH := admin.NewAdminHandler(repo)
	adminH.SetChatTester(chatH)
	clientH := clienthandlers.NewHandler(repo)
	userH := userhandlers.NewHandler(repo)
	// Public (unauthenticated) endpoints
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.Write([]byte(`{"status":"ok"}`))
	})

	r.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			w.Write([]byte(`{"status":"ok","message":"hello"}`))
		}
	})
	r.Get("/api/public/telemetry/stats", HandlePublicTelemetryStats(repo))
	r.Get("/api/public/telemetry/stream", HandlePublicTelemetryStream(repo))
	r.Options("/api/public/telemetry/stats", HandlePublicTelemetryStats(repo))
	r.Options("/api/public/telemetry/stream", HandlePublicTelemetryStream(repo))

	r.Post("/api/auth/login", HandleAuthLogin(repo))
	r.Post("/api/auth/logout", HandleAuthLogout())
	r.Get("/api/auth/status", HandleAuthStatus(repo))

	// Telegram verification is public only for challenge creation/status and the
	// locked bot webhook. No database or admin data is exposed by these routes.
	r.Post("/api/user/verification/start", userH.StartVerification)
	r.Get("/api/user/verification/{id}", userH.VerificationStatus)
	r.Post("/api/user/verification/complete", userH.CompleteVerification)
	r.Post("/api/telegram/webhook", userH.TelegramWebhook)
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireUserSession(repo))
		r.Use(middleware.RequireBrowserCSRF)
		r.Get("/api/user/profile", userH.Profile)
		r.Get("/api/user/usage", userH.Usage)
		r.Get("/api/user/logs", userH.Logs)
		r.Get("/api/user/logs/stream", userH.LogsStream)
		r.Get("/api/user/features", userH.Features)
		r.Put("/api/user/features", userH.UpdateFeatures)
		r.Get("/api/user/key", userH.GetKey)
		r.Post("/api/user/key", userH.GenerateKey)
		r.Post("/api/user/key/rotate", userH.RotateKey)
		r.Delete("/api/user/key", userH.RevokeKey)
		r.Post("/api/user/logout", userH.Logout)
		r.Get("/api/user/global-usage", HandleClientTelemetryStats(repo))
		r.Get("/api/user/global-usage/stream", HandleClientTelemetryStream(repo))
	})

	// Future client dashboard API. It is intentionally isolated from admin/API-key routes.
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireClientAccess(repo))
		r.Get("/api/client/profile", clientH.HandleProfile)
		r.Get("/api/client/policy", clientH.HandlePolicy)
		r.Get("/api/client/keys", clientH.HandleKeys)
		r.Post("/api/client/keys", clientH.HandleCreateKey)
		r.Delete("/api/client/keys/{id}", clientH.HandleRevokeKey)
		r.Get("/api/client/usage", clientH.HandleUsage)
		r.Get("/api/client/logs", clientH.HandleLogs)
		r.Get("/api/client/logs/stream", clientH.HandleLogsStream)
		r.Get("/api/client/global-usage", HandleClientTelemetryStats(repo))
		r.Get("/api/client/global-usage/stream", HandleClientTelemetryStream(repo))
	})

	// API-key / Dashboard session protected domain routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAdminAccess(repo))
		r.Post("/api/auth/change-password", HandleAuthChangePassword(repo))
		r.Post("/api/admin/client-policies", adminH.HandleCreateClientPolicy)
		r.Post("/api/admin/clients", adminH.HandleCreateClient)
		r.Get("/api/admin/account-types", adminH.HandleGetAccountTypes)
		r.Post("/api/admin/account-types", adminH.HandleUpsertAccountType)
		r.Put("/api/admin/account-types/{id}", adminH.HandleUpsertAccountType)
		r.Delete("/api/admin/account-types/{id}", adminH.HandleDeleteAccountType)
		r.Get("/api/admin/account-types/{id}/models", adminH.HandleGetAccountTypeModels)
		r.Post("/api/admin/account-types/{id}/models", adminH.HandleSetAccountTypeModels)
		r.Post("/api/admin/model-policy/preview", adminH.HandlePreviewModelPolicy)
		r.Get("/api/admin/security/summary", adminH.HandleSecuritySummary)
		r.Get("/api/admin/users", adminH.HandleGetUsers)
		r.Put("/api/admin/users/{id}/account-type", adminH.HandleUpdateUserAccountType)
		r.Delete("/api/admin/users/{id}/key", adminH.HandleRevokeUserKey)
		// Health reset endpoint for the admin dashboard.
		r.Post("/admin/health/reset", func(w http.ResponseWriter, r *http.Request) {
			provider := r.URL.Query().Get("provider")
			model := r.URL.Query().Get("model")
			if err := repo.ResetProviderHealth(provider, model); err != nil {
				handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		})
		SetupRoutes(r, repo, ts)
	})
}
