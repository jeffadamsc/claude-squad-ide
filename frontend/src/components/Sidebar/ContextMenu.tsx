import { useEffect, useRef } from "react";

interface ContextMenuProps {
  x: number;
  y: number;
  onClose: () => void;
  items: { label: string; onClick: () => void; danger?: boolean }[];
}

export function ContextMenu({ x, y, onClose, items }: ContextMenuProps) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose();
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [onClose]);

  return (
    <div
      ref={ref}
      style={{
        position: "fixed",
        left: x,
        top: y,
        background: "var(--surface0)",
        border: "1px solid var(--surface1)",
        borderRadius: 6,
        padding: 4,
        zIndex: 1000,
        minWidth: 140,
      }}
    >
      {items.map((item) => (
        <div
          key={item.label}
          onClick={() => {
            item.onClick();
            onClose();
          }}
          style={{
            padding: "6px 12px",
            cursor: "pointer",
            borderRadius: 4,
            color: item.danger ? "var(--red)" : "var(--text)",
            fontSize: 13,
          }}
          onMouseEnter={(e) => {
            (e.target as HTMLElement).style.background = "var(--surface1)";
          }}
          onMouseLeave={(e) => {
            (e.target as HTMLElement).style.background = "transparent";
          }}
        >
          {item.label}
        </div>
      ))}
    </div>
  );
}
