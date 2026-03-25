import { useSessionStore } from "../../store/sessionStore";
import { TerminalPane } from "./TerminalPane";

interface PaneManagerProps {
  wsPort: number;
}

export function PaneManager({ wsPort }: PaneManagerProps) {
  const tabs = useSessionStore((s) => s.tabs);
  const activeTabId = useSessionStore((s) => s.activeTabId);

  const activeTab = tabs.find((t) => t.id === activeTabId);

  if (!activeTab) {
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
        Open a session from the sidebar or press Ctrl+Shift+N
      </div>
    );
  }

  return (
    <div style={{ flex: 1, display: "flex", overflow: "hidden" }}>
      <TerminalPane
        key={activeTab.ptyId}
        sessionId={activeTab.ptyId}
        wsPort={wsPort}
        focused={true}
        instanceId={activeTab.sessionId}
      />
    </div>
  );
}
