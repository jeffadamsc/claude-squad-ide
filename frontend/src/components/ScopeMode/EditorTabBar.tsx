import { useSessionStore } from "../../store/sessionStore";

export function EditorTabBar() {
  const openEditorFiles = useSessionStore((s) => s.openEditorFiles);
  const activeEditorFile = useSessionStore((s) => s.activeEditorFile);
  const setActiveEditorFile = useSessionStore((s) => s.setActiveEditorFile);
  const closeEditorFile = useSessionStore((s) => s.closeEditorFile);

  return (
    <div
      style={{
        display: "flex",
        background: "var(--base)",
        borderBottom: "1px solid var(--surface0)",
        overflowX: "auto",
      }}
    >
      {openEditorFiles.map((file) => {
        const name = file.path.split("/").pop() ?? file.path;
        const active = file.path === activeEditorFile;
        return (
          <div
            key={file.path}
            onClick={() => setActiveEditorFile(file.path)}
            style={{
              padding: "6px 12px",
              background: active ? "var(--mantle)" : "transparent",
              color: active ? "var(--text)" : "var(--overlay0)",
              borderRight: "1px solid var(--surface0)",
              borderBottom: active
                ? "2px solid var(--blue)"
                : "2px solid transparent",
              display: "flex",
              alignItems: "center",
              gap: 6,
              cursor: "pointer",
              fontSize: 12,
              whiteSpace: "nowrap",
            }}
          >
            {name}
            <span
              onClick={(e) => {
                e.stopPropagation();
                closeEditorFile(file.path);
              }}
              style={{
                color: "var(--surface2)",
                fontSize: 11,
                cursor: "pointer",
              }}
            >
              {"\u00D7"}
            </span>
          </div>
        );
      })}
    </div>
  );
}
