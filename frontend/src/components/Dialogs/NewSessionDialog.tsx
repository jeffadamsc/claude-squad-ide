import { useState } from "react";
import type { CreateOptions } from "../../lib/wails";

interface NewSessionDialogProps {
  onSubmit: (opts: CreateOptions) => void;
  onCancel: () => void;
  profiles: { Name: string; Program: string }[];
  defaultWorkDir: string;
}

export function NewSessionDialog({
  onSubmit,
  onCancel,
  profiles,
  defaultWorkDir,
}: NewSessionDialogProps) {
  const [title, setTitle] = useState("");
  const [path, setPath] = useState(defaultWorkDir);
  const [program, setProgram] = useState(profiles[0]?.Program ?? "claude");

  const inputStyle: React.CSSProperties = {
    width: "100%",
    padding: "8px 12px",
    background: "var(--surface0)",
    border: "1px solid var(--surface1)",
    borderRadius: 4,
    color: "var(--text)",
    fontSize: 13,
    outline: "none",
  };

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.5)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        zIndex: 2000,
      }}
      onClick={onCancel}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: "var(--base)",
          border: "1px solid var(--surface0)",
          borderRadius: 8,
          padding: 24,
          width: 400,
        }}
      >
        <h3 style={{ marginBottom: 16 }}>New Session</h3>

        <label style={{ fontSize: 12, color: "var(--subtext0)", display: "block", marginBottom: 4 }}>
          Title
        </label>
        <input
          style={inputStyle}
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="fix-auth-bug"
          autoFocus
        />

        <label style={{ fontSize: 12, color: "var(--subtext0)", display: "block", marginTop: 12, marginBottom: 4 }}>
          Directory
        </label>
        <input
          style={inputStyle}
          value={path}
          onChange={(e) => setPath(e.target.value)}
        />

        <label style={{ fontSize: 12, color: "var(--subtext0)", display: "block", marginTop: 12, marginBottom: 4 }}>
          Program
        </label>
        <select
          style={{ ...inputStyle, cursor: "pointer" }}
          value={program}
          onChange={(e) => setProgram(e.target.value)}
        >
          {profiles.map((p) => (
            <option key={p.Name} value={p.Program}>
              {p.Name}
            </option>
          ))}
        </select>

        <div style={{ display: "flex", gap: 8, marginTop: 20, justifyContent: "flex-end" }}>
          <button
            onClick={onCancel}
            style={{
              padding: "8px 16px",
              background: "var(--surface0)",
              color: "var(--text)",
              border: "none",
              borderRadius: 6,
              cursor: "pointer",
            }}
          >
            Cancel
          </button>
          <button
            onClick={() => onSubmit({ title, path, program })}
            disabled={!title.trim()}
            style={{
              padding: "8px 16px",
              background: "var(--blue)",
              color: "var(--crust)",
              border: "none",
              borderRadius: 6,
              cursor: "pointer",
              opacity: title.trim() ? 1 : 0.5,
            }}
          >
            Create
          </button>
        </div>
      </div>
    </div>
  );
}
