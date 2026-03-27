import { useEffect, useRef } from "react";

export interface ContextMenuAction {
  label: string;
  shortcut?: string;
  onClick: () => void;
  danger?: boolean;
  separator?: boolean;
}

interface FileContextMenuProps {
  x: number;
  y: number;
  actions: ContextMenuAction[];
  onClose: () => void;
}

export function FileContextMenu({ x, y, actions, onClose }: FileContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose();
      }
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("mousedown", handleClick);
    document.addEventListener("keydown", handleKey);
    return () => {
      document.removeEventListener("mousedown", handleClick);
      document.removeEventListener("keydown", handleKey);
    };
  }, [onClose]);

  // Adjust position to keep menu within viewport
  useEffect(() => {
    const menu = menuRef.current;
    if (!menu) return;
    const rect = menu.getBoundingClientRect();
    if (rect.right > window.innerWidth) {
      menu.style.left = `${window.innerWidth - rect.width - 4}px`;
    }
    if (rect.bottom > window.innerHeight) {
      menu.style.top = `${window.innerHeight - rect.height - 4}px`;
    }
  }, [x, y]);

  return (
    <div
      ref={menuRef}
      style={{
        position: "fixed",
        left: x,
        top: y,
        zIndex: 1000,
        background: "var(--surface0)",
        border: "1px solid var(--surface1)",
        borderRadius: 6,
        padding: "4px 0",
        minWidth: 180,
        boxShadow: "0 4px 12px rgba(0,0,0,0.4)",
      }}
    >
      {actions.map((action, i) =>
        action.separator ? (
          <div
            key={i}
            style={{
              height: 1,
              background: "var(--surface1)",
              margin: "4px 0",
            }}
          />
        ) : (
          <ContextMenuItem key={i} action={action} onClose={onClose} />
        )
      )}
    </div>
  );
}

function ContextMenuItem({ action, onClose }: { action: ContextMenuAction; onClose: () => void }) {
  return (
    <div
      onClick={() => {
        action.onClick();
        onClose();
      }}
      style={{
        padding: "5px 12px",
        fontSize: 12,
        cursor: "pointer",
        display: "flex",
        justifyContent: "space-between",
        alignItems: "center",
        color: action.danger ? "var(--red)" : "var(--text)",
      }}
      onMouseEnter={(e) => {
        (e.currentTarget as HTMLDivElement).style.background = "var(--surface1)";
      }}
      onMouseLeave={(e) => {
        (e.currentTarget as HTMLDivElement).style.background = "transparent";
      }}
    >
      <span>{action.label}</span>
      {action.shortcut && (
        <span style={{ color: "var(--overlay0)", fontSize: 11, marginLeft: 16 }}>
          {action.shortcut}
        </span>
      )}
    </div>
  );
}
