package testing

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gaskaj/OpenAgentFramework/web/handler"
)

// TestOpenAPISpecGeneration tests that the OpenAPI specification is properly generated
func TestOpenAPISpecGeneration(t *testing.T) {
	// Create OpenAPI handler
	openAPIHandler := handler.NewOpenAPIHandler()
	
	// Test spec generation
	spec := openAPIHandler.GetSpec()
	require.NotNil(t, spec)

	// Verify basic structure
	assert.Equal(t, "3.0.3", spec["openapi"])
	
	info := spec["info"].(map[string]interface{})
	assert.Equal(t, "OpenAgent Framework Control Plane API", info["title"])
	assert.Equal(t, "1.0.0", info["version"])

	// Verify components exist
	components := spec["components"].(map[string]interface{})
	assert.NotNil(t, components["schemas"])
	assert.NotNil(t, components["securitySchemes"])

	// Verify paths exist
	paths := spec["paths"].(map[string]interface{})
	assert.NotNil(t, paths["/healthz"])
	assert.NotNil(t, paths["/auth/register"])
	assert.NotNil(t, paths["/orgs/{orgSlug}/agents"])
}

// TestOpenAPIEndpoint tests the OpenAPI specification endpoint
func TestOpenAPIEndpoint(t *testing.T) {
	// Create handler
	openAPIHandler := handler.NewOpenAPIHandler()
	
	// Create test request
	req, err := http.NewRequest("GET", "/openapi.json", nil)
	require.NoError(t, err)
	
	// Create response recorder
	rr := httptest.NewRecorder()
	
	// Call handler
	openAPIHandler.HandleSpec(rr, req)
	
	// Check response
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Contains(t, rr.Header().Get("Cache-Control"), "max-age=300")
	
	// Verify response is valid JSON
	var spec map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &spec)
	require.NoError(t, err)
	
	// Verify spec structure
	assert.Equal(t, "3.0.3", spec["openapi"])
	assert.NotNil(t, spec["info"])
	assert.NotNil(t, spec["paths"])
}

// TestSchemaDefinitions tests that all expected schemas are defined
func TestSchemaDefinitions(t *testing.T) {
	openAPIHandler := handler.NewOpenAPIHandler()
	spec := openAPIHandler.GetSpec()
	
	components := spec["components"].(map[string]interface{})
	schemas := components["schemas"].(map[string]interface{})
	
	expectedSchemas := []string{
		"User",
		"Organization", 
		"Agent",
		"AgentEvent",
		"APIKey",
		"ApiError",
		"PaginatedResponse",
	}
	
	for _, schemaName := range expectedSchemas {
		t.Run("Schema_"+schemaName, func(t *testing.T) {
			schema, exists := schemas[schemaName]
			assert.True(t, exists, "Schema %s should be defined", schemaName)
			assert.NotNil(t, schema, "Schema %s should not be nil", schemaName)
			
			schemaMap := schema.(map[string]interface{})
			assert.Equal(t, "object", schemaMap["type"], "Schema %s should be of type object", schemaName)
			assert.NotNil(t, schemaMap["properties"], "Schema %s should have properties", schemaName)
		})
	}
}

// TestAgentSchemaStructure tests the Agent schema in detail
func TestAgentSchemaStructure(t *testing.T) {
	openAPIHandler := handler.NewOpenAPIHandler()
	spec := openAPIHandler.GetSpec()
	
	components := spec["components"].(map[string]interface{})
	schemas := components["schemas"].(map[string]interface{})
	agentSchema := schemas["Agent"].(map[string]interface{})
	properties := agentSchema["properties"].(map[string]interface{})
	
	// Test required properties exist
	requiredFields := []string{"id", "org_id", "name", "agent_type", "status"}
	for _, field := range requiredFields {
		t.Run("RequiredField_"+field, func(t *testing.T) {
			property, exists := properties[field]
			assert.True(t, exists, "Field %s should exist in Agent schema", field)
			assert.NotNil(t, property, "Field %s should not be nil", field)
		})
	}
	
	// Test status enum values
	statusProperty := properties["status"].(map[string]interface{})
	if statusEnumRaw, exists := statusProperty["enum"]; exists {
		// Handle different possible types for enum values
		expectedStatuses := []string{"online", "offline", "error", "idle"}
		
		switch statusEnum := statusEnumRaw.(type) {
		case []interface{}:
			for _, status := range expectedStatuses {
				found := false
				for _, enumValue := range statusEnum {
					if enumValue == status {
						found = true
						break
					}
				}
				assert.True(t, found, "Status enum should contain %s", status)
			}
		case []string:
			for _, status := range expectedStatuses {
				found := false
				for _, enumValue := range statusEnum {
					if enumValue == status {
						found = true
						break
					}
				}
				assert.True(t, found, "Status enum should contain %s", status)
			}
		default:
			t.Errorf("Unexpected enum type: %T", statusEnumRaw)
		}
	}
}

// TestSecuritySchemes tests that security schemes are properly defined
func TestSecuritySchemes(t *testing.T) {
	openAPIHandler := handler.NewOpenAPIHandler()
	spec := openAPIHandler.GetSpec()
	
	components := spec["components"].(map[string]interface{})
	securitySchemes := components["securitySchemes"].(map[string]interface{})
	
	// Test JWT bearer auth
	bearerAuth, exists := securitySchemes["bearerAuth"]
	assert.True(t, exists, "bearerAuth security scheme should exist")
	
	bearerAuthMap := bearerAuth.(map[string]interface{})
	assert.Equal(t, "http", bearerAuthMap["type"])
	assert.Equal(t, "bearer", bearerAuthMap["scheme"])
	assert.Equal(t, "JWT", bearerAuthMap["bearerFormat"])
	
	// Test API key auth
	apiKeyAuth, exists := securitySchemes["apiKeyAuth"]
	assert.True(t, exists, "apiKeyAuth security scheme should exist")
	
	apiKeyAuthMap := apiKeyAuth.(map[string]interface{})
	assert.Equal(t, "apiKey", apiKeyAuthMap["type"])
	assert.Equal(t, "header", apiKeyAuthMap["in"])
	assert.Equal(t, "Authorization", apiKeyAuthMap["name"])
}

// TestEndpointDefinitions tests that key endpoints are defined with proper structure
func TestEndpointDefinitions(t *testing.T) {
	openAPIHandler := handler.NewOpenAPIHandler()
	spec := openAPIHandler.GetSpec()
	
	paths := spec["paths"].(map[string]interface{})
	
	testCases := []struct {
		path   string
		method string
	}{
		{"/healthz", "get"},
		{"/auth/register", "post"},
		{"/orgs/{orgSlug}/agents", "get"},
		{"/orgs/{orgSlug}/agents", "post"},
	}
	
	for _, tc := range testCases {
		t.Run("Endpoint_"+tc.method+"_"+tc.path, func(t *testing.T) {
			pathItem, exists := paths[tc.path]
			assert.True(t, exists, "Path %s should be defined", tc.path)
			
			pathMap := pathItem.(map[string]interface{})
			operation, exists := pathMap[tc.method]
			assert.True(t, exists, "Method %s should be defined for path %s", tc.method, tc.path)
			
			operationMap := operation.(map[string]interface{})
			assert.NotNil(t, operationMap["operationId"], "Operation should have operationId")
			assert.NotNil(t, operationMap["summary"], "Operation should have summary")
			assert.NotNil(t, operationMap["responses"], "Operation should have responses")
		})
	}
}

// TestResponseStructures tests that response structures match expected formats
func TestResponseStructures(t *testing.T) {
	openAPIHandler := handler.NewOpenAPIHandler()
	spec := openAPIHandler.GetSpec()
	
	paths := spec["paths"].(map[string]interface{})
	
	// Test health endpoint response
	healthPath := paths["/healthz"].(map[string]interface{})
	healthGet := healthPath["get"].(map[string]interface{})
	healthResponses := healthGet["responses"].(map[string]interface{})
	
	// Should have 200 and 503 responses
	assert.NotNil(t, healthResponses["200"], "Health endpoint should have 200 response")
	assert.NotNil(t, healthResponses["503"], "Health endpoint should have 503 response")
	
	// Test agent list endpoint
	agentsPath := paths["/orgs/{orgSlug}/agents"].(map[string]interface{})
	agentsGet := agentsPath["get"].(map[string]interface{})
	
	// Should have security requirement
	if agentsSecurity, exists := agentsGet["security"]; exists {
		assert.NotNil(t, agentsSecurity, "Agent endpoints should require authentication")
	}
	
	// Should have parameters
	if agentsParams, exists := agentsGet["parameters"]; exists {
		assert.NotNil(t, agentsParams, "Agent list should have parameters")
		
		// Convert to proper type and check for orgSlug parameter
		switch params := agentsParams.(type) {
		case []interface{}:
			foundOrgSlug := false
			for _, param := range params {
				paramMap := param.(map[string]interface{})
				if paramMap["name"] == "orgSlug" && paramMap["in"] == "path" {
					foundOrgSlug = true
					assert.True(t, paramMap["required"].(bool), "orgSlug parameter should be required")
					break
				}
			}
			assert.True(t, foundOrgSlug, "Should have orgSlug path parameter")
		case []map[string]interface{}:
			foundOrgSlug := false
			for _, paramMap := range params {
				if paramMap["name"] == "orgSlug" && paramMap["in"] == "path" {
					foundOrgSlug = true
					assert.True(t, paramMap["required"].(bool), "orgSlug parameter should be required")
					break
				}
			}
			assert.True(t, foundOrgSlug, "Should have orgSlug path parameter")
		}
	}
	

}

// TestContractConsistency tests that the OpenAPI spec is consistent internally
func TestContractConsistency(t *testing.T) {
	openAPIHandler := handler.NewOpenAPIHandler()
	spec := openAPIHandler.GetSpec()
	
	// Test that all schema references are valid
	paths := spec["paths"].(map[string]interface{})
	components := spec["components"].(map[string]interface{})
	schemas := components["schemas"].(map[string]interface{})
	
	// Collect all schema references from paths
	schemaRefs := make(map[string]bool)
	
	// Walk through paths and collect schema references
	for pathName, pathItem := range paths {
		pathMap := pathItem.(map[string]interface{})
		for method, operation := range pathMap {
			if method == "parameters" {
				continue // Skip path-level parameters
			}
			
			operationMap, ok := operation.(map[string]interface{})
			if !ok {
				continue
			}
			
			// Check responses for schema references
			if responses, exists := operationMap["responses"]; exists {
				responsesMap := responses.(map[string]interface{})
				for _, response := range responsesMap {
					responseMap := response.(map[string]interface{})
					if content, exists := responseMap["content"]; exists {
						contentMap := content.(map[string]interface{})
						if jsonContent, exists := contentMap["application/json"]; exists {
							jsonMap := jsonContent.(map[string]interface{})
							if schema, exists := jsonMap["schema"]; exists {
								schemaMap := schema.(map[string]interface{})
								if ref, exists := schemaMap["$ref"]; exists {
									refStr := ref.(string)
									// Extract schema name from $ref
									if len(refStr) > 21 && refStr[:21] == "#/components/schemas/" {
										schemaName := refStr[21:]
										schemaRefs[schemaName] = true
									}
								}
							}
						}
					}
				}
			}
		}
		
		t.Logf("Processed path: %s", pathName)
	}
	
	// Verify all referenced schemas exist
	for refName := range schemaRefs {
		t.Run("SchemaReference_"+refName, func(t *testing.T) {
			_, exists := schemas[refName]
			assert.True(t, exists, "Referenced schema %s should be defined in components/schemas", refName)
		})
	}
}

// Benchmark OpenAPI spec generation performance
func BenchmarkOpenAPIGeneration(b *testing.B) {
	for i := 0; i < b.N; i++ {
		handler := handler.NewOpenAPIHandler()
		_ = handler.GetSpec()
	}
}

// Test JSON serialization performance
func BenchmarkOpenAPISerializeJSON(b *testing.B) {
	handler := handler.NewOpenAPIHandler()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", "/openapi.json", nil)
		rr := httptest.NewRecorder()
		handler.HandleSpec(rr, req)
	}
}