import { useState, useEffect, useRef, useMemo } from 'react';
import {
  Pause,
  Play,
  Trash2,
  ArrowDown,
  ChevronDown,
  ChevronRight,
} from 'lucide-react';
import { useAuthStore } from '@/store/auth-store';
import { useLogStore } from '@/store/log-store';
import { useWebSocket } from '@/hooks/useWebSocket';
import type { AgentLogEntry } from '@/types';

const LEVEL_STYLES: Record<string, { bg: string; text: string; badge: string }> = {
  DEBUG: { bg: 'bg-zinc-900', text: 'text-zinc-500', badge: 'bg-zinc-700 text-zinc-400' },
  INFO: { bg: 'bg-zinc-900', text: 'text-zinc-300', badge: 'bg-blue-900/50 text-blue-400' },
  WARN: { bg: 'bg-yellow-950/20', text: 'text-yellow-200', badge: 'bg-yellow-900/50 text-yellow-400' },
  ERROR: { bg: 'bg-red-950/20', text: 'text-red-200', badge: 'bg-red-900/50 text-red-400' },
};

function getLevelStyle(level: string) {
  return LEVEL_STYLES[level.toUpperCase()] ?? LEVEL_STYLES.INFO;
}

function formatTimestamp(ts: string) {
  try {
    const d = new Date(ts);
    const base = d.toLocaleTimeString('en-US', { hour12: false });
    const ms = String(d.getMilliseconds()).padStart(3, '0');
    return `${base}.${ms}`;
  } catch {
    return ts;
  }
}

function LogRow({ entry, isExpanded, onToggle }: {
  entry: AgentLogEntry;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const style = getLevelStyle(entry.level);
  const hasFields = entry.fields && Object.keys(entry.fields).length > 0;

  return (
    <div className={`border-b border-zinc-800 ${style.bg}`}>
      <div
        className="flex items-start gap-2 px-4 py-1.5 font-mono text-xs cursor-pointer hover:bg-white/5"
        onClick={onToggle}
      >
        {/* Expand toggle */}
        <span className="mt-0.5 shrink-0 text-zinc-600 w-4">
          {hasFields ? (
            isExpanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />
          ) : null}
        </span>

        {/* Timestamp */}
        <span className="shrink-0 text-zinc-600 w-[90px]">
          {formatTimestamp(entry.timestamp)}
        </span>

        {/* Level badge */}
        <span className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-bold uppercase leading-none w-[52px] text-center ${style.badge}`}>
          {entry.level}
        </span>

        {/* Agent name */}
        <span className="shrink-0 text-purple-400 w-[120px] truncate" title={entry.agent_name}>
          {entry.agent_name}
        </span>

        {/* Message */}
        <span className={`flex-1 break-all ${style.text}`}>
          {entry.message}
        </span>
      </div>

      {/* Expanded fields */}
      {isExpanded && hasFields && (
        <div className="ml-[90px] border-l border-zinc-700 px-4 py-2 mb-1">
          <pre className="text-[11px] text-zinc-500 whitespace-pre-wrap">
            {JSON.stringify(entry.fields, null, 2)}
          </pre>
        </div>
      )}
    </div>
  );
}

export function AuditLogPage() {
  const currentOrg = useAuthStore((s) => s.currentOrg);
  const logs = useLogStore((s) => s.logs);
  const paused = useLogStore((s) => s.paused);
  const clearLogs = useLogStore((s) => s.clearLogs);
  const setPaused = useLogStore((s) => s.setPaused);

  const [expandedIdx, setExpandedIdx] = useState<number | null>(null);
  const [levelFilter, setLevelFilter] = useState<string>('ALL');
  const [agentFilter, setAgentFilter] = useState<string>('ALL');
  const [searchFilter, setSearchFilter] = useState<string>('');
  const [autoScroll, setAutoScroll] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);

  // Connect WebSocket
  useWebSocket(currentOrg?.slug);

  // Auto-scroll to top (newest entries are prepended)
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = 0;
    }
  }, [logs.length, autoScroll]);

  // Collect unique agent names for filter dropdown
  const agentNames = useMemo(() => {
    const names = new Set<string>();
    logs.forEach((l) => names.add(l.agent_name));
    return Array.from(names).sort();
  }, [logs]);

  // Filter logs
  const filteredLogs = useMemo(() => {
    return logs.filter((entry) => {
      if (levelFilter !== 'ALL' && entry.level.toUpperCase() !== levelFilter) return false;
      if (agentFilter !== 'ALL' && entry.agent_name !== agentFilter) return false;
      if (searchFilter && !entry.message.toLowerCase().includes(searchFilter.toLowerCase())) return false;
      return true;
    });
  }, [logs, levelFilter, agentFilter, searchFilter]);

  const logCount = filteredLogs.length;
  const totalCount = logs.length;

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="shrink-0 border-b border-zinc-700 px-6 py-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold text-zinc-100">Agent Logs</h1>
            <p className="mt-1 text-sm text-zinc-400">
              Real-time log stream from all connected agents
            </p>
          </div>
          <div className="flex items-center gap-2">
            {/* Live indicator */}
            <div className="flex items-center gap-2 rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-1.5">
              {paused ? (
                <span className="h-2 w-2 rounded-full bg-yellow-500" />
              ) : (
                <span className="relative flex h-2 w-2">
                  <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75" />
                  <span className="relative inline-flex h-2 w-2 rounded-full bg-green-500" />
                </span>
              )}
              <span className="text-xs text-zinc-400">
                {paused ? 'Paused' : 'Live'}
              </span>
            </div>
            <span className="text-xs text-zinc-600">
              {logCount === totalCount
                ? `${totalCount} entries`
                : `${logCount} / ${totalCount} entries`}
            </span>
          </div>
        </div>

        {/* Toolbar */}
        <div className="mt-3 flex flex-wrap items-center gap-3">
          {/* Level filter */}
          <select
            value={levelFilter}
            onChange={(e) => setLevelFilter(e.target.value)}
            className="rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-1.5 text-sm text-zinc-300 outline-none focus:border-blue-500"
          >
            <option value="ALL">All Levels</option>
            <option value="DEBUG">DEBUG</option>
            <option value="INFO">INFO</option>
            <option value="WARN">WARN</option>
            <option value="ERROR">ERROR</option>
          </select>

          {/* Agent filter */}
          <select
            value={agentFilter}
            onChange={(e) => setAgentFilter(e.target.value)}
            className="rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-1.5 text-sm text-zinc-300 outline-none focus:border-blue-500"
          >
            <option value="ALL">All Agents</option>
            {agentNames.map((name) => (
              <option key={name} value={name}>{name}</option>
            ))}
          </select>

          {/* Search */}
          <input
            type="text"
            placeholder="Search messages..."
            value={searchFilter}
            onChange={(e) => setSearchFilter(e.target.value)}
            className="rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-1.5 text-sm text-zinc-300 placeholder-zinc-600 outline-none focus:border-blue-500 min-w-[200px]"
          />

          <div className="flex-1" />

          {/* Action buttons */}
          <button
            onClick={() => setAutoScroll(!autoScroll)}
            className={`rounded-lg border p-1.5 ${
              autoScroll
                ? 'border-blue-600 bg-blue-600/20 text-blue-400'
                : 'border-zinc-700 bg-zinc-800 text-zinc-500 hover:text-zinc-300'
            }`}
            title={autoScroll ? 'Auto-scroll on' : 'Auto-scroll off'}
          >
            <ArrowDown className="h-4 w-4" />
          </button>

          <button
            onClick={() => setPaused(!paused)}
            className={`rounded-lg border p-1.5 ${
              paused
                ? 'border-yellow-600 bg-yellow-600/20 text-yellow-400'
                : 'border-zinc-700 bg-zinc-800 text-zinc-500 hover:text-zinc-300'
            }`}
            title={paused ? 'Resume' : 'Pause'}
          >
            {paused ? <Play className="h-4 w-4" /> : <Pause className="h-4 w-4" />}
          </button>

          <button
            onClick={clearLogs}
            className="rounded-lg border border-zinc-700 bg-zinc-800 p-1.5 text-zinc-500 hover:text-red-400"
            title="Clear logs"
          >
            <Trash2 className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* Log stream */}
      <div
        ref={scrollRef}
        className="flex-1 overflow-y-auto bg-zinc-950"
      >
        {filteredLogs.length === 0 ? (
          <div className="flex h-full items-center justify-center text-zinc-600">
            <div className="text-center">
              <p className="text-lg">No log entries yet</p>
              <p className="mt-1 text-sm">
                Logs will appear here in real time when agents are running
              </p>
            </div>
          </div>
        ) : (
          filteredLogs.map((entry, idx) => (
            <LogRow
              key={`${entry.timestamp}-${idx}`}
              entry={entry}
              isExpanded={expandedIdx === idx}
              onToggle={() => setExpandedIdx(expandedIdx === idx ? null : idx)}
            />
          ))
        )}
      </div>
    </div>
  );
}
