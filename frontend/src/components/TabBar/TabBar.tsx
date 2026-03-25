import { useSessionStore } from "../../store/sessionStore";
import { Tab } from "./Tab";

export function TabBar() {
  const tabs = useSessionStore((s) => s.tabs);
  const activeTabId = useSessionStore((s) => s.activeTabId);
  const sessions = useSessionStore((s) => s.sessions);
  const statuses = useSessionStore((s) => s.statuses);
  const setActiveTab = useSessionStore((s) => s.setActiveTab);
  const closeTab = useSessionStore((s) => s.closeTab);
  const enterScopeMode = useSessionStore((s) => s.enterScopeMode);

  return (
    <div
      style={{
        display: "flex",
        background: "var(--base)",
        borderBottom: "1px solid var(--surface0)",
      }}
    >
      {tabs.map((tab) => {
        const session = sessions.find((s) => s.id === tab.sessionId);
        const status = statuses.get(tab.sessionId);
        if (!session) return null;
        return (
          <Tab
            key={tab.id}
            title={session.title}
            status={status?.status ?? session.status}
            active={tab.id === activeTabId}
            onClick={() => setActiveTab(tab.id)}
            onClose={() => closeTab(tab.id)}
          />
        );
      })}
      <div
        onClick={() => {
          if (activeTabId) {
            const tab = tabs.find((t) => t.id === activeTabId);
            if (tab) enterScopeMode(tab.sessionId);
          }
        }}
        style={{
          padding: "8px 12px",
          color: "var(--overlay0)",
          cursor: activeTabId ? "pointer" : "default",
          opacity: activeTabId ? 1 : 0.3,
          fontSize: 14,
          display: "flex",
          alignItems: "center",
          marginLeft: "auto",
        }}
        title="Scope Mode (Ctrl+Shift+S)"
      >
        {"\uD83D\uDD2C"}
      </div>
    </div>
  );
}
