/**
 * App.test.tsx — double-submit guard for CreateSession
 *
 * Approach B: render full <App /> with the global window.go mock from
 * src/test/setup.ts, override CreateSession to be slow, open the
 * NewSessionDialog, fill in a title, and fire two rapid clicks.
 *
 * The guard under test is the `creatingRef` in App.tsx:
 *   if (creatingRef.current) return;  // catches same-tick double-clicks
 *   creatingRef.current = true;
 *   ...
 * Because the ref is set synchronously before the first await, a second
 * invocation in the same JS tick (before any React re-render) is blocked
 * even though `creating` state hasn't propagated yet.
 */

import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { mockSessionAPI } from "./test/setup";
import { useSessionStore } from "./store/sessionStore";
import App from "./App";

beforeEach(() => {
  // App.tsx subscribes to a Wails event via EventsOn at mount; stub the
  // runtime so the listener registration is a no-op in the test environment.
  (window as any).runtime = {
    EventsOnMultiple: vi.fn(() => () => {}),
    EventsOn: vi.fn(() => () => {}),
    EventsOff: vi.fn(),
    EventsEmit: vi.fn(),
  };

  // Reset store to a clean slate
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
  vi.clearAllMocks();

  // Default mocks for App's init effect
  mockSessionAPI.GetWebSocketPort.mockResolvedValue(0);
  mockSessionAPI.GetConfig.mockResolvedValue({
    DefaultProgram: "claude",
    AutoYes: false,
    BranchPrefix: "cs/",
    Profiles: [],
    DefaultWorkDir: "/tmp",
  });
  mockSessionAPI.LoadSessions.mockResolvedValue([]);
  mockSessionAPI.GetHosts.mockResolvedValue([]);
  mockSessionAPI.PollAllStatuses.mockResolvedValue([]);
  mockSessionAPI.GetDirInfo.mockResolvedValue({ defaultBranch: "main", branches: [] });
  mockSessionAPI.StartSession.mockResolvedValue(undefined);
});

describe("App double-submit guard", () => {
  it("calls CreateSession only once for rapid back-to-back clicks", async () => {
    // CreateSession is slow — it won't resolve until we explicitly resolve it.
    // This simulates the Go-side mutex delay that motivated the guard.
    let resolveCreate!: (val: any) => void;
    const createPromise = new Promise<any>((r) => {
      resolveCreate = r;
    });
    mockSessionAPI.CreateSession.mockReturnValue(createPromise);

    render(<App />);

    // Wait for the init effect to set config so the dialog can appear.
    // The Sidebar's "+ New Session" button is only rendered when sidebarVisible=true (default).
    const newSessionBtn = await screen.findByRole("button", { name: /\+\s*New Session/i });
    await act(async () => {
      fireEvent.click(newSessionBtn);
    });

    // The dialog is now open. Fill in the required Title field so canCreate = true.
    const titleInput = await screen.findByPlaceholderText("fix-auth-bug");
    fireEvent.change(titleInput, { target: { value: "my-session" } });

    // Find the Create button (not the Cancel button).
    const createBtn = screen.getByRole("button", { name: /^Create$/i });

    // Fire two clicks in the same synchronous block — the ref guard must
    // intercept the second before any re-render can disable the button.
    fireEvent.click(createBtn);
    fireEvent.click(createBtn);

    // Only one CreateSession call should have been made.
    expect(mockSessionAPI.CreateSession).toHaveBeenCalledTimes(1);

    // Resolve the pending promise so React can finish cleanup without warnings.
    const fakeSession = {
      id: "s-1",
      title: "my-session",
      path: "/tmp",
      branch: "main",
      program: "claude",
      status: "loading" as const,
    };
    await act(async () => {
      resolveCreate(fakeSession);
      // Flush microtasks / state updates
      await createPromise;
    });

    // After resolution the dialog should close and the session appears in the store.
    await waitFor(() => {
      expect(useSessionStore.getState().sessions.some((s) => s.id === "s-1")).toBe(true);
    });
  });

  it("allows a second CreateSession after the first completes", async () => {
    // First call resolves immediately.
    const firstSession = {
      id: "s-1",
      title: "first",
      path: "/tmp",
      branch: "main",
      program: "claude",
      status: "loading" as const,
    };
    mockSessionAPI.CreateSession.mockResolvedValueOnce(firstSession);

    render(<App />);

    // Open dialog and create the first session.
    const newSessionBtn = await screen.findByRole("button", { name: /\+\s*New Session/i });
    await act(async () => {
      fireEvent.click(newSessionBtn);
    });

    const titleInput = await screen.findByPlaceholderText("fix-auth-bug");
    fireEvent.change(titleInput, { target: { value: "first" } });

    const createBtn = screen.getByRole("button", { name: /^Create$/i });
    await act(async () => {
      fireEvent.click(createBtn);
    });

    // Dialog should close after the first submission completes.
    await waitFor(() => {
      expect(screen.queryByPlaceholderText("fix-auth-bug")).not.toBeInTheDocument();
    });

    // Open dialog again for the second session.
    const secondSession = {
      id: "s-2",
      title: "second",
      path: "/tmp",
      branch: "main",
      program: "claude",
      status: "loading" as const,
    };
    mockSessionAPI.CreateSession.mockResolvedValueOnce(secondSession);

    const newSessionBtn2 = screen.getByRole("button", { name: /\+\s*New Session/i });
    await act(async () => {
      fireEvent.click(newSessionBtn2);
    });

    const titleInput2 = await screen.findByPlaceholderText("fix-auth-bug");
    fireEvent.change(titleInput2, { target: { value: "second" } });

    const createBtn2 = screen.getByRole("button", { name: /^Create$/i });
    await act(async () => {
      fireEvent.click(createBtn2);
    });

    // Both calls should have gone through — the guard was reset after the first.
    await waitFor(() => {
      expect(mockSessionAPI.CreateSession).toHaveBeenCalledTimes(2);
    });
  });
});
