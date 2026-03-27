export interface SessionInfo {
  id: string;
  title: string;
  path: string;
  branch: string;
  program: string;
  status: "running" | "ready" | "loading" | "paused";
  hostId?: string;
}

export interface SessionStatus {
  id: string;
  status: "running" | "ready" | "loading" | "paused";
  branch: string;
  diffStats: { added: number; removed: number };
  hasPrompt: boolean;
  sshConnected?: boolean | null;
}

export interface CreateOptions {
  title: string;
  path: string;
  program: string;
  branch?: string;
  autoYes?: boolean;
  inPlace?: boolean;
  prompt?: string;
  hostId?: string;
}

export interface HostInfo {
  id: string;
  name: string;
  host: string;
  port: number;
  user: string;
  authMethod: string;
  keyPath: string;
  lastPath: string;
}

export interface RemoteDirEntry {
  name: string;
  isDir: boolean;
}

export interface DirectoryEntry {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
}

export interface CreateHostOptions {
  name: string;
  host: string;
  port: number;
  user: string;
  authMethod: string;
  keyPath: string;
  secret: string;
}

export interface TestHostResult {
  connectionOK: boolean;
  programOK: boolean;
  message: string;
}

export interface DirInfo {
  defaultBranch: string;
  branches: string[];
}

export interface SymbolDefinition {
  name: string;
  path: string;
  line: number;
  kind: string;
  language: string;
  scope: string;
}

export interface DiffFile {
  path: string;
  oldContent: string;
  newContent: string;
  status: "added" | "modified" | "deleted";
  submodule: string;
}

export interface AppConfig {
  DefaultProgram: string;
  AutoYes: boolean;
  BranchPrefix: string;
  Profiles: { Name: string; Program: string }[];
  DefaultWorkDir: string;
}

declare global {
  interface Window {
    go: {
      app: {
        SessionAPI: {
          CreateSession(opts: CreateOptions): Promise<SessionInfo>;
          LoadSessions(): Promise<SessionInfo[]>;
          DeleteSession(id: string): Promise<void>;
          OpenSession(id: string): Promise<string>;
          StartSession(id: string): Promise<void>;
          PauseSession(id: string): Promise<void>;
          ResumeSession(id: string): Promise<void>;
          KillSession(id: string): Promise<void>;
          PushSession(id: string, createPR: boolean): Promise<void>;
          PollAllStatuses(): Promise<SessionStatus[]>;
          GetWebSocketPort(): Promise<number>;
          GetConfig(): Promise<AppConfig>;
          GetDirInfo(dir: string): Promise<DirInfo>;
          SearchBranches(dir: string, filter: string): Promise<string[]>;
          GetHosts(): Promise<HostInfo[]>;
          CreateHost(opts: CreateHostOptions): Promise<HostInfo>;
          DeleteHost(id: string): Promise<void>;
          TestHost(opts: CreateHostOptions, program: string): Promise<TestHostResult>;
          GetRemoteDirInfo(hostId: string, dir: string): Promise<DirInfo>;
          SearchRemoteBranches(hostId: string, dir: string, filter: string): Promise<string[]>;
          ListRemoteDir(hostId: string, dir: string): Promise<RemoteDirEntry[]>;
          CheckRemoteGitRepo(hostId: string, dir: string): Promise<boolean>;
          SetHostLastPath(hostId: string, path: string): Promise<void>;
          SelectFile(startDir: string): Promise<string>;
          ListDirectory(sessionId: string, dirPath: string): Promise<DirectoryEntry[]>;
          ReadFile(sessionId: string, filePath: string): Promise<string>;
          WriteFile(sessionId: string, filePath: string, contents: string): Promise<void>;
          ListFiles(sessionId: string): Promise<string[]>;
          IndexSession(sessionId: string): Promise<void>;
          StopIndexer(sessionId: string): Promise<void>;
          LookupSymbol(sessionId: string, symbol: string): Promise<SymbolDefinition[]>;
          GetAllSymbols(sessionId: string): Promise<Record<string, SymbolDefinition[]> | null>;
          GetDiffFiles(sessionId: string): Promise<DiffFile[]>;
          CreateFile(sessionId: string, filePath: string): Promise<void>;
          CreateDirectory(sessionId: string, dirPath: string): Promise<void>;
          DeletePath(sessionId: string, targetPath: string): Promise<void>;
          RenamePath(sessionId: string, oldPath: string, newPath: string): Promise<void>;
          CopyPath(sessionId: string, srcPath: string, destPath: string): Promise<void>;
        };
      };
    };
  }
}

export const api = () => window.go.app.SessionAPI;
