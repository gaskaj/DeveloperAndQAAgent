// ---- Union types matching Go constants ----

export type EventType =
  | 'agent.registered'
  | 'agent.deregistered'
  | 'agent.heartbeat'
  | 'agent.status_change'
  | 'agent.error'
  | 'issue.claimed'
  | 'issue.analyzed'
  | 'issue.decomposed'
  | 'issue.implemented'
  | 'issue.pr_created'
  | 'issue.completed'
  | 'issue.failed'
  | 'workflow.started'
  | 'workflow.step_completed'
  | 'workflow.completed'
  | 'workflow.failed';

export type Severity = 'info' | 'warning' | 'error' | 'critical';

export type AgentStatus = 'online' | 'offline' | 'error' | 'idle';

export type OrgRole = 'owner' | 'admin' | 'member' | 'viewer';

export type InvitationStatus = 'pending' | 'accepted' | 'expired' | 'cancelled';

// ---- Core models ----

export interface User {
  id: string;
  email: string;
  display_name: string;
  avatar_url?: string;
  provider?: string;
  created_at: string;
  updated_at: string;
}

export interface Organization {
  id: string;
  name: string;
  slug: string;
  created_at: string;
  updated_at: string;
}

export interface OrgMember {
  id: string;
  user_id: string;
  org_id: string;
  role: OrgRole;
  joined_at: string;
  email?: string;
  display_name?: string;
  avatar_url?: string;
}

export interface Agent {
  id: string;
  org_id: string;
  name: string;
  agent_type: string;
  status: AgentStatus;
  version: string;
  hostname: string;
  github_repo: string;
  tags: string[];
  config_snapshot: Record<string, unknown>;
  last_heartbeat: string;
  created_at: string;
  updated_at: string;
}

export interface AgentEvent {
  id: string;
  org_id: string;
  agent_id: string;
  agent_name: string;
  event_type: EventType;
  severity: Severity;
  message: string;
  metadata: Record<string, unknown>;
  created_at: string;
}

export interface APIKey {
  id: string;
  org_id: string;
  name: string;
  key_prefix: string;
  created_by: string;
  last_used_at?: string;
  created_at: string;
  revoked_at?: string;
}

export interface APIKeyCreateResponse {
  data: APIKey;
  key: string;
}

export interface Invitation {
  id: string;
  org_id: string;
  email: string;
  role: OrgRole;
  status: InvitationStatus;
  invited_by: string;
  invited_by_name?: string;
  org_name?: string;
  token: string;
  expires_at: string;
  created_at: string;
}

export interface AuditLog {
  id: string;
  org_id: string;
  user_id: string;
  user_email: string;
  action: string;
  resource_type: string;
  resource_id: string;
  details: Record<string, unknown>;
  ip_address: string;
  created_at: string;
}

// ---- API response wrappers ----

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface ApiError {
  error: string;
  message: string;
  status: number;
  details?: Record<string, string>;
}

// ---- Filter / query types ----

export interface EventFilters {
  event_type?: EventType;
  severity?: Severity;
  agent_id?: string;
  search?: string;
  from?: string;
  to?: string;
  page?: number;
  per_page?: number;
}

export interface AgentFilters {
  status?: AgentStatus;
  search?: string;
  agent_type?: string;
  page?: number;
  per_page?: number;
}

export interface AuditFilters {
  action?: string;
  user_id?: string;
  resource_type?: string;
  page?: number;
  per_page?: number;
}

export interface EventStats {
  total_events: number;
  events_today: number;
  events_by_type: Record<string, number>;
  events_by_severity: Record<string, number>;
  agents_online: number;
  agents_total: number;
  issues_processed_today: number;
  prs_created_today: number;
}
