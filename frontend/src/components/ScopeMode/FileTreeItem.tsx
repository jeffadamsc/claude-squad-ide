import { useState } from "react";

interface FileTreeItemProps {
  name: string;
  isDir: boolean;
  depth: number;
  expanded?: boolean;
  selected?: boolean;
  onClick: () => void;
}

export function FileTreeItem({
  name,
  isDir,
  depth,
  expanded,
  selected,
  onClick,
}: FileTreeItemProps) {
  const [hovered, setHovered] = useState(false);

  return (
    <div
      onClick={onClick}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      style={{
        padding: "2px 8px",
        paddingLeft: depth * 16 + 8,
        display: "flex",
        alignItems: "center",
        gap: 4,
        cursor: "pointer",
        fontSize: 13,
        color: selected ? "var(--blue)" : "var(--text)",
        background: selected
          ? "var(--surface1)"
          : hovered
            ? "var(--surface0)"
            : "transparent",
        whiteSpace: "nowrap",
        overflow: "hidden",
        textOverflow: "ellipsis",
      }}
    >
      {isDir ? (
        <span
          style={{
            color: "#cba6f7",
            fontSize: 11,
            width: 14,
            textAlign: "center",
          }}
        >
          {expanded ? "\u25BC" : "\u25B6"}
        </span>
      ) : (
        <span style={{ width: 14 }} />
      )}
      <span style={{ color: isDir ? "#cba6f7" : "var(--text)" }}>{name}</span>
    </div>
  );
}
