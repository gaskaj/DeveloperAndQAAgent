# API Contract Testing Guide

This guide covers the comprehensive API contract testing framework implemented in the OpenAgent Framework control plane, designed to ensure consistency between the backend API and frontend TypeScript interfaces.

## Overview

API contract testing validates that:

1. **API responses match TypeScript interfaces** defined in `frontend/src/types/index.ts`
2. **OpenAPI specifications** accurately describe actual API behavior
3. **Breaking changes are detected** before deployment
4. **Documentation stays in sync** with implementation

## Architecture

### Backend Components

#### OpenAPI Specification Generation
- **Location**: `web/handler/openapi.go`
- **Purpose**: Programmatically generates OpenAPI 3.0 specifications from Go handler implementations
- **Endpoint**: `GET /api/v1/openapi.json`
- **Features**:
  - Comprehensive schema definitions matching Go structs
  - Authentication requirements (JWT and API key)
  - Request/response validation schemas
  - Error response documentation

#### Contract Test Suite
- **Location**: `web/testing/contract_test.go`
- **Purpose**: Validates all API endpoints against OpenAPI specifications
- **Coverage**:
  - Request/response schema compliance
  - Authentication flow validation
  - API versioning behavior
  - Error response structure
  - Pagination compliance

### Frontend Components

#### Runtime Type Validation
- **Location**: `frontend/src/api/validation.ts`
- **Purpose**: Validates API responses match TypeScript types at runtime
- **Features**:
  - Zod schemas matching TypeScript interfaces
  - Automatic response validation in development
  - Contract violation reporting
  - Type compatibility testing

#### API Client Integration
- **Location**: `frontend/src/api/client.ts`
- **Enhancement**: Added response validation interceptor
- **Behavior**:
  - Validates responses in development mode
  - Logs contract violations without breaking requests
  - Maintains backward compatibility

#### Contract Test Suite
- **Location**: `frontend/src/test/api-contract.test.ts`
- **Purpose**: Frontend-side contract validation testing
- **Coverage**:
  - Schema validation testing
  - TypeScript/runtime type compatibility
  - API client interceptor validation
  - Error handling verification

## Usage

### Running Contract Tests

#### Backend Tests
```bash
# Run all contract tests
go test ./web/testing -v

# Run specific contract test
go test ./web/testing -v -run TestAPIContractCompliance

# Run with coverage
go test ./web/testing -coverprofile=contract-coverage.out
```

#### Frontend Tests
```bash
# Run contract validation tests
npm run test:contract

# Generate TypeScript types from OpenAPI spec (requires running backend)
npm run generate-types

# Full contract validation pipeline
npm run validate-contracts
```

### Adding New Endpoints

When adding a new API endpoint, follow these steps:

#### 1. Backend Implementation

Add OpenAPI documentation to your handler:

```go
// In web/handler/openapi.go, add to generatePaths()
paths["/orgs/{orgSlug}/new-resource"] = &openapi3.PathItem{
    Post: &openapi3.Operation{
        OperationID: "createNewResource",
        Summary:     "Create new resource",
        Description: "Creates a new resource in the organization",
        Security:    []openapi3.SecurityRequirement{{"bearerAuth": {}}},
        Parameters: []*openapi3.ParameterRef{
            {Value: &openapi3.Parameter{
                Name: "orgSlug", 
                In: "path", 
                Required: true, 
                Schema: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: "string"}},
            }},
        },
        RequestBody: &openapi3.RequestBodyRef{
            Value: &openapi3.RequestBody{
                Required: true,
                Content: map[string]*openapi3.MediaType{
                    "application/json": {
                        Schema: &openapi3.SchemaRef{Ref: "#/components/schemas/NewResourceRequest"},
                    },
                },
            },
        },
        Responses: map[string]*openapi3.ResponseRef{
            "201": {Value: &openapi3.Response{
                Description: openapi3.Ptr("Resource created successfully"),
                Content: map[string]*openapi3.MediaType{
                    "application/json": {
                        Schema: &openapi3.SchemaRef{Ref: "#/components/schemas/NewResource"},
                    },
                },
            }},
        },
    },
}
```

Add corresponding schema definitions:

```go
// In generateSchemas()
schemas["NewResource"] = &openapi3.SchemaRef{
    Value: &openapi3.Schema{
        Type: "object",
        Properties: map[string]*openapi3.SchemaRef{
            "id":   {Value: &openapi3.Schema{Type: "string", Format: "uuid"}},
            "name": {Value: &openapi3.Schema{Type: "string"}},
            // ... other properties
        },
        Required: []string{"id", "name"},
    },
}
```

#### 2. Contract Test Coverage

Add test cases to `web/testing/contract_test.go`:

```go
t.Run("New Resource Management", func(t *testing.T) {
    userID := uuid.New()
    orgClaims := []auth.OrgClaim{{ID: uuid.New(), Slug: "test-org", Role: "owner"}}
    authHeaders := suite.createAuthHeaders(userID, "test@example.com", orgClaims)

    // Test resource creation
    resourceReq := map[string]interface{}{
        "name": "Test Resource",
        "type": "example",
    }
    suite.TestEndpointContract("POST", "/api/v1/orgs/test-org/new-resource", resourceReq, authHeaders, http.StatusCreated)
    
    // Test resource listing
    suite.TestEndpointContract("GET", "/api/v1/orgs/test-org/new-resource", nil, authHeaders, http.StatusOK)
})
```

#### 3. Frontend Type Integration

Add TypeScript interface to `frontend/src/types/index.ts`:

```typescript
export interface NewResource {
  id: string;
  name: string;
  type: string;
  created_at: string;
  updated_at: string;
}
```

Add validation schema to `frontend/src/api/validation.ts`:

```typescript
const NewResourceSchema = z.object({
  id: z.string().uuid(),
  name: z.string(),
  type: z.string(),
  created_at: z.string().datetime(),
  updated_at: z.string().datetime(),
});

// Add to ENDPOINT_SCHEMAS
['POST:/api/v1/orgs/*/new-resource', z.object({ data: NewResourceSchema })],
['GET:/api/v1/orgs/*/new-resource', PaginatedResponseSchema(NewResourceSchema)],
```

Add test cases to `frontend/src/test/api-contract.test.ts`:

```typescript
it('should validate NewResource schema correctly', () => {
  const validResource: NewResource = {
    id: '123e4567-e89b-12d3-a456-426614174000',
    name: 'Test Resource',
    type: 'example',
    created_at: '2023-01-01T12:00:00Z',
    updated_at: '2023-01-01T12:00:00Z',
  };

  expect(() => schemas.NewResource.parse(validResource)).not.toThrow();
});
```

## Configuration

### Development Environment

Contract validation runs automatically in development mode. Set environment variables:

```env
# Enable detailed contract validation logging
VITE_API_CONTRACT_VALIDATION=true

# API base URL for type generation
VITE_API_BASE_URL=http://localhost:3001/api/v1
```

### CI/CD Integration

Add contract testing to your CI pipeline:

```yaml
# .github/workflows/test.yml
- name: Backend Contract Tests
  run: go test ./web/testing -v

- name: Frontend Contract Tests  
  run: |
    cd frontend
    npm run validate-contracts
```

### Makefile Integration

Update `Makefile` with contract testing targets:

```makefile
# Contract testing targets
test-contract:
	go test -v ./web/testing
	cd frontend && npm run test:contract

validate-contracts:
	@echo "Running comprehensive API contract validation..."
	go test -v ./web/testing
	cd frontend && npm run validate-contracts

generate-api-spec:
	@echo "Generating OpenAPI specification..."
	go run cmd/controlplane --generate-spec > api-spec.json

# Add to existing test targets
test-all: test-unit test-integration test-contract
```

## Best Practices

### Schema Design

1. **Use consistent field naming**: Follow snake_case in API, camelCase in TypeScript
2. **Required fields**: Mark all non-nullable fields as required in OpenAPI
3. **Type precision**: Use specific types (uuid, email, datetime) instead of generic strings
4. **Enums**: Define strict enums for status fields and other constrained values

### Testing Strategy

1. **Test positive and negative cases**: Include both valid and invalid request/response scenarios
2. **Authentication testing**: Verify all auth flows (JWT, API key, unauthorized)
3. **Edge cases**: Test pagination limits, empty responses, error conditions
4. **Version compatibility**: Test API versioning behavior

### Error Handling

1. **Consistent error format**: All errors should follow the `ApiError` schema
2. **Status codes**: Use appropriate HTTP status codes (400, 401, 403, 404, 500)
3. **Error details**: Provide specific validation error details in the `details` field

### Documentation

1. **Keep OpenAPI spec updated**: Add documentation for every new endpoint
2. **Include examples**: Provide request/response examples in OpenAPI spec
3. **Document authentication**: Clear auth requirements for each endpoint
4. **Version changelog**: Document breaking changes and migration paths

## Troubleshooting

### Common Issues

#### Contract Test Failures

```
Response validation failed for POST /api/v1/orgs/test/agents (status 201): response body doesn't match the schema
```

**Solution**: Check that the actual response structure matches the OpenAPI schema definition. Verify required fields and data types.

#### Frontend Validation Errors

```
API contract violation for GET:/api/v1/orgs/*/agents: data.0.status: Invalid enum value
```

**Solution**: Ensure the enum values in Zod schema match those in TypeScript types and OpenAPI spec.

#### Missing Schema Definitions

```
Warning: Could not find OpenAPI route for GET /api/v1/new-endpoint
```

**Solution**: Add the endpoint to the OpenAPI specification in `web/handler/openapi.go`.

### Debug Mode

Enable verbose contract validation:

```bash
# Backend
OPENAPI_VALIDATION_VERBOSE=true go test ./web/testing -v

# Frontend  
VITE_API_CONTRACT_DEBUG=true npm run test:contract
```

### Schema Validation

Validate your OpenAPI spec:

```bash
# Install openapi-generator-cli
npm install -g @openapitools/openapi-generator-cli

# Validate spec
openapi-generator-cli validate -i http://localhost:3001/api/v1/openapi.json
```

## Integration with Quality Gates

Contract tests integrate with the existing quality assurance framework:

1. **Coverage Requirements**: Contract tests count toward test coverage metrics
2. **Quality Gates**: Failed contract tests block deployment
3. **Performance Impact**: Contract validation adds minimal overhead in development
4. **Monitoring**: Contract violations are logged for production monitoring

## Future Enhancements

- **Auto-generated TypeScript types**: Generate types directly from OpenAPI spec
- **Consumer-driven contracts**: Support for consumer-driven contract testing
- **Mock server generation**: Generate mock servers from OpenAPI specs
- **Performance contract testing**: Validate API performance characteristics
- **Breaking change detection**: Automated detection of backward compatibility issues

## Related Documentation

- [API Versioning](../webui/api-versioning.md) - API version management strategies
- [Testing Strategy](testing-strategy.md) - Overall testing approach
- [WebUI Architecture](../webui/webui-architecture.md) - Frontend/backend architecture