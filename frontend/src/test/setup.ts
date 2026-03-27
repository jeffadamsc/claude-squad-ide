import { vi } from "vitest";
import "@testing-library/jest-dom/vitest";

// Mock the Wails API globally — all tests run outside the Wails runtime
const mockSessionAPI = {
  CreateSession: vi.fn(),
  LoadSessions: vi.fn().mockResolvedValue([]),
  DeleteSession: vi.fn(),
  OpenSession: vi.fn().mockResolvedValue("pty-1"),
  StartSession: vi.fn(),
  PauseSession: vi.fn(),
  ResumeSession: vi.fn(),
  KillSession: vi.fn(),
  PushSession: vi.fn(),
  PollAllStatuses: vi.fn().mockResolvedValue([]),
  GetWebSocketPort: vi.fn().mockResolvedValue(0),
  GetConfig: vi.fn().mockResolvedValue({
    DefaultProgram: "claude",
    AutoYes: false,
    BranchPrefix: "cs/",
    Profiles: [],
    DefaultWorkDir: "/tmp",
  }),
  GetDirInfo: vi.fn().mockResolvedValue({ defaultBranch: "main", branches: [] }),
  SearchBranches: vi.fn().mockResolvedValue([]),
  GetHosts: vi.fn().mockResolvedValue([]),
  CreateHost: vi.fn(),
  DeleteHost: vi.fn(),
  TestHost: vi.fn(),
  GetRemoteDirInfo: vi.fn(),
  SearchRemoteBranches: vi.fn().mockResolvedValue([]),
  ListRemoteDir: vi.fn().mockResolvedValue([]),
  CheckRemoteGitRepo: vi.fn().mockResolvedValue(false),
  SetHostLastPath: vi.fn(),
  SelectFile: vi.fn().mockResolvedValue(""),
  ListDirectory: vi.fn().mockResolvedValue([]),
  ReadFile: vi.fn().mockResolvedValue(""),
  WriteFile: vi.fn(),
  ListFiles: vi.fn().mockResolvedValue([]),
  IndexSession: vi.fn(),
  StopIndexer: vi.fn(),
  LookupSymbol: vi.fn().mockResolvedValue([]),
};

Object.defineProperty(window, "go", {
  value: { app: { SessionAPI: mockSessionAPI } },
  writable: true,
});

export { mockSessionAPI };
