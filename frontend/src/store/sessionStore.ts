import { create } from "zustand";
import type { SessionInfo, SessionStatus, HostInfo, DirectoryEntry, DiffFile } from "../lib/wails";
import { api } from "../lib/wails";

interface Tab {
  id: string;
  sessionId: string;
  ptyId: string;
  splits: string[];
}

interface EditorFile {
  path: string;
  contents: string;
  language: string;
  type: "file" | "diff";
}

interface ScopeMode {
  active: boolean;
  sessionId: string | null;
  snapshot: {
    tabs: Tab[];
    activeTabId: string | null;
    sidebarVisible: boolean;
  } | null;
}

const DEFAULT_FONT_SIZE = 13;
const FONT_SIZE_KEY = "cs-editor-font-size";
const TERMINAL_FONT_SIZE_KEY = "cs-terminal-font-size";

function loadPersistedFontSize(key: string): number {
  try {
    const stored = localStorage.getItem(key);
    if (stored) {
      const n = Number(stored);
      if (n >= 8 && n <= 40) return n;
    }
  } catch {}
  return DEFAULT_FONT_SIZE;
}

interface SessionState {
  sessions: SessionInfo[];
  statuses: Map<string, SessionStatus>;
  tabs: Tab[];
  activeTabId: string | null;
  selectedSidebarIdx: number;
  sidebarVisible: boolean;
  loadingSessionIds: Set<string>;
  flashSessionIds: Set<string>;
  hosts: HostInfo[];
  // Scope mode
  scopeMode: ScopeMode;
  explorerTree: Map<string, DirectoryEntry[]>;
  openEditorFiles: EditorFile[];
  activeEditorFile: string | null;
  pendingReveal: { line: number; column: number } | null;
  fileList: string[];
  quickOpenVisible: boolean;
  diffFiles: DiffFile[];
  diffLoading: boolean;
  editorFontSize: number;
  terminalFontSize: number;
  // Per-session scroll state: true = auto-scroll on new output, false = user scrolled up
  scrollLockedToBottom: Map<string, boolean>;

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
  // Scope mode actions
  enterScopeMode: (sessionId: string) => void;
  exitScopeMode: () => void;
  setExplorerEntries: (dirPath: string, entries: DirectoryEntry[]) => void;
  clearExplorerTree: () => void;
  openEditorFile: (path: string, contents: string, language: string) => void;
  closeEditorFile: (path: string) => void;
  setActiveEditorFile: (path: string) => void;
  setPendingReveal: (reveal: { line: number; column: number } | null) => void;
  updateEditorFileContents: (path: string, contents: string) => void;
  setFileList: (files: string[]) => void;
  setQuickOpenVisible: (visible: boolean) => void;
  toggleQuickOpen: () => void;
  fetchDiffFiles: (sessionId: string) => Promise<void>;
  clearDiffFiles: () => void;
  openDiffTab: () => void;
  setEditorFontSize: (size: number) => void;
  zoomEditorFont: (delta: number) => void;
  setTerminalFontSize: (size: number) => void;
  zoomTerminalFont: (delta: number) => void;
  setScrollLocked: (ptyId: string, locked: boolean) => void;
  getScrollLocked: (ptyId: string) => boolean;
}

const extToLanguage: Record<string, string> = {
  ".go": "go", ".ts": "typescript", ".tsx": "typescript", ".js": "javascript",
  ".jsx": "javascript", ".py": "python", ".json": "json", ".md": "markdown",
  ".css": "css", ".html": "html", ".yaml": "yaml", ".yml": "yaml",
  ".toml": "toml", ".rs": "rust", ".sh": "shell", ".bash": "shell",
  ".sql": "sql", ".graphql": "graphql", ".proto": "protobuf",
  ".dockerfile": "dockerfile", ".xml": "xml", ".svg": "xml",
  ".c": "c", ".h": "c", ".cpp": "cpp", ".hpp": "cpp",
  ".java": "java", ".rb": "ruby", ".php": "php", ".swift": "swift",
  ".kt": "kotlin", ".lua": "lua", ".r": "r", ".R": "r",
};

export function detectLanguage(filePath: string): string {
  const name = filePath.toLowerCase();
  if (name === "dockerfile" || name.endsWith("/dockerfile")) return "dockerfile";
  if (name === "makefile" || name.endsWith("/makefile")) return "makefile";
  const ext = name.includes(".") ? "." + name.split(".").pop() : "";
  return extToLanguage[ext] ?? "plaintext";
}

let tabCounter = 0;

// Track pending flash-clear timers to avoid duplicate/unbounded setTimeout accumulation
const _flashTimers = new Map<string, ReturnType<typeof setTimeout>>();

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
  scopeMode: { active: false, sessionId: null, snapshot: null },
  explorerTree: new Map(),
  openEditorFiles: [],
  activeEditorFile: null,
  pendingReveal: null,
  fileList: [],
  quickOpenVisible: false,
  diffFiles: [],
  diffLoading: false,
  editorFontSize: loadPersistedFontSize(FONT_SIZE_KEY),
  terminalFontSize: loadPersistedFontSize(TERMINAL_FONT_SIZE_KEY),
  scrollLockedToBottom: new Map(),

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
          // Schedule flash clear, but only one timer per id
          if (!_flashTimers.has(id)) {
            const timer = setTimeout(() => {
              _flashTimers.delete(id);
              get().clearFlash(id);
            }, 1500);
            _flashTimers.set(id, timer);
          }
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
      // If the ptyId changed (e.g. session was paused and resumed with a fresh
      // PTY), update the tab so TerminalPane remounts against the new ptyId.
      // Without this, the tab keeps trying to connect to a dead PTY.
      if (existing.ptyId !== ptyId) {
        const nextScroll = new Map(get().scrollLockedToBottom);
        nextScroll.delete(existing.ptyId);
        nextScroll.set(ptyId, true);
        set((state) => ({
          tabs: state.tabs.map((t) =>
            t.id === existing.id ? { ...t, ptyId } : t
          ),
          activeTabId: existing.id,
          scrollLockedToBottom: nextScroll,
        }));
      } else {
        set({ activeTabId: existing.id });
      }
      return;
    }
    const tabId = `tab-${++tabCounter}`;
    const tab: Tab = { id: tabId, sessionId, ptyId, splits: [] };
    // New/reopened sessions start locked to bottom
    const nextScroll = new Map(get().scrollLockedToBottom);
    nextScroll.set(ptyId, true);
    set((state) => ({
      tabs: [...state.tabs, tab],
      activeTabId: tabId,
      scrollLockedToBottom: nextScroll,
    }));
  },

  closeTab: (tabId) =>
    set((state) => {
      const closedTab = state.tabs.find((t) => t.id === tabId);
      const tabs = state.tabs.filter((t) => t.id !== tabId);
      const activeTabId =
        state.activeTabId === tabId
          ? tabs.length > 0
            ? tabs[tabs.length - 1].id
            : null
          : state.activeTabId;
      // Clean up scroll state for the closed tab's ptyId
      if (closedTab) {
        const nextScroll = new Map(state.scrollLockedToBottom);
        nextScroll.delete(closedTab.ptyId);
        return { tabs, activeTabId, scrollLockedToBottom: nextScroll };
      }
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

  // Scope mode actions
  enterScopeMode: (sessionId) =>
    set((state) => ({
      scopeMode: {
        active: true,
        sessionId,
        snapshot: {
          tabs: [...state.tabs],
          activeTabId: state.activeTabId,
          sidebarVisible: state.sidebarVisible,
        },
      },
    })),

  exitScopeMode: () =>
    set((state) => {
      const snapshot = state.scopeMode.snapshot;
      return {
        scopeMode: { active: false, sessionId: null, snapshot: null },
        explorerTree: new Map(),
        openEditorFiles: [],
        activeEditorFile: null,
        pendingReveal: null,
        fileList: [],
        quickOpenVisible: false,
        diffFiles: [],
        diffLoading: false,
        ...(snapshot
          ? {
              activeTabId: snapshot.activeTabId,
              sidebarVisible: snapshot.sidebarVisible,
            }
          : {}),
      };
    }),

  setExplorerEntries: (dirPath, entries) =>
    set((state) => {
      const next = new Map(state.explorerTree);
      next.set(dirPath, entries);
      return { explorerTree: next };
    }),

  clearExplorerTree: () => set({ explorerTree: new Map() }),

  openEditorFile: (path, contents, language) =>
    set((state) => {
      const existing = state.openEditorFiles.find((f) => f.path === path);
      if (existing) return { activeEditorFile: path };
      return {
        openEditorFiles: [...state.openEditorFiles, { path, contents, language, type: "file" }],
        activeEditorFile: path,
      };
    }),

  closeEditorFile: (path) =>
    set((state) => {
      const files = state.openEditorFiles.filter((f) => f.path !== path);
      const activeEditorFile =
        state.activeEditorFile === path
          ? files.length > 0
            ? files[files.length - 1].path
            : null
          : state.activeEditorFile;
      return { openEditorFiles: files, activeEditorFile };
    }),

  setActiveEditorFile: (path) => set({ activeEditorFile: path }),

  setPendingReveal: (reveal) => set({ pendingReveal: reveal }),

  updateEditorFileContents: (path, contents) =>
    set((state) => ({
      openEditorFiles: state.openEditorFiles.map((f) =>
        f.path === path ? { ...f, contents } : f
      ),
    })),

  setFileList: (files) => set({ fileList: files }),
  setQuickOpenVisible: (visible) => set({ quickOpenVisible: visible }),
  toggleQuickOpen: () => set((state) => ({ quickOpenVisible: !state.quickOpenVisible })),

  fetchDiffFiles: async (sessionId) => {
    set({ diffLoading: true });
    try {
      const files = await api().GetDiffFiles(sessionId);
      set({ diffFiles: files ?? [], diffLoading: false });
    } catch (err) {
      console.error("Failed to fetch diff files:", err);
      set({ diffLoading: false });
    }
  },

  clearDiffFiles: () => set({ diffFiles: [], diffLoading: false }),

  openDiffTab: () =>
    set((state) => {
      const existing = state.openEditorFiles.find((f) => f.type === "diff");
      if (existing) return { activeEditorFile: existing.path };
      return {
        openEditorFiles: [
          ...state.openEditorFiles,
          { path: "__diff__", contents: "", language: "plaintext", type: "diff" },
        ],
        activeEditorFile: "__diff__",
      };
    }),

  setEditorFontSize: (size) => {
    const clamped = Math.max(8, Math.min(40, size));
    localStorage.setItem(FONT_SIZE_KEY, String(clamped));
    set({ editorFontSize: clamped });
  },

  zoomEditorFont: (delta) => {
    const current = get().editorFontSize;
    const next = Math.max(8, Math.min(40, current + delta));
    localStorage.setItem(FONT_SIZE_KEY, String(next));
    set({ editorFontSize: next });
  },

  setTerminalFontSize: (size) => {
    const clamped = Math.max(8, Math.min(40, size));
    localStorage.setItem(TERMINAL_FONT_SIZE_KEY, String(clamped));
    set({ terminalFontSize: clamped });
  },

  zoomTerminalFont: (delta) => {
    const current = get().terminalFontSize;
    const next = Math.max(8, Math.min(40, current + delta));
    localStorage.setItem(TERMINAL_FONT_SIZE_KEY, String(next));
    set({ terminalFontSize: next });
  },

  setScrollLocked: (ptyId, locked) => {
    const next = new Map(get().scrollLockedToBottom);
    next.set(ptyId, locked);
    set({ scrollLockedToBottom: next });
  },

  getScrollLocked: (ptyId) => {
    return get().scrollLockedToBottom.get(ptyId) ?? true;
  },
}));
