import ReconnectingWebSocket from 'reconnecting-websocket';
import type { AgentEvent, AgentLogEntry } from '@/types';

export function createEventStream(
  orgSlug: string,
  token: string,
): ReconnectingWebSocket {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = import.meta.env.VITE_WS_HOST || window.location.host;
  const url = `${protocol}//${host}/api/v1/orgs/${orgSlug}/ws?token=${encodeURIComponent(token)}`;

  const ws = new ReconnectingWebSocket(url, [], {
    maxRetries: 10,
    connectionTimeout: 5000,
    maxReconnectionDelay: 30000,
  });

  return ws;
}

export type WSMessage =
  | { type: 'agent.log'; log: AgentLogEntry }
  | { type: 'event'; event: AgentEvent };

export function parseWSMessage(data: string): WSMessage | null {
  try {
    const parsed = JSON.parse(data);
    if (parsed.type === 'agent.log' && parsed.log) {
      return { type: 'agent.log', log: parsed.log as AgentLogEntry };
    }
    // Legacy/regular event messages (no envelope)
    return { type: 'event', event: parsed as AgentEvent };
  } catch {
    console.error('Failed to parse WebSocket message:', data);
    return null;
  }
}

/** @deprecated Use parseWSMessage instead */
export function parseEventMessage(data: string): AgentEvent | null {
  const msg = parseWSMessage(data);
  if (msg?.type === 'event') return msg.event;
  return null;
}
