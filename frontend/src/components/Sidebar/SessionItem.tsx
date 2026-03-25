import type { SessionInfo, SessionStatus } from "../../lib/wails";

interface SessionItemProps {
  session: SessionInfo;
  status?: SessionStatus;
  selected: boolean;
  loading?: boolean;
  flash?: boolean;
  onClick: () => void;
  onContextMenu: (e: React.MouseEvent) => void;
}

const statusColors: Record<string, string> = {
  running: "var(--green)",
  ready: "var(--yellow)",
  loading: "var(--subtext0)",
  paused: "var(--overlay0)",
};

export function SessionItem({
  session,
  status,
  selected,
  loading,
  flash,
  onClick,
  onContextMenu,
}: SessionItemProps) {
  const sshDisconnected = status?.sshConnected === false;
  const color = sshDisconnected ? "var(--red)" : (statusColors[status?.status ?? session.status] ?? "var(--overlay0)");
  const diff = status?.diffStats;

  let background = selected ? "var(--surface0)" : "transparent";
  if (flash) {
    background = "var(--surface1)";
  }

  return (
    <div
      onClick={onClick}
      onContextMenu={onContextMenu}
      style={{
        padding: "10px",
        marginBottom: 4,
        borderRadius: 6,
        background,
        borderLeft: selected ? "3px solid var(--blue)" : "3px solid transparent",
        cursor: "pointer",
        transition: "background 0.4s ease",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
        <span style={{ color, fontSize: 10 }}>
          {loading ? "\u23F3" : sshDisconnected ? "\u26A0" : session.status === "paused" ? "\u23F8" : "\u25CF"}
        </span>
        <span
          style={{
            color: selected ? "var(--text)" : "var(--subtext0)",
            fontSize: 13,
            fontWeight: selected ? 500 : 400,
          }}
        >
          {session.title}
        </span>
      </div>
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          marginTop: 4,
        }}
      >
        <span style={{ color: "var(--overlay0)", fontSize: 11, fontStyle: "italic" }}>
          {status?.branch ?? session.branch}
        </span>
        {diff && (diff.added > 0 || diff.removed > 0) && (
          <span style={{ fontSize: 11 }}>
            <span style={{ color: "var(--green)" }}>+{diff.added}</span>{" "}
            <span style={{ color: "var(--red)" }}>-{diff.removed}</span>
          </span>
        )}
      </div>
    </div>
  );
}
