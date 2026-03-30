import { useSessionStore } from "../../store/sessionStore";
import { TerminalPane } from "./TerminalPane";

interface PaneManagerProps {
  wsPort: number;
}

export function PaneManager({ wsPort }: PaneManagerProps) {
  const tabs = useSessionStore((s) => s.tabs);
  const activeTabId = useSessionStore((s) => s.activeTabId);

  if (tabs.length === 0 || !activeTabId) {
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
    <div style={{ flex: 1, display: "flex", overflow: "hidden", position: "relative" }}>
      {tabs.map((tab) => (
        <div
          key={tab.id}
          style={{
            flex: 1,
            display: tab.id === activeTabId ? "flex" : "none",
            overflow: "hidden",
          }}
        >
          <TerminalPane
            key={tab.ptyId}
            sessionId={tab.ptyId}
            wsPort={wsPort}
            focused={tab.id === activeTabId}
            instanceId={tab.sessionId}
          />
        </div>
      ))}
    </div>
  );
}
