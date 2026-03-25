import { create } from "zustand";
import type { SessionInfo, SessionStatus, HostInfo } from "../lib/wails";

interface Tab {
  id: string;
  sessionId: string;
  ptyId: string;
  splits: string[];
}

interface SessionState {
  sessions: SessionInfo[];
  statuses: Map<string, SessionStatus>;
  tabs: Tab[];
  activeTabId: string | null;
  selectedSidebarIdx: number;
  sidebarVisible: boolean;
  // Sessions currently being started (show loading indicator)
  loadingSessionIds: Set<string>;
  // Sessions that just finished loading (flash briefly)
  flashSessionIds: Set<string>;
  // SSH hosts
  hosts: HostInfo[];

  setSessions: (sessions: SessionInfo[]) => void;
  updateStatuses: (statuses: SessionStatus[]) => void;
  addSession: (session: SessionInfo) => void;
  removeSession: (id: string) => void;
  markLoading: (id: string) => void;
  clearFlash: (id: string) => void;
  openTab: (sessionId: string, ptyId: string) => void;
  closeTab: (tabId: string) => void;
  setActiveTab: (tabId: string) => void;
  setSelectedSidebarIdx: (idx: number) => void;
  toggleSidebar: () => void;
  setHosts: (hosts: HostInfo[]) => void;
  addHost: (host: HostInfo) => void;
  removeHost: (id: string) => void;
}

let tabCounter = 0;

export const useSessionStore = create<SessionState>((set, get) => ({
  sessions: [],
  statuses: new Map(),
  tabs: [],
  activeTabId: null,
  selectedSidebarIdx: 0,
  sidebarVisible: true,
  loadingSessionIds: new Set(),
  flashSessionIds: new Set(),
  hosts: [],

  setSessions: (sessions) => set({ sessions }),

  updateStatuses: (statuses) => {
    const map = new Map<string, SessionStatus>();
    statuses.forEach((s) => map.set(s.id, s));

    // Check if any loading sessions have transitioned to running/ready
    const { loadingSessionIds } = get();
    if (loadingSessionIds.size > 0) {
      const newLoading = new Set(loadingSessionIds);
      const newFlash = new Set(get().flashSessionIds);
      for (const id of loadingSessionIds) {
        const status = map.get(id);
        if (status && status.status !== "loading") {
          newLoading.delete(id);
          newFlash.add(id);
          // Auto-clear flash after 1.5s
          setTimeout(() => {
            get().clearFlash(id);
          }, 1500);
        }
      }
      if (newLoading.size !== loadingSessionIds.size) {
        set({ statuses: map, loadingSessionIds: newLoading, flashSessionIds: newFlash });
        return;
      }
    }

    set({ statuses: map });
  },

  addSession: (session) =>
    set((state) => ({ sessions: [...state.sessions, session] })),

  removeSession: (id) =>
    set((state) => ({
      sessions: state.sessions.filter((s) => s.id !== id),
      tabs: state.tabs.filter((t) => t.sessionId !== id),
    })),

  markLoading: (id) =>
    set((state) => {
      const next = new Set(state.loadingSessionIds);
      next.add(id);
      return { loadingSessionIds: next };
    }),

  clearFlash: (id) =>
    set((state) => {
      const next = new Set(state.flashSessionIds);
      next.delete(id);
      return { flashSessionIds: next };
    }),

  openTab: (sessionId, ptyId) => {
    const { tabs } = get();
    const existing = tabs.find((t) => t.sessionId === sessionId);
    if (existing) {
      set({ activeTabId: existing.id });
      return;
    }
    const tabId = `tab-${++tabCounter}`;
    const tab: Tab = { id: tabId, sessionId, ptyId, splits: [] };
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

  setActiveTab: (tabId) => {
    const { tabs, sessions } = get();
    const tab = tabs.find((t) => t.id === tabId);
    if (tab) {
      const idx = sessions.findIndex((s) => s.id === tab.sessionId);
      if (idx >= 0) {
        set({ activeTabId: tabId, selectedSidebarIdx: idx });
        return;
      }
    }
    set({ activeTabId: tabId });
  },
  setSelectedSidebarIdx: (idx) => set({ selectedSidebarIdx: idx }),
  toggleSidebar: () =>
    set((state) => ({ sidebarVisible: !state.sidebarVisible })),
  setHosts: (hosts) => set({ hosts }),
  addHost: (host) => set((s) => ({ hosts: [...s.hosts, host] })),
  removeHost: (id) => set((s) => ({ hosts: s.hosts.filter((h) => h.id !== id) })),
}));
