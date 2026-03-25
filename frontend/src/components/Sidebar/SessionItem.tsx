import type { SessionInfo, SessionStatus } from "../../lib/wails";

interface SessionItemProps {
  session: SessionInfo;
  status?: SessionStatus;
  selected: boolean;
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
  onClick,
  onContextMenu,
}: SessionItemProps) {
  const color = statusColors[status?.status ?? session.status] ?? "var(--overlay0)";
  const diff = status?.diffStats;

  return (
    <div
      onClick={onClick}
      onContextMenu={onContextMenu}
      style={{
        padding: "10px",
        marginBottom: 4,
        borderRadius: 6,
        background: selected ? "var(--surface0)" : "transparent",
        borderLeft: selected ? "3px solid var(--blue)" : "3px solid transparent",
        cursor: "pointer",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
        <span style={{ color, fontSize: 10 }}>
          {session.status === "paused" ? "\u23F8" : "\u25CF"}
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
