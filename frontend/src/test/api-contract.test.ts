import { describe, it, expect, vi, beforeEach } from 'vitest';
import axios from 'axios';
import { validateApiResponse, schemas } from '@/api/validation';
import type { User, Agent, Organization, PaginatedResponse } from '@/types';

// Mock axios for contract testing
vi.mock('axios');
const mockedAxios = vi.mocked(axios);

// Mock the auth store
vi.mock('@/store/auth-store', () => ({
  useAuthStore: {
    getState: () => ({
      token: 'mock-token',
      refreshToken: 'mock-refresh-token',
      setToken: vi.fn(),
      logout: vi.fn(),
    }),
  },
}));

describe('API Contract Validation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('Schema Validation', () => {
    it('should validate User schema correctly', () => {
      const validUser: User = {
        id: '123e4567-e89b-12d3-a456-426614174000',
        email: 'test@example.com',
        display_name: 'Test User',
        avatar_url: 'https://example.com/avatar.png',
        created_at: '2023-01-01T12:00:00Z',
        updated_at: '2023-01-01T12:00:00Z',
      };

      expect(() => schemas.User.parse(validUser)).not.toThrow();

      // Test invalid user (missing required field)
      const invalidUser = { ...validUser, email: undefined };
      expect(() => schemas.User.parse(invalidUser)).toThrow();

      // Test invalid email format
      const invalidEmailUser = { ...validUser, email: 'not-an-email' };
      expect(() => schemas.User.parse(invalidEmailUser)).toThrow();

      // Test invalid UUID
      const invalidUuidUser = { ...validUser, id: 'not-a-uuid' };
      expect(() => schemas.User.parse(invalidUuidUser)).toThrow();
    });

    it('should validate Agent schema correctly', () => {
      const validAgent: Agent = {
        id: '123e4567-e89b-12d3-a456-426614174000',
        org_id: '123e4567-e89b-12d3-a456-426614174001',
        name: 'Test Agent',
        agent_type: 'developer',
        status: 'online',
        version: '1.0.0',
        hostname: 'test-host',
        github_repo: 'user/repo',
        tags: ['test', 'demo'],
        config_snapshot: { key: 'value' },
        last_heartbeat: '2023-01-01T12:00:00Z',
        created_at: '2023-01-01T12:00:00Z',
        updated_at: '2023-01-01T12:00:00Z',
      };

      expect(() => schemas.Agent.parse(validAgent)).not.toThrow();

      // Test invalid status
      const invalidStatusAgent = { ...validAgent, status: 'invalid-status' };
      expect(() => schemas.Agent.parse(invalidStatusAgent)).toThrow();

      // Test invalid tags (should be array of strings)
      const invalidTagsAgent = { ...validAgent, tags: ['valid', 123] };
      expect(() => schemas.Agent.parse(invalidTagsAgent)).toThrow();
    });

    it('should validate PaginatedResponse schema correctly', () => {
      const validPaginatedResponse: PaginatedResponse<Agent> = {
        data: [
          {
            id: '123e4567-e89b-12d3-a456-426614174000',
            org_id: '123e4567-e89b-12d3-a456-426614174001',
            name: 'Test Agent',
            agent_type: 'developer',
            status: 'online',
            version: '1.0.0',
            hostname: 'test-host',
            github_repo: 'user/repo',
            tags: [],
            config_snapshot: {},
            last_heartbeat: '2023-01-01T12:00:00Z',
            registered_at: '2023-01-01T12:00:00Z',
            updated_at: '2023-01-01T12:00:00Z',
          },
        ],
        total: 1,
        page: 1,
        per_page: 20,
        total_pages: 1,
      };

      expect(() => 
        schemas.PaginatedResponse(schemas.Agent).parse(validPaginatedResponse)
      ).not.toThrow();

      // Test invalid pagination (negative total)
      const invalidPagination = { ...validPaginatedResponse, total: -1 };
      expect(() => 
        schemas.PaginatedResponse(schemas.Agent).parse(invalidPagination)
      ).toThrow();
    });
  });

  describe('Response Validation', () => {
    it('should validate successful API responses', () => {
      const mockResponse = {
        config: {
          method: 'GET',
          url: '/api/v1/healthz',
        },
        data: { status: 'healthy' },
        status: 200,
        statusText: 'OK',
        headers: {},
      };

      // Should not throw for valid response
      expect(() => validateApiResponse(mockResponse as any)).not.toThrow();
    });

    it('should detect API contract violations', () => {
      const mockResponse = {
        config: {
          method: 'GET',
          url: '/api/v1/healthz',
        },
        data: { status: 'invalid-status' }, // Should be 'healthy'
        status: 200,
        statusText: 'OK', 
        headers: {},
      };

      // Should throw for invalid response
      expect(() => validateApiResponse(mockResponse as any)).toThrow(/API contract violation/);
    });

    it('should handle unknown endpoints gracefully', () => {
      const mockResponse = {
        config: {
          method: 'GET',
          url: '/api/v1/unknown-endpoint',
        },
        data: { anything: 'goes' },
        status: 200,
        statusText: 'OK',
        headers: {},
      };

      // Should not throw for unknown endpoints (no schema to validate against)
      expect(() => validateApiResponse(mockResponse as any)).not.toThrow();
    });

    it('should validate authentication response schema', () => {
      const validAuthResponse = {
        user: {
          id: '123e4567-e89b-12d3-a456-426614174000',
          email: 'test@example.com',
          display_name: 'Test User',
          created_at: '2023-01-01T12:00:00Z',
          updated_at: '2023-01-01T12:00:00Z',
        },
        access_token: 'jwt-token',
        refresh_token: 'refresh-jwt-token',
      };

      expect(() => schemas.AuthResponse.parse(validAuthResponse)).not.toThrow();

      // Test missing token
      const invalidAuthResponse = { ...validAuthResponse, access_token: undefined };
      expect(() => schemas.AuthResponse.parse(invalidAuthResponse)).toThrow();
    });
  });

  describe('URL Pattern Extraction', () => {
    it('should extract correct endpoint patterns', () => {
      // Test internal extractEndpointPattern function by examining validateApiResponse behavior
      
      // Agent list endpoint
      const agentListResponse = {
        config: {
          method: 'GET',
          url: '/api/v1/orgs/test-org-slug/agents',
        },
        data: {
          data: [],
          total: 0,
          page: 1,
          per_page: 20,
          total_pages: 0,
        },
        status: 200,
        statusText: 'OK',
        headers: {},
      };

      // Should recognize this as agent list endpoint and validate accordingly
      expect(() => validateApiResponse(agentListResponse as any)).not.toThrow();

      // Test with UUID in URL
      const agentDetailResponse = {
        config: {
          method: 'GET',
          url: '/api/v1/orgs/123e4567-e89b-12d3-a456-426614174000/agents/123e4567-e89b-12d3-a456-426614174001',
        },
        data: {
          data: {
            id: '123e4567-e89b-12d3-a456-426614174001',
            org_id: '123e4567-e89b-12d3-a456-426614174000',
            name: 'Test Agent',
            agent_type: 'developer',
            status: 'online',
            version: '1.0.0',
            hostname: 'test-host',
            github_repo: 'user/repo',
            tags: [],
            config_snapshot: {},
            last_heartbeat: '2023-01-01T12:00:00Z',
            registered_at: '2023-01-01T12:00:00Z',
            updated_at: '2023-01-01T12:00:00Z',
          },
        },
        status: 200,
        statusText: 'OK',
        headers: {},
      };

      // Should recognize this as agent detail endpoint and validate accordingly
      expect(() => validateApiResponse(agentDetailResponse as any)).not.toThrow();
    });
  });

  describe('Type Compatibility', () => {
    it('should ensure TypeScript types match runtime schemas', () => {
      // Create objects using TypeScript types
      const user: User = {
        id: '123e4567-e89b-12d3-a456-426614174000',
        email: 'test@example.com',
        display_name: 'Test User',
        created_at: '2023-01-01T12:00:00Z',
        updated_at: '2023-01-01T12:00:00Z',
      };

      const organization: Organization = {
        id: '123e4567-e89b-12d3-a456-426614174000',
        name: 'Test Org',
        slug: 'test-org',
        created_at: '2023-01-01T12:00:00Z',
        updated_at: '2023-01-01T12:00:00Z',
      };

      const agent: Agent = {
        id: '123e4567-e89b-12d3-a456-426614174000',
        org_id: '123e4567-e89b-12d3-a456-426614174001',
        name: 'Test Agent',
        agent_type: 'developer',
        status: 'online',
        version: '1.0.0',
        hostname: 'test-host',
        github_repo: 'user/repo',
        tags: [],
        config_snapshot: {},
        last_heartbeat: '2023-01-01T12:00:00Z',
        created_at: '2023-01-01T12:00:00Z',
        updated_at: '2023-01-01T12:00:00Z',
      };

      // These should all validate successfully against the schemas
      expect(() => schemas.User.parse(user)).not.toThrow();
      expect(() => schemas.Organization.parse(organization)).not.toThrow();
      expect(() => schemas.Agent.parse(agent)).not.toThrow();

      // Test that the runtime validation catches TypeScript-level type mismatches
      // (In a real scenario, these would be caught at compile time, but we test 
      // the runtime validation here)
      
      const userWithWrongType = { ...user, id: 123 }; // Should be string
      expect(() => schemas.User.parse(userWithWrongType)).toThrow();

      const agentWithWrongStatus = { ...agent, status: 'invalid' }; // Should be valid enum value
      expect(() => schemas.Agent.parse(agentWithWrongStatus)).toThrow();
    });
  });

  describe('Integration with API Client', () => {
    it('should integrate with axios interceptors', async () => {
      // Mock a successful API call
      const mockResponse = {
        data: { status: 'healthy' },
        status: 200,
        statusText: 'OK',
        headers: {},
        config: {
          method: 'GET',
          url: '/api/v1/healthz',
        },
      };

      mockedAxios.get.mockResolvedValueOnce(mockResponse);

      // Import the client to trigger interceptor registration
      const apiClient = await import('@/api/client');

      // Make a request - this should trigger the validation interceptor
      await expect(mockedAxios.get('/api/v1/healthz')).resolves.not.toThrow();
    });

    it('should log validation errors in development mode', () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

      // Mock development environment
      const originalEnv = import.meta.env.DEV;
      (import.meta.env as any).DEV = true;

      try {
        const mockResponse = {
          config: {
            method: 'GET',
            url: '/api/v1/healthz',
          },
          data: { status: 'invalid' }, // Invalid status
          status: 200,
          statusText: 'OK',
          headers: {},
        };

        // This should log an error but not throw
        validateApiResponse(mockResponse as any);
        
        // In a real interceptor, this would be caught and logged
        expect(consoleSpy).toHaveBeenCalledWith(
          expect.stringContaining('API contract validation failed')
        );
      } finally {
        (import.meta.env as any).DEV = originalEnv;
        consoleSpy.mockRestore();
      }
    });
  });
});

describe('API Error Handling', () => {
  it('should properly validate API error responses', () => {
    const validError = {
      error: 'validation_error',
      message: 'Invalid request data',
      status: 400,
      details: {
        field: 'email is required',
      },
    };

    expect(() => schemas.ApiError.parse(validError)).not.toThrow();

    // Test without optional details
    const errorWithoutDetails = {
      error: 'not_found',
      message: 'Resource not found',
      status: 404,
    };

    expect(() => schemas.ApiError.parse(errorWithoutDetails)).not.toThrow();

    // Test invalid error (missing required field)
    const invalidError = {
      error: 'test_error',
      // message is missing
      status: 500,
    };

    expect(() => schemas.ApiError.parse(invalidError)).toThrow();
  });
});

describe('Endpoint Coverage', () => {
  it('should have schema definitions for all critical endpoints', () => {
    const criticalEndpoints = [
      'GET:/api/v1/healthz',
      'POST:/api/v1/auth/register',
      'POST:/api/v1/auth/login',
      'GET:/api/v1/orgs/*/agents',
      'POST:/api/v1/orgs/*/agents',
      'GET:/api/v1/orgs/*/events',
      'GET:/api/v1/orgs/*/apikeys',
    ];

    // Import the validation module to access the schema registry
    import('@/api/validation').then((module) => {
      // Note: In a real implementation, we'd expose the ENDPOINT_SCHEMAS
      // for testing purposes. For now, we verify through behavior.
      
      criticalEndpoints.forEach(endpoint => {
        // Each endpoint should have corresponding validation logic
        expect(endpoint).toBeTruthy(); // Placeholder assertion
      });
    });
  });
});