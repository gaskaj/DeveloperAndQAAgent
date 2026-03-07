# WebUI Pages Reference

This document describes all pages in the OpenAgentFramework control plane WebUI, their features, and how to use them. Screenshots are captured automatically via Playwright (see `frontend/e2e/screenshots.spec.ts`).

## Public Pages (No Authentication Required)

### Login Page (`/login`)

![Login Page](screenshots/login.png)

The entry point for existing users. Features:
- **Email and password** authentication form
- **Link to register** for new users
- Auto-redirects to Dashboard if already authenticated
- Dark-themed centered card with OpenAgent branding

**Source**: `frontend/src/pages/LoginPage.tsx`

---

### Register Page (`/register`)

![Register Page](screenshots/register.png)

New user account creation with validation:
- **Display name** (2+ characters)
- **Email** address
- **Password** (8+ characters) with confirmation
- Real-time form validation (React Hook Form + Zod)
- Error display for failed registration
- On success, creates a new organization and redirects to Dashboard

**Source**: `frontend/src/pages/RegisterPage.tsx`

---

### Invite Accept Page (`/invite/:token`)

Accept organization invitations via email link. States:
- **Loading** — Validating invitation token
- **Success** — Joined organization, navigate to dashboard
- **Error** — Invalid or expired token
- **Login required** — Redirects to login if not authenticated

**Source**: `frontend/src/pages/InviteAcceptPage.tsx`

---

## Authenticated Pages (Require Login)

All authenticated pages are wrapped in the `AppShell` layout with a persistent sidebar navigation.

### Dashboard (`/dashboard`)

![Dashboard](screenshots/dashboard.png)

Fleet overview with real-time statistics and visualizations:

- **Stat Cards** (top row):
  - Total Agents — count of all registered agents
  - Online — currently active agents (green)
  - Offline — disconnected agents (gray)
  - Errors Today — error events in the last 24h (red)
- **Quick Stats**: Issues Processed Today, PRs Created Today
- **Charts**:
  - Pie chart — agent status distribution (online/offline/error)
  - Bar chart — events grouped by type
- **Live Event Feed** — real-time events via WebSocket connection
- Auto-refreshes stats every 30 seconds

**Source**: `frontend/src/pages/DashboardPage.tsx`

---

### Agents List (`/agents`)

![Agents List](screenshots/agents.png)

Browse and manage the agent fleet:

- **View toggle** — Switch between grid (cards) and table view
- **Search** — Filter agents by name, type, or GitHub repository
- **Status tabs** — All, Online, Offline, Error
- **Responsive grid** — 1 column (mobile) to 3 columns (desktop)
- Click any agent to navigate to its detail page

**Source**: `frontend/src/pages/AgentListPage.tsx`

---

### Agent Detail (`/agents/:agentId`)

Detailed view of a single agent:

- **Header** — Agent name, status badge, type
- **Metadata** — Version, hostname, GitHub repo, last heartbeat, tags
- **Configuration** — JSON snapshot of the agent's config at registration time
- **Event Timeline** — Last 50 events from this agent
- **Deregister** — Remove agent (with confirmation dialog)
- Back button to return to agent list

**Source**: `frontend/src/pages/AgentDetailPage.tsx`

---

### Event Feed (`/events`)

![Event Feed](screenshots/events.png)

Comprehensive real-time and historical event viewer:

- **Live Events** section at top with green pulsing indicator
- **Filters**:
  - By agent (dropdown)
  - By event type
  - By severity
  - By date range
- **Pagination** — 50 events per page with Previous/Next controls
- **Refresh** button to reload
- Real-time events stream via WebSocket alongside historical data

**Source**: `frontend/src/pages/EventFeedPage.tsx`

---

### API Keys (`/settings/api-keys`)

![API Keys](screenshots/api-keys.png)

Manage API keys used by agents to authenticate with the control plane:

- **Create Key** — Enter a name and click Create
- **Key Display Modal** — Shows the raw key exactly once after creation:

![API Key Created Modal](screenshots/api-key-created.png)

  - Green-highlighted key value with monospace font
  - Copy to clipboard button with visual feedback
  - Warning: key is only shown once, store it securely
  - Done button dismisses the modal

- **Active Keys List** — Each key shows:
  - Key prefix (masked)
  - Creation time
  - Last used time
  - Revoke button (removes key permanently)

**Source**: `frontend/src/pages/ApiKeysPage.tsx`

---

### Organization Settings (`/settings`)

![Organization Settings](screenshots/org-settings.png)

Manage organization profile, team, and invitations:

- **Organization Details**:
  - Edit organization name
  - View slug (read-only, auto-generated)
  - Save button with success feedback
- **Members List**:
  - Each member shows name and email
  - Role selector dropdown: Owner, Admin, Member, Viewer
  - Remove button with confirmation
- **Invite New Member**:
  - Email input with validation
  - Role selector (Admin, Member, Viewer)
  - Invite button sends email invitation
- **Pending Invitations**:
  - Shows email, role, status badge, expiration time
  - Cancel button to revoke invitation

**Source**: `frontend/src/pages/OrgSettingsPage.tsx`

---

### Audit Log (`/audit`)

![Audit Log](screenshots/audit-log.png)

Track all organization actions for compliance and debugging:

- **DataTable** with columns: Action, User, Resource, IP Address, Time
- **Expandable rows** — Click chevron to view full JSON details
- **Filters**:
  - By action name (text search)
  - By resource type (text search)
- **Pagination** — 25 entries per page
- **Sortable columns** — Action and Time

**Source**: `frontend/src/pages/AuditLogPage.tsx`

---

## Common UI Patterns

| Pattern | Description |
|---------|-------------|
| Dark theme | Zinc-900/800 backgrounds, zinc-100/400 text throughout |
| Status badges | Color-coded indicators (green=online, gray=offline, red=error) |
| Form validation | React Hook Form + Zod with inline error messages |
| Loading states | Spinners and skeleton placeholders |
| Confirmation | `window.confirm()` dialogs for destructive actions |
| Real-time | WebSocket integration for live events on Dashboard and Event Feed |
| Responsive | Mobile-first grids that expand on larger screens |

## Updating Screenshots

Screenshots are captured automatically by the Playwright test at `frontend/e2e/screenshots.spec.ts`. To regenerate:

```bash
cd frontend
npx playwright test e2e/screenshots.spec.ts
```

This requires the full stack to be running (`docker compose up`). Screenshots are saved to `docs/webui/screenshots/`.
