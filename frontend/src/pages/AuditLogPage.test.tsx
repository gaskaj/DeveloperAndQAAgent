import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AuditLogPage } from './AuditLogPage';
import { useAuthStore } from '@/store/auth-store';
import { useLogStore } from '@/store/log-store';
import { TEST_USER, TEST_ORG } from '@/test/mocks';

vi.mock('@/hooks/useWebSocket', () => ({
  useWebSocket: vi.fn(),
}));

describe('AuditLogPage', () => {
  beforeEach(() => {
    useAuthStore.setState({
      user: TEST_USER, token: 'test-token', refreshToken: 'test-refresh', currentOrg: TEST_ORG,
    });
    useLogStore.setState({ logs: [], paused: false });
  });

  it('renders without crashing', () => {
    expect(() => render(<AuditLogPage />)).not.toThrow();
  });

  it('renders heading', () => {
    render(<AuditLogPage />);
    expect(screen.getByText('Agent Logs')).toBeInTheDocument();
  });

  it('shows empty state', () => {
    render(<AuditLogPage />);
    expect(screen.getByText('No log entries yet')).toBeInTheDocument();
  });

  it('renders level filter', () => {
    render(<AuditLogPage />);
    expect(screen.getByText('All Levels')).toBeInTheDocument();
  });

  it('renders log entries when present', () => {
    useLogStore.setState({
      logs: [
        {
          agent_name: 'test-agent',
          level: 'INFO',
          message: 'Starting workflow',
          timestamp: new Date().toISOString(),
        },
      ],
    });
    render(<AuditLogPage />);
    expect(screen.getByText('Starting workflow')).toBeInTheDocument();
    expect(screen.getAllByText('test-agent').length).toBeGreaterThan(0);
  });

  it('renders without currentOrg', () => {
    useAuthStore.setState({ currentOrg: null });
    expect(() => render(<AuditLogPage />)).not.toThrow();
  });
});
