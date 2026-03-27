import { describe, it, expect, beforeEach } from "vitest";
import { useSessionStore, detectLanguage } from "./sessionStore";

// Reset store between tests
beforeEach(() => {
  useSessionStore.setState({
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
  });
});

describe("detectLanguage", () => {
  it("detects Go files", () => {
    expect(detectLanguage("main.go")).toBe("go");
    expect(detectLanguage("app/bindings.go")).toBe("go");
  });

  it("detects TypeScript files", () => {
    expect(detectLanguage("App.tsx")).toBe("typescript");
    expect(detectLanguage("store.ts")).toBe("typescript");
  });

  it("detects JavaScript files", () => {
    expect(detectLanguage("index.js")).toBe("javascript");
    expect(detectLanguage("App.jsx")).toBe("javascript");
  });

  it("detects C/C++ files", () => {
    expect(detectLanguage("main.c")).toBe("c");
    expect(detectLanguage("header.h")).toBe("c");
    expect(detectLanguage("main.cpp")).toBe("cpp");
    expect(detectLanguage("header.hpp")).toBe("cpp");
  });

  it("detects special filenames", () => {
    expect(detectLanguage("Dockerfile")).toBe("dockerfile");
    expect(detectLanguage("src/Dockerfile")).toBe("dockerfile");
    expect(detectLanguage("Makefile")).toBe("makefile");
  });

  it("defaults to plaintext for unknown extensions", () => {
    expect(detectLanguage("file.xyz")).toBe("plaintext");
    expect(detectLanguage("noextension")).toBe("plaintext");
  });
});

describe("session management", () => {
  it("adds and removes sessions", () => {
    const store = useSessionStore.getState();
    const session = {
      id: "s1",
      title: "test",
      path: "/tmp",
      branch: "main",
      program: "claude",
      status: "running" as const,
    };

    store.addSession(session);
    expect(useSessionStore.getState().sessions).toHaveLength(1);
    expect(useSessionStore.getState().sessions[0].id).toBe("s1");

    store.removeSession("s1");
    expect(useSessionStore.getState().sessions).toHaveLength(0);
  });

  it("opens and closes tabs", () => {
    const store = useSessionStore.getState();
    store.openTab("s1", "pty-1");

    let state = useSessionStore.getState();
    expect(state.tabs).toHaveLength(1);
    expect(state.tabs[0].sessionId).toBe("s1");
    expect(state.activeTabId).toBe(state.tabs[0].id);

    // Opening same session again just activates existing tab
    store.openTab("s1", "pty-1");
    state = useSessionStore.getState();
    expect(state.tabs).toHaveLength(1);

    // Close tab
    store.closeTab(state.tabs[0].id);
    state = useSessionStore.getState();
    expect(state.tabs).toHaveLength(0);
    expect(state.activeTabId).toBeNull();
  });

  it("removes tabs when session is removed", () => {
    const store = useSessionStore.getState();
    store.addSession({
      id: "s1",
      title: "test",
      path: "/tmp",
      branch: "main",
      program: "claude",
      status: "running",
    });
    store.openTab("s1", "pty-1");
    expect(useSessionStore.getState().tabs).toHaveLength(1);

    store.removeSession("s1");
    expect(useSessionStore.getState().tabs).toHaveLength(0);
  });
});

describe("scope mode", () => {
  it("enters and exits scope mode", () => {
    const store = useSessionStore.getState();
    store.openTab("s1", "pty-1");
    store.enterScopeMode("s1");

    let state = useSessionStore.getState();
    expect(state.scopeMode.active).toBe(true);
    expect(state.scopeMode.sessionId).toBe("s1");
    expect(state.scopeMode.snapshot).toBeTruthy();

    store.exitScopeMode();
    state = useSessionStore.getState();
    expect(state.scopeMode.active).toBe(false);
    expect(state.scopeMode.sessionId).toBeNull();
  });

  it("captures and restores snapshot on scope exit", () => {
    const store = useSessionStore.getState();
    store.openTab("s1", "pty-1");
    const tabId = useSessionStore.getState().activeTabId;

    store.enterScopeMode("s1");
    // Modify state while in scope mode
    useSessionStore.setState({ activeTabId: null });

    store.exitScopeMode();
    // Should restore the activeTabId from snapshot
    expect(useSessionStore.getState().activeTabId).toBe(tabId);
  });

  it("clears editor and file state on scope exit", () => {
    const store = useSessionStore.getState();
    store.enterScopeMode("s1");
    store.openEditorFile("test.go", "package main", "go");
    store.setFileList(["test.go", "main.go"]);

    expect(useSessionStore.getState().openEditorFiles).toHaveLength(1);
    expect(useSessionStore.getState().fileList).toHaveLength(2);

    store.exitScopeMode();
    expect(useSessionStore.getState().openEditorFiles).toHaveLength(0);
    expect(useSessionStore.getState().fileList).toHaveLength(0);
    expect(useSessionStore.getState().quickOpenVisible).toBe(false);
  });
});

describe("editor files", () => {
  it("opens editor files and sets active", () => {
    const store = useSessionStore.getState();
    store.openEditorFile("main.go", "package main", "go");
    store.openEditorFile("app.ts", "export {}", "typescript");

    const state = useSessionStore.getState();
    expect(state.openEditorFiles).toHaveLength(2);
    expect(state.activeEditorFile).toBe("app.ts");
  });

  it("does not duplicate open files", () => {
    const store = useSessionStore.getState();
    store.openEditorFile("main.go", "package main", "go");
    store.openEditorFile("main.go", "package main", "go");

    expect(useSessionStore.getState().openEditorFiles).toHaveLength(1);
    expect(useSessionStore.getState().activeEditorFile).toBe("main.go");
  });

  it("closes editor files and updates active", () => {
    const store = useSessionStore.getState();
    store.openEditorFile("a.go", "a", "go");
    store.openEditorFile("b.go", "b", "go");
    store.openEditorFile("c.go", "c", "go");

    // Close active file (c.go) — should activate b.go (last remaining)
    store.closeEditorFile("c.go");
    let state = useSessionStore.getState();
    expect(state.openEditorFiles).toHaveLength(2);
    expect(state.activeEditorFile).toBe("b.go");

    // Close non-active file — active shouldn't change
    store.closeEditorFile("a.go");
    state = useSessionStore.getState();
    expect(state.openEditorFiles).toHaveLength(1);
    expect(state.activeEditorFile).toBe("b.go");
  });

  it("updates file contents", () => {
    const store = useSessionStore.getState();
    store.openEditorFile("main.go", "old content", "go");
    store.updateEditorFileContents("main.go", "new content");

    const file = useSessionStore.getState().openEditorFiles[0];
    expect(file.contents).toBe("new content");
  });
});

describe("quick open state", () => {
  it("toggles quick open visibility", () => {
    const store = useSessionStore.getState();
    expect(useSessionStore.getState().quickOpenVisible).toBe(false);

    store.toggleQuickOpen();
    expect(useSessionStore.getState().quickOpenVisible).toBe(true);

    store.toggleQuickOpen();
    expect(useSessionStore.getState().quickOpenVisible).toBe(false);
  });

  it("sets quick open visibility directly", () => {
    const store = useSessionStore.getState();
    store.setQuickOpenVisible(true);
    expect(useSessionStore.getState().quickOpenVisible).toBe(true);

    store.setQuickOpenVisible(false);
    expect(useSessionStore.getState().quickOpenVisible).toBe(false);
  });

  it("sets file list", () => {
    const store = useSessionStore.getState();
    store.setFileList(["a.go", "b.ts", "c.tsx"]);
    expect(useSessionStore.getState().fileList).toEqual(["a.go", "b.ts", "c.tsx"]);
  });
});

describe("file explorer tree", () => {
  it("sets and clears explorer entries", () => {
    const store = useSessionStore.getState();
    const entries = [
      { name: "src", path: "src", isDir: true, size: 0 },
      { name: "main.go", path: "main.go", isDir: false, size: 100 },
    ];

    store.setExplorerEntries(".", entries);
    expect(useSessionStore.getState().explorerTree.get(".")).toEqual(entries);

    store.clearExplorerTree();
    expect(useSessionStore.getState().explorerTree.size).toBe(0);
  });
});

describe("sidebar", () => {
  it("toggles sidebar visibility", () => {
    const store = useSessionStore.getState();
    expect(useSessionStore.getState().sidebarVisible).toBe(true);

    store.toggleSidebar();
    expect(useSessionStore.getState().sidebarVisible).toBe(false);

    store.toggleSidebar();
    expect(useSessionStore.getState().sidebarVisible).toBe(true);
  });

  it("sets selected sidebar index", () => {
    const store = useSessionStore.getState();
    store.setSelectedSidebarIdx(3);
    expect(useSessionStore.getState().selectedSidebarIdx).toBe(3);
  });
});

describe("hosts", () => {
  it("adds and removes hosts", () => {
    const store = useSessionStore.getState();
    const host = {
      id: "h1",
      name: "test-host",
      host: "192.168.1.1",
      port: 22,
      user: "root",
      authMethod: "key",
      keyPath: "~/.ssh/id_rsa",
      lastPath: "/root",
    };

    store.addHost(host);
    expect(useSessionStore.getState().hosts).toHaveLength(1);

    store.removeHost("h1");
    expect(useSessionStore.getState().hosts).toHaveLength(0);
  });
});

describe("diff tab management", () => {
  it("openDiffTab creates a diff tab with __diff__ path", () => {
    const { openDiffTab } = useSessionStore.getState();
    openDiffTab();
    const state = useSessionStore.getState();
    expect(state.openEditorFiles).toHaveLength(1);
    expect(state.openEditorFiles[0].type).toBe("diff");
    expect(state.openEditorFiles[0].path).toBe("__diff__");
    expect(state.activeEditorFile).toBe("__diff__");
  });

  it("openDiffTab switches to existing diff tab", () => {
    const store = useSessionStore.getState();
    store.openDiffTab();
    store.openEditorFile("test.go", "content", "go");
    expect(useSessionStore.getState().activeEditorFile).toBe("test.go");
    useSessionStore.getState().openDiffTab();
    expect(useSessionStore.getState().activeEditorFile).toBe("__diff__");
    expect(useSessionStore.getState().openEditorFiles).toHaveLength(2);
  });

  it("clearDiffFiles resets diff state", () => {
    useSessionStore.setState({
      diffFiles: [
        { path: "a", oldContent: "", newContent: "", status: "added" as const, submodule: "" },
      ],
      diffLoading: true,
    });
    useSessionStore.getState().clearDiffFiles();
    const state = useSessionStore.getState();
    expect(state.diffFiles).toHaveLength(0);
    expect(state.diffLoading).toBe(false);
  });

  it("openEditorFile defaults type to file", () => {
    useSessionStore.getState().openEditorFile("test.go", "content", "go");
    const f = useSessionStore.getState().openEditorFiles[0];
    expect(f.type).toBe("file");
  });

  it("exitScopeMode clears diff state", () => {
    const store = useSessionStore.getState();
    store.enterScopeMode("s1");
    useSessionStore.setState({
      diffFiles: [
        { path: "a", oldContent: "", newContent: "", status: "modified" as const, submodule: "" },
      ],
      diffLoading: true,
    });
    useSessionStore.getState().exitScopeMode();
    const state = useSessionStore.getState();
    expect(state.diffFiles).toHaveLength(0);
    expect(state.diffLoading).toBe(false);
  });
});
