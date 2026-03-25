import { useEffect } from "react";
import { Allotment } from "allotment";
import "allotment/dist/style.css";
import { useSessionStore } from "../../store/sessionStore";
import { ScopeSidebar } from "./ScopeSidebar";
import { FileExplorer } from "./FileExplorer";
import { EditorPane } from "./EditorPane";
import { ClaudeTerminal } from "./ClaudeTerminal";

interface ScopeLayoutProps {
  wsPort: number;
}

export function ScopeLayout({ wsPort }: ScopeLayoutProps) {
  const scopeMode = useSessionStore((s) => s.scopeMode);
  const tabs = useSessionStore((s) => s.tabs);
  const sessions = useSessionStore((s) => s.sessions);
  const exitScopeMode = useSessionStore((s) => s.exitScopeMode);

  const sessionId = scopeMode.sessionId;
  const tab = tabs.find((t) => t.sessionId === sessionId);

  // Exit scope mode if scoped session is removed
  useEffect(() => {
    if (sessionId && !sessions.find((s) => s.id === sessionId)) {
      exitScopeMode();
    }
  }, [sessions, sessionId, exitScopeMode]);

  if (!sessionId || !tab) {
    return (
      <div
        style={{
          flex: 1,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          color: "var(--overlay0)",
        }}
      >
        Session not found. Press Ctrl+Shift+S to exit scope mode.
      </div>
    );
  }

  return (
    <div style={{ display: "flex", height: "100%", flex: 1 }}>
      <ScopeSidebar />
      <Allotment>
        <Allotment.Pane preferredSize={200} minSize={150} maxSize={350}>
          <FileExplorer sessionId={sessionId} />
        </Allotment.Pane>
        <Allotment.Pane>
          <EditorPane sessionId={sessionId} />
        </Allotment.Pane>
        <Allotment.Pane preferredSize={300} minSize={200} maxSize={500}>
          <ClaudeTerminal
            sessionId={sessionId}
            ptyId={tab.ptyId}
            wsPort={wsPort}
          />
        </Allotment.Pane>
      </Allotment>
    </div>
  );
}
