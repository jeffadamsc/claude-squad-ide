import { useSessionStore } from "../store/sessionStore";

export function StatusBar() {
  const tabs = useSessionStore((s) => s.tabs);
  const activeTabId = useSessionStore((s) => s.activeTabId);
  const sessions = useSessionStore((s) => s.sessions);
  const statuses = useSessionStore((s) => s.statuses);
  const scopeMode = useSessionStore((s) => s.scopeMode);

  const activeTab = tabs.find((t) => t.id === activeTabId);
  const session = activeTab
    ? sessions.find((s) => s.id === activeTab.sessionId)
    : null;
  const status = activeTab ? statuses.get(activeTab.sessionId) : null;

  return (
    <div
      style={{
        display: "flex",
        justifyContent: "space-between",
        padding: "4px 12px",
        background: "var(--base)",
        borderTop: "1px solid var(--surface0)",
        fontSize: 11,
        color: "var(--overlay0)",
      }}
    >
      <span>
        {session
          ? `${session.title} \u00B7 ${status?.branch ?? session.branch} \u00B7 ${status?.status ?? session.status}`
          : "No session selected"}
      </span>
      <span>
        {scopeMode.active
          ? "Ctrl+Shift+S: Exit Scope"
          : "Ctrl+Shift+N: New \u00B7 Ctrl+Shift+S: Scope \u00B7 Ctrl+Shift+Q: Quit"}
      </span>
    </div>
  );
}
