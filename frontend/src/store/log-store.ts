import { create } from 'zustand';
import type { AgentLogEntry } from '@/types';

const MAX_LOG_ENTRIES = 1000;

interface LogState {
  logs: AgentLogEntry[];
  paused: boolean;
  addLogEntry: (entry: AgentLogEntry) => void;
  clearLogs: () => void;
  setPaused: (paused: boolean) => void;
}

export const useLogStore = create<LogState>()((set) => ({
  logs: [],
  paused: false,

  addLogEntry: (entry) =>
    set((state) => {
      if (state.paused) return state;
      return {
        logs: [entry, ...state.logs].slice(0, MAX_LOG_ENTRIES),
      };
    }),

  clearLogs: () => set({ logs: [] }),

  setPaused: (paused) => set({ paused }),
}));
