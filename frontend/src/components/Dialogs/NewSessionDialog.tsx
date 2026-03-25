import { useState, useEffect, useCallback, useRef } from "react";
import type { CreateOptions, DirInfo } from "../../lib/wails";
import { api } from "../../lib/wails";

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
  const [prompt, setPrompt] = useState("");
  const [inPlace, setInPlace] = useState(false);
  const [branch, setBranch] = useState("");
  const [branchSearch, setBranchSearch] = useState("");
  const [branches, setBranches] = useState<string[]>([]);
  const [defaultBranch, setDefaultBranch] = useState("main");
  const [loadingBranches, setLoadingBranches] = useState(false);
  const searchTimeout = useRef<ReturnType<typeof setTimeout>>();

  const newBranchLabel = `New branch (from ${defaultBranch})`;

  // Load branch info when path changes
  const loadDirInfo = useCallback(async (dir: string) => {
    if (!dir.trim()) return;
    setLoadingBranches(true);
    try {
      const info: DirInfo = await api().GetDirInfo(dir);
      setDefaultBranch(info.defaultBranch);
      setBranches(info.branches);
      // Default to origin/<defaultBranch> if available, otherwise new branch
      const originDefault = info.branches.find(
        (b) => b === `origin/${info.defaultBranch}`
      );
      setBranch(originDefault ?? "");
    } catch {
      setBranches([]);
    } finally {
      setLoadingBranches(false);
    }
  }, []);

  useEffect(() => {
    loadDirInfo(path);
  }, [path, loadDirInfo]);

  // Debounced branch search
  const handleBranchSearch = useCallback(
    (filter: string) => {
      setBranchSearch(filter);
      if (searchTimeout.current) clearTimeout(searchTimeout.current);
      searchTimeout.current = setTimeout(async () => {
        try {
          const results = await api().SearchBranches(path, filter);
          setBranches(results);
        } catch {
          // ignore
        }
      }, 200);
    },
    [path]
  );

  const inputStyle: React.CSSProperties = {
    width: "100%",
    padding: "8px 12px",
    background: "var(--surface0)",
    border: "1px solid var(--surface1)",
    borderRadius: 4,
    color: "var(--text)",
    fontSize: 13,
    outline: "none",
    boxSizing: "border-box",
  };

  const labelStyle: React.CSSProperties = {
    fontSize: 12,
    color: "var(--subtext0)",
    display: "block",
    marginTop: 12,
    marginBottom: 4,
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
          width: 460,
          maxHeight: "80vh",
          overflowY: "auto",
        }}
      >
        <h3 style={{ marginBottom: 16, marginTop: 0 }}>New Session</h3>

        <label style={{ ...labelStyle, marginTop: 0 }}>Title</label>
        <input
          style={inputStyle}
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="fix-auth-bug"
          autoFocus
        />

        <label style={labelStyle}>Directory</label>
        <input
          style={inputStyle}
          value={path}
          onChange={(e) => setPath(e.target.value)}
        />

        <label style={labelStyle}>
          <input
            type="checkbox"
            checked={inPlace}
            onChange={(e) => setInPlace(e.target.checked)}
            style={{ marginRight: 6 }}
          />
          Run in-place (no git isolation)
        </label>

        {!inPlace && (
          <>
            <label style={labelStyle}>Branch</label>
            <input
              style={{ ...inputStyle, marginBottom: 4 }}
              value={branchSearch}
              onChange={(e) => handleBranchSearch(e.target.value)}
              placeholder="Search branches..."
            />
            <select
              style={{ ...inputStyle, cursor: "pointer" }}
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
            >
              <option value="">{newBranchLabel}</option>
              {branches.map((b) => (
                <option key={b} value={b}>
                  {b}
                </option>
              ))}
            </select>
            {loadingBranches && (
              <span style={{ fontSize: 11, color: "var(--overlay0)" }}>
                Loading branches...
              </span>
            )}
          </>
        )}

        <label style={labelStyle}>Prompt (optional)</label>
        <textarea
          style={{ ...inputStyle, minHeight: 60, resize: "vertical", fontFamily: "inherit" }}
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          placeholder="Initial prompt for the session..."
        />

        {profiles.length > 1 && (
          <>
            <label style={labelStyle}>Program</label>
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
          </>
        )}

        <div
          style={{
            display: "flex",
            gap: 8,
            marginTop: 20,
            justifyContent: "flex-end",
          }}
        >
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
            onClick={() =>
              onSubmit({
                title,
                path,
                program,
                branch: inPlace ? undefined : branch || undefined,
                inPlace,
                prompt: prompt.trim() || undefined,
              })
            }
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
