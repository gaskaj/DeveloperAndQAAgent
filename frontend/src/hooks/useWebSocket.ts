import { useEffect, useRef } from 'react';
import ReconnectingWebSocket from 'reconnecting-websocket';
import { createEventStream, parseWSMessage } from '@/api/ws';
import { useAuthStore } from '@/store/auth-store';
import { useEventStore } from '@/store/event-store';
import { useLogStore } from '@/store/log-store';

export function useWebSocket(orgSlug: string | undefined) {
  const wsRef = useRef<ReconnectingWebSocket | null>(null);
  const token = useAuthStore((s) => s.token);
  const addRealtimeEvent = useEventStore((s) => s.addRealtimeEvent);
  const addLogEntry = useLogStore((s) => s.addLogEntry);

  useEffect(() => {
    if (!orgSlug || !token) return;

    const ws = createEventStream(orgSlug, token);
    wsRef.current = ws;

    ws.onmessage = (evt) => {
      const msg = parseWSMessage(evt.data);
      if (!msg) return;

      if (msg.type === 'agent.log') {
        addLogEntry(msg.log);
      } else {
        addRealtimeEvent(msg.event);
      }
    };

    ws.onerror = (err) => {
      console.error('WebSocket error:', err);
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, [orgSlug, token, addRealtimeEvent, addLogEntry]);

  return wsRef;
}
