import { useState, useEffect, useCallback, useRef } from "react";
import type { CreateOptions, DirInfo } from "../../lib/wails";
import { api } from "../../lib/wails";
import { AddHostDialog } from "./AddHostDialog";
import { RemotePathBrowser } from "./RemotePathBrowser";
import { useSessionStore } from "../../store/sessionStore";

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
  const hosts = useSessionStore((s) => s.hosts);
  const addHost = useSessionStore((s) => s.addHost);
  const [selectedHostId, setSelectedHostId] = useState("");
  const [showAddHost, setShowAddHost] = useState(false);
  const [showPathBrowser, setShowPathBrowser] = useState(false);
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
  const [pathError, setPathError] = useState("");
  const [pathValidated, setPathValidated] = useState(false);
  const searchTimeout = useRef<ReturnType<typeof setTimeout>>();

  const isRemote = selectedHostId !== "";
  const selectedHost = hosts.find((h) => h.id === selectedHostId);

  // When host changes, update path to last-used path for that host
  useEffect(() => {
    if (isRemote && selectedHost) {
      const lastPath = selectedHost.lastPath || "~";
      setPath(lastPath);
      setPathValidated(false);
      setPathError("");
    } else {
      setPath(defaultWorkDir);
      setPathValidated(false);
      setPathError("");
    }
  }, [selectedHostId]); // eslint-disable-line react-hooks/exhaustive-deps

  const newBranchLabel = `New branch (from ${defaultBranch})`;

  // Load branch info when path changes
  const loadDirInfo = useCallback(async (dir: string) => {
    if (!dir.trim()) return;
    setLoadingBranches(true);
    try {
      const info: DirInfo = selectedHostId
        ? await api().GetRemoteDirInfo(selectedHostId, dir)
        : await api().GetDirInfo(dir);
      setDefaultBranch(info.defaultBranch);
      setBranches(info.branches);
      const originDefault = info.branches.find(
        (b) => b === `origin/${info.defaultBranch}`
      );
      setBranch(originDefault ?? "");
    } catch {
      setBranches([]);
    } finally {
      setLoadingBranches(false);
    }
  }, [selectedHostId]);

  useEffect(() => {
    if (pathValidated || !isRemote) {
      loadDirInfo(path);
    }
  }, [path, pathValidated, loadDirInfo, isRemote]);

  // For local sessions, always load branches when path changes
  useEffect(() => {
    if (!isRemote && path.trim()) {
      loadDirInfo(path);
    }
  }, [path, isRemote]); // eslint-disable-line react-hooks/exhaustive-deps

  // Debounced branch search
  const handleBranchSearch = useCallback(
    (filter: string) => {
      setBranchSearch(filter);
      if (searchTimeout.current) clearTimeout(searchTimeout.current);
      searchTimeout.current = setTimeout(async () => {
        try {
          const results = selectedHostId
            ? await api().SearchRemoteBranches(selectedHostId, path, filter)
            : await api().SearchBranches(path, filter);
          setBranches(results);
        } catch {
          // ignore
        }
      }, 200);
    },
    [path, selectedHostId]
  );

  // Auto-open path browser when remote host is selected and path not yet validated
  useEffect(() => {
    if (isRemote && !pathValidated && !showPathBrowser && !showAddHost) {
      setShowPathBrowser(true);
    }
  }, [selectedHostId]); // eslint-disable-line react-hooks/exhaustive-deps

  const handlePathSelected = async (selectedPath: string) => {
    setPath(selectedPath);
    setPathValidated(true);
    setPathError("");
    setShowPathBrowser(false);
    // Save as last-used path for this host
    if (selectedHostId) {
      api().SetHostLastPath(selectedHostId, selectedPath).catch(() => {});
    }
  };

  const canCreate = title.trim() && (!isRemote || pathValidated);

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
      onMouseDown={onCancel}
    >
      <div
        onMouseDown={(e) => e.stopPropagation()}
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
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
        />

        <label style={labelStyle}>Host</label>
        <div style={{ display: "flex", gap: 8 }}>
          <select
            style={{ ...inputStyle, flex: 1, cursor: "pointer" }}
            value={selectedHostId}
            onChange={(e) => setSelectedHostId(e.target.value)}
          >
            <option value="">localhost</option>
            {hosts.map((h) => (
              <option key={h.id} value={h.id}>{h.name} ({h.host})</option>
            ))}
          </select>
          <button
            onClick={() => setShowAddHost(true)}
            style={{
              padding: "8px 12px",
              background: "var(--surface0)",
              color: "var(--text)",
              border: "none",
              borderRadius: 4,
              cursor: "pointer",
              fontSize: 16,
              lineHeight: 1,
            }}
            title="Add SSH host"
          >
            +
          </button>
        </div>

        <label style={labelStyle}>Directory</label>
        {isRemote ? (
          <div style={{ display: "flex", gap: 8 }}>
            <div
              style={{
                ...inputStyle,
                flex: 1,
                display: "flex",
                alignItems: "center",
                color: pathValidated ? "var(--text)" : "var(--overlay0)",
                fontFamily: "monospace",
                fontSize: 12,
              }}
            >
              {pathValidated ? path : "No directory selected"}
            </div>
            <button
              onClick={() => setShowPathBrowser(true)}
              style={{
                padding: "8px 12px",
                background: "var(--surface0)",
                color: "var(--text)",
                border: "none",
                borderRadius: 4,
                cursor: "pointer",
                fontSize: 13,
                whiteSpace: "nowrap",
              }}
            >
              Browse
            </button>
          </div>
        ) : (
          <input
            style={inputStyle}
            value={path}
            onChange={(e) => setPath(e.target.value)}
            autoCapitalize="off"
            autoCorrect="off"
            spellCheck={false}
          />
        )}
        {pathError && (
          <div style={{ color: "var(--red)", fontSize: 12, marginTop: 4 }}>{pathError}</div>
        )}

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
              autoCapitalize="off"
              autoCorrect="off"
              spellCheck={false}
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
                hostId: selectedHostId || undefined,
              })
            }
            disabled={!canCreate}
            style={{
              padding: "8px 16px",
              background: "var(--blue)",
              color: "var(--crust)",
              border: "none",
              borderRadius: 6,
              cursor: "pointer",
              opacity: canCreate ? 1 : 0.5,
            }}
          >
            Create
          </button>
        </div>
      </div>

      {showAddHost && (
        <AddHostDialog
          program={program}
          onCancel={() => setShowAddHost(false)}
          onSubmit={async (opts) => {
            try {
              const host = await api().CreateHost(opts);
              addHost(host);
              setSelectedHostId(host.id);
              setShowAddHost(false);
            } catch (e: any) {
              console.error("Failed to create host:", e);
            }
          }}
        />
      )}

      {showPathBrowser && selectedHostId && (
        <RemotePathBrowser
          hostId={selectedHostId}
          initialPath={selectedHost?.lastPath || "~"}
          onSelect={handlePathSelected}
          onCancel={() => setShowPathBrowser(false)}
        />
      )}
    </div>
  );
}
