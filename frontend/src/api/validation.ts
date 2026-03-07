import { AxiosResponse } from 'axios';
import { z } from 'zod';
import type { 
  User, 
  Organization, 
  Agent, 
  AgentEvent, 
  APIKey, 
  Invitation,
  AuditLog,
  PaginatedResponse,
  ApiError,
  EventType,
  Severity,
  AgentStatus,
  OrgRole,
  InvitationStatus 
} from '@/types';

// Zod schemas for runtime validation matching TypeScript types

const EventTypeSchema = z.enum([
  'agent.registered',
  'agent.deregistered', 
  'agent.heartbeat',
  'agent.status_change',
  'agent.error',
  'issue.claimed',
  'issue.analyzed',
  'issue.decomposed', 
  'issue.implemented',
  'issue.pr_created',
  'issue.completed',
  'issue.failed',
  'workflow.started',
  'workflow.step_completed',
  'workflow.completed',
  'workflow.failed'
]);

const SeveritySchema = z.enum(['info', 'warning', 'error', 'critical']);
const AgentStatusSchema = z.enum(['online', 'offline', 'error', 'idle']);
const OrgRoleSchema = z.enum(['owner', 'admin', 'member', 'viewer']);
const InvitationStatusSchema = z.enum(['pending', 'accepted', 'expired', 'cancelled']);

const UserSchema = z.object({
  id: z.string().uuid(),
  email: z.string().email(),
  display_name: z.string(),
  avatar_url: z.string().url().optional(),
  provider: z.string().optional(),
  created_at: z.string().datetime(),
  updated_at: z.string().datetime(),
});

const OrganizationSchema = z.object({
  id: z.string().uuid(),
  name: z.string(),
  slug: z.string(),
  created_at: z.string().datetime(),
  updated_at: z.string().datetime(),
});

const AgentSchema = z.object({
  id: z.string().uuid(),
  org_id: z.string().uuid(),
  name: z.string(),
  agent_type: z.string(),
  status: AgentStatusSchema,
  version: z.string(),
  hostname: z.string(),
  github_repo: z.string(),
  tags: z.array(z.string()),
  config_snapshot: z.record(z.unknown()),
  last_heartbeat: z.string().datetime(),
  created_at: z.string().datetime(),
  updated_at: z.string().datetime(),
});

const AgentEventSchema = z.object({
  id: z.string().uuid(),
  org_id: z.string().uuid(),
  agent_id: z.string().uuid(),
  agent_name: z.string(),
  event_type: EventTypeSchema,
  severity: SeveritySchema,
  message: z.string(),
  metadata: z.record(z.unknown()),
  created_at: z.string().datetime(),
});

const APIKeySchema = z.object({
  id: z.string().uuid(),
  org_id: z.string().uuid(),
  name: z.string(),
  key_prefix: z.string(),
  created_by: z.string().uuid(),
  last_used_at: z.string().datetime().optional(),
  created_at: z.string().datetime(),
  revoked_at: z.string().datetime().optional(),
});

const InvitationSchema = z.object({
  id: z.string().uuid(),
  org_id: z.string().uuid(),
  email: z.string().email(),
  role: OrgRoleSchema,
  status: InvitationStatusSchema,
  invited_by: z.string().uuid(),
  invited_by_name: z.string().optional(),
  org_name: z.string().optional(),
  token: z.string(),
  expires_at: z.string().datetime(),
  created_at: z.string().datetime(),
});

const AuditLogSchema = z.object({
  id: z.string().uuid(),
  org_id: z.string().uuid(),
  user_id: z.string().uuid(),
  user_email: z.string().email(),
  action: z.string(),
  resource_type: z.string(),
  resource_id: z.string(),
  details: z.record(z.unknown()),
  ip_address: z.string(),
  created_at: z.string().datetime(),
});

const PaginatedResponseSchema = <T>(itemSchema: z.ZodType<T>) => z.object({
  data: z.array(itemSchema),
  total: z.number().int().nonnegative(),
  page: z.number().int().positive(),
  per_page: z.number().int().positive(),
  total_pages: z.number().int().nonnegative(),
});

const ApiErrorSchema = z.object({
  error: z.string(),
  message: z.string(),
  status: z.number().int(),
  details: z.record(z.string()).optional(),
});

// Response validation schemas for specific endpoints
const AuthResponseSchema = z.object({
  user: UserSchema,
  access_token: z.string(),
  refresh_token: z.string(),
});

const AgentListResponseSchema = PaginatedResponseSchema(AgentSchema);
const AgentCreateResponseSchema = z.object({
  data: AgentSchema,
});

// Schema registry for endpoint validation
const ENDPOINT_SCHEMAS = new Map<string, z.ZodType>([
  // Health check
  ['GET:/api/v1/healthz', z.object({ status: z.literal('healthy') })],
  
  // Authentication
  ['POST:/api/v1/auth/register', AuthResponseSchema],
  ['POST:/api/v1/auth/login', AuthResponseSchema],
  ['POST:/api/v1/auth/refresh', z.object({ access_token: z.string() })],
  ['GET:/api/v1/auth/me', z.object({ user: UserSchema, orgs: z.array(OrganizationSchema) })],
  
  // Agents
  ['GET:/api/v1/orgs/*/agents', AgentListResponseSchema],
  ['POST:/api/v1/orgs/*/agents', AgentCreateResponseSchema],
  ['GET:/api/v1/orgs/*/agents/*', AgentCreateResponseSchema],
  
  // Events  
  ['GET:/api/v1/orgs/*/events', PaginatedResponseSchema(AgentEventSchema)],
  
  // API Keys
  ['GET:/api/v1/orgs/*/apikeys', PaginatedResponseSchema(APIKeySchema)],
  ['POST:/api/v1/orgs/*/apikeys', z.object({ 
    data: APIKeySchema, 
    key: z.string() 
  })],
  
  // Organizations
  ['GET:/api/v1/orgs', PaginatedResponseSchema(OrganizationSchema)],
  ['POST:/api/v1/orgs', z.object({ data: OrganizationSchema })],
]);

/**
 * Validates an API response against the expected schema
 */
export function validateApiResponse(response: AxiosResponse): void {
  const { method, url } = response.config;
  if (!method || !url) return;

  // Extract endpoint pattern
  const endpoint = `${method.toUpperCase()}:${extractEndpointPattern(url)}`;
  
  const schema = ENDPOINT_SCHEMAS.get(endpoint);
  if (!schema) {
    // No validation schema defined for this endpoint
    return;
  }

  try {
    schema.parse(response.data);
  } catch (error) {
    if (error instanceof z.ZodError) {
      throw new Error(
        `API contract violation for ${endpoint}: ${error.errors
          .map(e => `${e.path.join('.'): ${e.message}`)
          .join(', ')}`
      );
    }
    throw error;
  }
}

/**
 * Extracts a pattern from a URL for schema matching
 * Converts /api/v1/orgs/123/agents to /api/v1/orgs/*/agents
 */
function extractEndpointPattern(url: string): string {
  // Remove query parameters
  const baseUrl = url.split('?')[0];
  
  // Replace UUID patterns with wildcard
  const uuidRegex = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi;
  let pattern = baseUrl.replace(uuidRegex, '*');
  
  // Replace other ID-like patterns (slug, etc.)
  pattern = pattern.replace(/\/[^/]+\/agents/g, '/*/agents');
  pattern = pattern.replace(/\/[^/]+\/events/g, '/*/events');
  pattern = pattern.replace(/\/[^/]+\/apikeys/g, '/*/apikeys');
  pattern = pattern.replace(/\/orgs\/[^/]+$/g, '/orgs/*');
  
  return pattern;
}

/**
 * Type guard to check if a response is an API error
 */
export function isApiError(data: unknown): data is ApiError {
  try {
    ApiErrorSchema.parse(data);
    return true;
  } catch {
    return false;
  }
}

/**
 * Validates that frontend types match expected API response structure
 */
export function validateTypeCompatibility() {
  // This would be called during build/test to ensure types match
  // For now, we rely on the schema validation at runtime
  
  console.info('API contract validation enabled');
}

// Export schemas for testing
export const schemas = {
  User: UserSchema,
  Organization: OrganizationSchema,  
  Agent: AgentSchema,
  AgentEvent: AgentEventSchema,
  APIKey: APIKeySchema,
  Invitation: InvitationSchema,
  AuditLog: AuditLogSchema,
  PaginatedResponse: PaginatedResponseSchema,
  ApiError: ApiErrorSchema,
  AuthResponse: AuthResponseSchema,
};