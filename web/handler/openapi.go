package handler

import (
	"encoding/json"
	"net/http"
)

// OpenAPIHandler serves a simple OpenAPI specification
type OpenAPISimpleHandler struct {
	spec map[string]interface{}
}

// NewOpenAPIHandler creates a new simplified OpenAPI handler
func NewOpenAPIHandler() *OpenAPISimpleHandler {
	return &OpenAPISimpleHandler{
		spec: generateSimpleOpenAPISpec(),
	}
}

// HandleSpec serves the OpenAPI specification as JSON
func (h *OpenAPISimpleHandler) HandleSpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300") // Cache for 5 minutes
	
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(h.spec); err != nil {
		http.Error(w, "Failed to generate OpenAPI spec", http.StatusInternalServerError)
		return
	}
}

// GetSpec returns the OpenAPI specification for testing
func (h *OpenAPISimpleHandler) GetSpec() map[string]interface{} {
	return h.spec
}

// generateSimpleOpenAPISpec creates a basic OpenAPI specification
func generateSimpleOpenAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title":       "OpenAgent Framework Control Plane API",
			"description": "REST API for managing agents, events, and organizational resources",
			"version":     "1.0.0",
			"contact": map[string]interface{}{
				"name": "OpenAgent Framework Team",
			},
		},
		"servers": []map[string]interface{}{
			{
				"url":         "/api/v1",
				"description": "Production server",
			},
		},
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"bearerAuth": map[string]interface{}{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
					"description":  "JWT access token for authenticated users",
				},
				"apiKeyAuth": map[string]interface{}{
					"type":        "apiKey",
					"in":          "header",
					"name":        "Authorization",
					"description": "API key for agent ingestion endpoints (format: 'Bearer <api_key>')",
				},
			},
			"schemas": generateSimpleSchemas(),
		},
		"paths": generateSimplePaths(),
	}
}

// generateSimpleSchemas creates basic schema definitions
func generateSimpleSchemas() map[string]interface{} {
	return map[string]interface{}{
		"User": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":           map[string]interface{}{"type": "string", "format": "uuid"},
				"email":        map[string]interface{}{"type": "string", "format": "email"},
				"display_name": map[string]interface{}{"type": "string"},
				"avatar_url":   map[string]interface{}{"type": "string", "format": "uri"},
				"created_at":   map[string]interface{}{"type": "string", "format": "date-time"},
				"updated_at":   map[string]interface{}{"type": "string", "format": "date-time"},
			},
			"required": []string{"id", "email", "display_name", "created_at", "updated_at"},
		},
		"Organization": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":         map[string]interface{}{"type": "string", "format": "uuid"},
				"name":       map[string]interface{}{"type": "string"},
				"slug":       map[string]interface{}{"type": "string"},
				"plan":       map[string]interface{}{"type": "string", "enum": []string{"free", "pro", "enterprise"}},
				"created_at": map[string]interface{}{"type": "string", "format": "date-time"},
				"updated_at": map[string]interface{}{"type": "string", "format": "date-time"},
			},
			"required": []string{"id", "name", "slug", "plan", "created_at", "updated_at"},
		},
		"Agent": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":               map[string]interface{}{"type": "string", "format": "uuid"},
				"org_id":           map[string]interface{}{"type": "string", "format": "uuid"},
				"name":             map[string]interface{}{"type": "string"},
				"agent_type":       map[string]interface{}{"type": "string"},
				"status":           map[string]interface{}{"type": "string", "enum": []string{"online", "offline", "error", "idle"}},
				"version":          map[string]interface{}{"type": "string"},
				"hostname":         map[string]interface{}{"type": "string"},
				"github_owner":     map[string]interface{}{"type": "string"},
				"github_repo":      map[string]interface{}{"type": "string"},
				"description":      map[string]interface{}{"type": "string"},
				"tags":             map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				"config_snapshot":  map[string]interface{}{"type": "object", "additionalProperties": true},
				"last_heartbeat":   map[string]interface{}{"type": "string", "format": "date-time", "nullable": true},
				"created_at":       map[string]interface{}{"type": "string", "format": "date-time"},
				"updated_at":       map[string]interface{}{"type": "string", "format": "date-time"},
			},
			"required": []string{"id", "org_id", "name", "agent_type", "status", "created_at", "updated_at"},
		},
		"AgentEvent": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":         map[string]interface{}{"type": "string", "format": "uuid"},
				"org_id":     map[string]interface{}{"type": "string", "format": "uuid"},
				"agent_id":   map[string]interface{}{"type": "string", "format": "uuid"},
				"agent_name": map[string]interface{}{"type": "string"},
				"event_type": map[string]interface{}{
					"type": "string",
					"enum": []string{
						"agent.registered", "agent.deregistered", "agent.heartbeat", "agent.status_change", "agent.error",
						"issue.claimed", "issue.analyzed", "issue.decomposed", "issue.implemented", "issue.pr_created", "issue.completed", "issue.failed",
						"workflow.started", "workflow.step_completed", "workflow.completed", "workflow.failed",
					},
				},
				"severity":   map[string]interface{}{"type": "string", "enum": []string{"info", "warning", "error", "critical"}},
				"message":    map[string]interface{}{"type": "string"},
				"metadata":   map[string]interface{}{"type": "object", "additionalProperties": true},
				"created_at": map[string]interface{}{"type": "string", "format": "date-time"},
			},
			"required": []string{"id", "org_id", "agent_id", "agent_name", "event_type", "severity", "message", "created_at"},
		},
		"APIKey": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":           map[string]interface{}{"type": "string", "format": "uuid"},
				"org_id":       map[string]interface{}{"type": "string", "format": "uuid"},
				"name":         map[string]interface{}{"type": "string"},
				"key_prefix":   map[string]interface{}{"type": "string"},
				"created_by":   map[string]interface{}{"type": "string", "format": "uuid"},
				"last_used_at": map[string]interface{}{"type": "string", "format": "date-time", "nullable": true},
				"created_at":   map[string]interface{}{"type": "string", "format": "date-time"},
				"revoked_at":   map[string]interface{}{"type": "string", "format": "date-time", "nullable": true},
			},
			"required": []string{"id", "org_id", "name", "key_prefix", "created_by", "created_at"},
		},
		"ApiError": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"error":   map[string]interface{}{"type": "string"},
				"message": map[string]interface{}{"type": "string"},
				"status":  map[string]interface{}{"type": "integer"},
				"details": map[string]interface{}{"type": "object", "additionalProperties": map[string]interface{}{"type": "string"}},
			},
			"required": []string{"error", "message", "status"},
		},
		"PaginatedResponse": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"data":        map[string]interface{}{"type": "array", "items": map[string]interface{}{}},
				"total":       map[string]interface{}{"type": "integer"},
				"page":        map[string]interface{}{"type": "integer"},
				"per_page":    map[string]interface{}{"type": "integer"},
				"total_pages": map[string]interface{}{"type": "integer"},
			},
			"required": []string{"data", "total", "page", "per_page", "total_pages"},
		},
	}
}

// generateSimplePaths creates basic path definitions
func generateSimplePaths() map[string]interface{} {
	return map[string]interface{}{
		"/healthz": map[string]interface{}{
			"get": map[string]interface{}{
				"operationId": "getHealth",
				"summary":     "Health check",
				"description": "Check the health status of the API",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "Service is healthy",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"status": map[string]interface{}{"type": "string", "example": "healthy"},
									},
								},
							},
						},
					},
					"503": map[string]interface{}{
						"description": "Service is unhealthy",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{"$ref": "#/components/schemas/ApiError"},
							},
						},
					},
				},
			},
		},
		"/auth/register": map[string]interface{}{
			"post": map[string]interface{}{
				"operationId": "authRegister",
				"summary":     "Register new user",
				"description": "Register a new user account and create a default organization",
				"requestBody": map[string]interface{}{
					"required": true,
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"email":        map[string]interface{}{"type": "string", "format": "email"},
									"password":     map[string]interface{}{"type": "string", "minLength": 8},
									"display_name": map[string]interface{}{"type": "string"},
									"org_name":     map[string]interface{}{"type": "string"},
								},
								"required": []string{"email", "password", "display_name"},
							},
						},
					},
				},
				"responses": map[string]interface{}{
					"201": map[string]interface{}{
						"description": "User registered successfully",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"user":          map[string]interface{}{"$ref": "#/components/schemas/User"},
										"access_token":  map[string]interface{}{"type": "string"},
										"refresh_token": map[string]interface{}{"type": "string"},
									},
								},
							},
						},
					},
					"400": map[string]interface{}{
						"description": "Invalid request",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{"$ref": "#/components/schemas/ApiError"},
							},
						},
					},
				},
			},
		},
		"/orgs/{orgSlug}/agents": map[string]interface{}{
			"get": map[string]interface{}{
				"operationId": "listAgents",
				"summary":     "List agents",
				"description": "Get a list of agents for the organization",
				"security":    []map[string]interface{}{{"bearerAuth": []string{}}},
				"parameters": []map[string]interface{}{
					{
						"name":        "orgSlug",
						"in":          "path",
						"required":    true,
						"description": "Organization slug",
						"schema":      map[string]interface{}{"type": "string"},
					},
					{
						"name":        "limit",
						"in":          "query",
						"description": "Number of items to return",
						"schema":      map[string]interface{}{"type": "integer", "default": 20},
					},
					{
						"name":        "offset",
						"in":          "query",
						"description": "Number of items to skip",
						"schema":      map[string]interface{}{"type": "integer", "default": 0},
					},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "List of agents",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{"$ref": "#/components/schemas/PaginatedResponse"},
							},
						},
					},
				},
			},
			"post": map[string]interface{}{
				"operationId": "registerAgent",
				"summary":     "Register new agent",
				"description": "Register a new agent in the organization",
				"security":    []map[string]interface{}{{"bearerAuth": []string{}}},
				"parameters": []map[string]interface{}{
					{
						"name":        "orgSlug",
						"in":          "path",
						"required":    true,
						"description": "Organization slug",
						"schema":      map[string]interface{}{"type": "string"},
					},
				},
				"requestBody": map[string]interface{}{
					"required": true,
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"name":         map[string]interface{}{"type": "string"},
									"agent_type":   map[string]interface{}{"type": "string"},
									"description":  map[string]interface{}{"type": "string"},
									"github_owner": map[string]interface{}{"type": "string"},
									"github_repo":  map[string]interface{}{"type": "string"},
									"tags":         map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
								},
								"required": []string{"name", "agent_type"},
							},
						},
					},
				},
				"responses": map[string]interface{}{
					"201": map[string]interface{}{
						"description": "Agent registered successfully",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"data": map[string]interface{}{"$ref": "#/components/schemas/Agent"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}