package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/vinaysrao1/nest/internal/auth"
	"github.com/vinaysrao1/nest/internal/domain"
	"github.com/vinaysrao1/nest/internal/service"
	"github.com/vinaysrao1/nest/internal/signal"
)

// NewRouter constructs the complete chi router with all routes and middleware.
//
// Auth middleware is received as closures -- handler never imports store.
// Route groups:
//
//	Public (no auth): POST /api/v1/auth/login, GET /api/v1/health
//	External (API key auth): POST /api/v1/items, POST /api/v1/items/async, GET /api/v1/policies
//	Internal (Session auth + CSRF): all other routes
//
// Pre-conditions: all service pointers must be non-nil.
// Post-conditions: returns a fully configured chi.Router ready to serve HTTP.
func NewRouter(
	ruleService *service.RuleService,
	configService *service.ConfigService,
	mrtService *service.MRTService,
	itemService *service.ItemService,
	userService *service.UserService,
	apiKeyService *service.APIKeyService,
	signingKeyService *service.SigningKeyService,
	textBankService *service.TextBankService,
	pipeline *service.PostVerdictPipeline,
	signalRegistry *signal.Registry,
	sessionAuthMw func(http.Handler) http.Handler,
	apiKeyAuthMw func(http.Handler) http.Handler,
	enqueueFunc EnqueueFunc,
	logger *slog.Logger,
) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	// -------------------------------------------------------------------------
	// Public routes (no authentication required)
	// -------------------------------------------------------------------------
	r.Post("/api/v1/auth/login", handleLogin(userService, logger))
	r.Get("/api/v1/health", handleHealth())

	// -------------------------------------------------------------------------
	// External routes (API key authentication)
	// -------------------------------------------------------------------------
	r.Group(func(r chi.Router) {
		r.Use(apiKeyAuthMw)
		r.Post("/api/v1/items", handleSubmitSync(itemService, logger))
		r.Post("/api/v1/items/async", handleSubmitAsync(itemService, enqueueFunc, logger))
		r.Get("/api/v1/policies", handleListPolicies(configService, logger))
	})

	// -------------------------------------------------------------------------
	// Internal routes (session authentication + CSRF protection)
	// -------------------------------------------------------------------------
	r.Group(func(r chi.Router) {
		r.Use(sessionAuthMw)
		r.Use(auth.CSRFProtect())

		// Auth management
		r.Post("/api/v1/auth/logout", handleLogout(userService, logger))
		r.Get("/api/v1/auth/me", handleMe())
		r.Post("/api/v1/auth/reset-password", handleRequestPasswordReset(userService, logger))

		// Rules
		r.Get("/api/v1/rules", handleListRules(ruleService, logger))
		r.Post("/api/v1/rules", handleCreateRule(ruleService, logger))
		r.Post("/api/v1/rules/test", handleTestRule(ruleService, logger))
		r.Get("/api/v1/rules/{id}", handleGetRule(ruleService, logger))
		r.Put("/api/v1/rules/{id}", handleUpdateRule(ruleService, logger))
		r.Delete("/api/v1/rules/{id}", handleDeleteRule(ruleService, logger))
		r.Post("/api/v1/rules/{id}/test", handleTestExistingRule(ruleService, logger))

		// Actions
		r.Get("/api/v1/actions", handleListActions(configService, logger))
		r.Post("/api/v1/actions", handleCreateAction(configService, logger))
		r.Get("/api/v1/actions/{id}", handleGetAction(configService, logger))
		r.Put("/api/v1/actions/{id}", handleUpdateAction(configService, logger))
		r.Delete("/api/v1/actions/{id}", handleDeleteAction(configService, logger))

		// Policies (GET list also exposed via API key group above)
		r.Get("/api/v1/policies", handleListPolicies(configService, logger))
		r.Post("/api/v1/policies", handleCreatePolicy(configService, logger))
		r.Get("/api/v1/policies/{id}", handleGetPolicy(configService, logger))
		r.Put("/api/v1/policies/{id}", handleUpdatePolicy(configService, logger))
		r.Delete("/api/v1/policies/{id}", handleDeletePolicy(configService, logger))

		// Item Types
		r.Get("/api/v1/item-types", handleListItemTypes(configService, logger))
		r.Post("/api/v1/item-types", handleCreateItemType(configService, logger))
		r.Get("/api/v1/item-types/{id}", handleGetItemType(configService, logger))
		r.Put("/api/v1/item-types/{id}", handleUpdateItemType(configService, logger))
		r.Delete("/api/v1/item-types/{id}", handleDeleteItemType(configService, logger))

		// MRT
		r.Get("/api/v1/mrt/queues", handleListQueues(mrtService, logger))
		r.With(auth.RequireRole(domain.UserRoleAdmin)).Post("/api/v1/mrt/queues", handleCreateQueue(mrtService, logger))
		r.With(auth.RequireRole(domain.UserRoleAdmin)).Delete("/api/v1/mrt/queues/{id}", handleArchiveQueue(mrtService, logger))
		r.Get("/api/v1/mrt/queues/{id}/jobs", handleListJobs(mrtService, logger))
		r.Post("/api/v1/mrt/queues/{id}/assign", handleAssignJob(mrtService, logger))
		r.Post("/api/v1/mrt/decisions", handleRecordDecision(mrtService, pipeline, logger))
		r.Get("/api/v1/mrt/jobs/*", handleGetJob(mrtService, logger))
		r.Post("/api/v1/mrt/jobs/claim", handleClaimJob(mrtService, logger))

		// Users
		r.Get("/api/v1/users", handleListUsers(userService, logger))
		r.Post("/api/v1/users/invite", handleInviteUser(userService, logger))
		r.Put("/api/v1/users/{id}", handleUpdateUser(userService, logger))
		r.Delete("/api/v1/users/{id}", handleDeleteUser(userService, logger))

		// API Keys
		r.Get("/api/v1/api-keys", handleListAPIKeys(apiKeyService, logger))
		r.Post("/api/v1/api-keys", handleCreateAPIKey(apiKeyService, logger))
		r.Delete("/api/v1/api-keys/{id}", handleRevokeAPIKey(apiKeyService, logger))

		// Text Banks
		r.Get("/api/v1/text-banks", handleListTextBanks(textBankService, logger))
		r.Post("/api/v1/text-banks", handleCreateTextBank(textBankService, logger))
		r.Get("/api/v1/text-banks/{id}", handleGetTextBank(textBankService, logger))
		r.Get("/api/v1/text-banks/{id}/entries", handleListTextBankEntries(textBankService, logger))
		r.Post("/api/v1/text-banks/{id}/entries", handleAddTextBankEntry(textBankService, logger))
		r.Delete("/api/v1/text-banks/{id}/entries/{entryId}", handleDeleteTextBankEntry(textBankService, logger))

		// Signals
		r.Get("/api/v1/signals", handleListSignals(signalRegistry))
		r.Post("/api/v1/signals/test", handleTestSignal(signalRegistry, logger))

		// Signing Keys
		r.Get("/api/v1/signing-keys", handleListSigningKeys(signingKeyService, logger))
		r.Post("/api/v1/signing-keys/rotate", handleRotateSigningKey(signingKeyService, logger))

		// UDFs (static list)
		r.Get("/api/v1/udfs", handleListUDFs())

		// Org Settings
		r.Get("/api/v1/orgs/settings", handleGetOrgSettings(configService, logger))
	})

	return r
}
