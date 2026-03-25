import { create } from "zustand";
import type { SessionInfo, SessionStatus } from "../lib/wails";

interface Tab {
  id: string;
  sessionId: string;
  splits: string[];
}

interface SessionState {
  sessions: SessionInfo[];
  statuses: Map<string, SessionStatus>;
  tabs: Tab[];
  activeTabId: string | null;
  selectedSidebarIdx: number;
  sidebarVisible: boolean;

  setSessions: (sessions: SessionInfo[]) => void;
  updateStatuses: (statuses: SessionStatus[]) => void;
  addSession: (session: SessionInfo) => void;
  removeSession: (id: string) => void;
  openTab: (sessionId: string) => void;
  closeTab: (tabId: string) => void;
  setActiveTab: (tabId: string) => void;
  setSelectedSidebarIdx: (idx: number) => void;
  toggleSidebar: () => void;
}

let tabCounter = 0;

export const useSessionStore = create<SessionState>((set, get) => ({
  sessions: [],
  statuses: new Map(),
  tabs: [],
  activeTabId: null,
  selectedSidebarIdx: 0,
  sidebarVisible: true,

  setSessions: (sessions) => set({ sessions }),

  updateStatuses: (statuses) => {
    const map = new Map<string, SessionStatus>();
    statuses.forEach((s) => map.set(s.id, s));
    set({ statuses: map });
  },

  addSession: (session) =>
    set((state) => ({ sessions: [...state.sessions, session] })),

  removeSession: (id) =>
    set((state) => ({
      sessions: state.sessions.filter((s) => s.id !== id),
      tabs: state.tabs.filter((t) => t.sessionId !== id),
    })),

  openTab: (sessionId) => {
    const { tabs } = get();
    const existing = tabs.find((t) => t.sessionId === sessionId);
    if (existing) {
      set({ activeTabId: existing.id });
      return;
    }
    const tabId = `tab-${++tabCounter}`;
    const tab: Tab = { id: tabId, sessionId, splits: [] };
    set((state) => ({
      tabs: [...state.tabs, tab],
      activeTabId: tabId,
    }));
  },

  closeTab: (tabId) =>
    set((state) => {
      const tabs = state.tabs.filter((t) => t.id !== tabId);
      const activeTabId =
        state.activeTabId === tabId
          ? tabs.length > 0
            ? tabs[tabs.length - 1].id
            : null
          : state.activeTabId;
      return { tabs, activeTabId };
    }),

  setActiveTab: (tabId) => set({ activeTabId: tabId }),
  setSelectedSidebarIdx: (idx) => set({ selectedSidebarIdx: idx }),
  toggleSidebar: () =>
    set((state) => ({ sidebarVisible: !state.sidebarVisible })),
}));
