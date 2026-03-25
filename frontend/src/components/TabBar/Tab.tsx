interface TabProps {
  title: string;
  status: string;
  active: boolean;
  onClick: () => void;
  onClose: () => void;
}

const statusColors: Record<string, string> = {
  running: "var(--green)",
  ready: "var(--yellow)",
  loading: "var(--subtext0)",
  paused: "var(--overlay0)",
};

export function Tab({ title, status, active, onClick, onClose }: TabProps) {
  return (
    <div
      onClick={onClick}
      style={{
        padding: "8px 16px",
        background: active ? "var(--mantle)" : "transparent",
        color: active ? "var(--text)" : "var(--overlay0)",
        borderRight: "1px solid var(--surface0)",
        borderBottom: active ? "2px solid var(--blue)" : "2px solid transparent",
        display: "flex",
        alignItems: "center",
        gap: 8,
        cursor: "pointer",
        fontSize: 13,
      }}
    >
      <span style={{ color: statusColors[status] ?? "var(--overlay0)", fontSize: 9 }}>{"\u25CF"}</span>
      {title}
      <span
        onClick={(e) => {
          e.stopPropagation();
          onClose();
        }}
        style={{ color: "var(--surface2)", fontSize: 11, cursor: "pointer" }}
      >
        {"\u00D7"}
      </span>
    </div>
  );
}
