package router

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/gaskaj/OpenAgentFramework/web/auth"
	"github.com/gaskaj/OpenAgentFramework/web/config"
	"github.com/gaskaj/OpenAgentFramework/web/handler"
	"github.com/gaskaj/OpenAgentFramework/web/middleware"
	"github.com/gaskaj/OpenAgentFramework/web/store"
	"github.com/gaskaj/OpenAgentFramework/web/ws"
)

// New creates a new chi router with all routes configured.
func New(
	stores *store.PostgresStore,
	jwtMgr *auth.JWTManager,
	hub *ws.Hub,
	authHandler *handler.AuthHandler,
	versionConfig config.VersionConfig,
	logger *slog.Logger,
	allowedOrigins []string,
) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.RequestLogger(logger))
	r.Use(chimw.Recoverer)
	
	// Convert config.VersionConfig to middleware.VersionConfig
	middlewareVersionConfig := middleware.VersionConfig{
		DefaultVersion:     versionConfig.DefaultVersion,
		DeprecationWarning: versionConfig.DeprecationWarning,
		SupportedVersions:  make([]middleware.APIVersion, len(versionConfig.SupportedVersions)),
	}
	for i, v := range versionConfig.SupportedVersions {
		middlewareVersionConfig.SupportedVersions[i] = middleware.APIVersion{
			Version:      v.Version,
			IsDefault:    v.IsDefault,
			IsDeprecated: v.IsDeprecated,
			DeprecatedAt: v.DeprecatedAt,
			SunsetAt:     v.SunsetAt,
		}
	}
	
	// Add API versioning middleware
	r.Use(middleware.NewVersionMiddleware(middlewareVersionConfig).VersionHandler())
	
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "API-Version"},
		ExposedHeaders:   []string{"Link", "API-Version", "Deprecation", "Warning", "Deprecation-Date", "Sunset"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Create handlers
	orgHandler := handler.NewOrgHandler(stores.Orgs, logger)
	agentHandler := handler.NewAgentHandler(stores.Agents, stores.APIKeys, logger)
	eventHandler := handler.NewEventHandler(stores.Events, stores.Agents, hub, logger)
	apikeyHandler := handler.NewAPIKeyHandler(stores.APIKeys, logger)
	invitationHandler := handler.NewInvitationHandler(stores.Invitations, stores.Orgs, stores.Users, logger)
	auditHandler := handler.NewAuditHandler(stores.AuditLogs, logger)
	wsHandler := handler.NewWSHandler(hub, jwtMgr, logger)
	openAPIHandler := handler.NewOpenAPIHandler()
	configHandler := handler.NewConfigHandler(stores.Configs, stores.Agents, logger)
	releaseHandler := handler.NewReleaseHandler("gaskaj", "OpenAgentFramework", logger)

	r.Route("/api/v1", func(r chi.Router) {
		// OpenAPI specification (public)
		r.Get("/openapi.json", openAPIHandler.HandleSpec)

		// Latest release info (public, cached)
		r.Get("/releases/latest", releaseHandler.HandleLatestRelease)

		// Health check
		r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
			if err := stores.Ping(r.Context()); err != nil {
				http.Error(w, `{"status":"unhealthy"}`, http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"healthy"}`))
		})

		// Auth routes (public)
		r.Mount("/auth", authHandler.Routes())

		// Invitation acceptance (public)
		r.Post("/invitations/accept", invitationHandler.HandleAccept)

		// Agent ingestion routes (API key auth)
		r.Route("/ingest", func(r chi.Router) {
			r.Use(middleware.RequireAPIKey(stores.APIKeys))
			r.Post("/register", eventHandler.HandleIngestRegister)
			r.Post("/events", eventHandler.HandleIngestSingle)
			r.Post("/events/batch", eventHandler.HandleIngestBatch)
			r.Post("/heartbeat", eventHandler.HandleIngestHeartbeat)
			r.Post("/logs", eventHandler.HandleIngestLogs)
			r.Get("/config", configHandler.HandleIngestConfig)
		})

		// Protected routes (JWT auth)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAuth(jwtMgr))

			// Organization CRUD
			r.Post("/orgs", orgHandler.HandleCreate)
			r.Get("/orgs", orgHandler.HandleList)

			// Org-scoped routes
			r.Route("/orgs/{orgSlug}", func(r chi.Router) {
				r.Use(middleware.RequireOrgAccess(stores.Orgs))

				r.Get("/", orgHandler.HandleGet)
				r.Put("/", orgHandler.HandleUpdate)
				r.Delete("/", orgHandler.HandleDelete)

				// Members
				r.Get("/members", orgHandler.HandleListMembers)
				r.Put("/members/{userId}", orgHandler.HandleUpdateMemberRole)
				r.Delete("/members/{userId}", orgHandler.HandleRemoveMember)

				// Agents
				r.Post("/agents", agentHandler.HandleRegister)
				r.Post("/agents/provision", agentHandler.HandleProvision)
				r.Get("/agents", agentHandler.HandleList)
				r.Get("/agents/{agentId}", agentHandler.HandleGet)
				r.Put("/agents/{agentId}", agentHandler.HandleUpdate)
				r.Delete("/agents/{agentId}", agentHandler.HandleDelete)

				// Events
				r.Get("/events", eventHandler.HandleQuery)
				r.Get("/events/stats", eventHandler.HandleStats)
				r.Get("/agents/{agentId}/events", eventHandler.HandleAgentEvents)

				// API Keys
				r.Post("/apikeys", apikeyHandler.HandleCreate)
				r.Get("/apikeys", apikeyHandler.HandleList)
				r.Delete("/apikeys/{keyId}", apikeyHandler.HandleRevoke)

				// Invitations
				r.Post("/invitations", invitationHandler.HandleCreate)
				r.Get("/invitations", invitationHandler.HandleList)
				r.Delete("/invitations/{invId}", invitationHandler.HandleCancel)

				// Audit logs
				r.Get("/audit", auditHandler.HandleQuery)

				// Configuration management
				r.Route("/config", func(r chi.Router) {
					r.Get("/types", configHandler.HandleListAgentTypeConfigs)
					r.Get("/types/{agentType}", configHandler.HandleGetAgentTypeConfig)
					r.Put("/types/{agentType}", configHandler.HandleUpsertAgentTypeConfig)
					r.Get("/agents/{agentId}", configHandler.HandleGetAgentOverride)
					r.Put("/agents/{agentId}", configHandler.HandleUpsertAgentOverride)
					r.Delete("/agents/{agentId}", configHandler.HandleDeleteAgentOverride)
					r.Get("/agents/{agentId}/merged", configHandler.HandleGetMergedConfig)
					r.Get("/audit", configHandler.HandleConfigAudit)
				})

				// WebSocket
				r.Get("/ws", wsHandler.HandleConnect)
			})
		})
	})

	return r
}
