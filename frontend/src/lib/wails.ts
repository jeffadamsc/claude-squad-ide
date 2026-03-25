export interface SessionInfo {
  id: string;
  title: string;
  path: string;
  branch: string;
  program: string;
  status: "running" | "ready" | "loading" | "paused";
}

export interface SessionStatus {
  id: string;
  status: "running" | "ready" | "loading" | "paused";
  branch: string;
  diffStats: { added: number; removed: number };
  hasPrompt: boolean;
}

export interface CreateOptions {
  title: string;
  path: string;
  program: string;
  branch?: string;
  autoYes?: boolean;
  inPlace?: boolean;
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
          StartSession(id: string): Promise<void>;
          PauseSession(id: string): Promise<void>;
          ResumeSession(id: string): Promise<void>;
          KillSession(id: string): Promise<void>;
          PushSession(id: string, createPR: boolean): Promise<void>;
          PollAllStatuses(): Promise<SessionStatus[]>;
          GetWebSocketPort(): Promise<number>;
          GetConfig(): Promise<AppConfig>;
        };
      };
    };
  }
}

export const api = () => window.go.app.SessionAPI;
