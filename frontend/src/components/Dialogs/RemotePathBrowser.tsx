import { useState, useEffect, useCallback } from "react";
import type { RemoteDirEntry } from "../../lib/wails";
import { api } from "../../lib/wails";

interface RemotePathBrowserProps {
  hostId: string;
  initialPath: string;
  onSelect: (path: string) => void;
  onCancel: () => void;
}

export function RemotePathBrowser({ hostId, initialPath, onSelect, onCancel }: RemotePathBrowserProps) {
  const [currentPath, setCurrentPath] = useState(initialPath || "~");
  const [entries, setEntries] = useState<RemoteDirEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [isGitRepo, setIsGitRepo] = useState<boolean | null>(null);
  const [checking, setChecking] = useState(false);

  const loadDir = useCallback(async (path: string) => {
    setLoading(true);
    setError("");
    setIsGitRepo(null);
    try {
      const items = await api().ListRemoteDir(hostId, path);
      setEntries(items);
      setCurrentPath(path);
    } catch (e: any) {
      setError(e.message ?? "Failed to list directory");
    } finally {
      setLoading(false);
    }
  }, [hostId]);

  useEffect(() => {
    loadDir(currentPath);
  }, []);  // eslint-disable-line react-hooks/exhaustive-deps

  const navigate = (name: string) => {
    const next = currentPath === "/" ? "/" + name : currentPath + "/" + name;
    loadDir(next);
  };

  const goUp = () => {
    if (currentPath === "/") return;
    const parts = currentPath.split("/");
    parts.pop();
    const parent = parts.join("/") || "/";
    loadDir(parent);
  };

  const handleSelect = async () => {
    setChecking(true);
    setIsGitRepo(null);
    try {
      const ok = await api().CheckRemoteGitRepo(hostId, currentPath);
      setIsGitRepo(ok);
      if (ok) {
        onSelect(currentPath);
      }
    } catch {
      setIsGitRepo(false);
    } finally {
      setChecking(false);
    }
  };

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.6)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        zIndex: 3500,
      }}
      onMouseDown={onCancel}
    >
      <div
        onMouseDown={(e) => e.stopPropagation()}
        style={{
          background: "var(--base)",
          border: "1px solid var(--surface0)",
          borderRadius: 8,
          padding: 20,
          width: 440,
          maxHeight: "70vh",
          display: "flex",
          flexDirection: "column",
        }}
      >
        <h3 style={{ margin: "0 0 12px 0" }}>Choose Remote Directory</h3>

        <div style={{
          padding: "6px 10px",
          background: "var(--surface0)",
          borderRadius: 4,
          fontSize: 12,
          color: "var(--text)",
          marginBottom: 8,
          wordBreak: "break-all",
          fontFamily: "monospace",
        }}>
          {currentPath}
        </div>

        <div style={{
          flex: 1,
          overflowY: "auto",
          border: "1px solid var(--surface1)",
          borderRadius: 4,
          minHeight: 200,
          maxHeight: 300,
        }}>
          {loading ? (
            <div style={{ padding: 16, color: "var(--overlay0)", fontSize: 13 }}>Loading...</div>
          ) : error ? (
            <div style={{ padding: 16, color: "var(--red)", fontSize: 13 }}>{error}</div>
          ) : (
            <>
              {currentPath !== "/" && (
                <div
                  onClick={goUp}
                  style={{
                    padding: "6px 10px",
                    cursor: "pointer",
                    fontSize: 13,
                    color: "var(--blue)",
                    borderBottom: "1px solid var(--surface0)",
                  }}
                >
                  ..
                </div>
              )}
              {entries.length === 0 && (
                <div style={{ padding: 16, color: "var(--overlay0)", fontSize: 13, fontStyle: "italic" }}>
                  No subdirectories
                </div>
              )}
              {entries.map((e) => (
                <div
                  key={e.name}
                  onClick={() => navigate(e.name)}
                  style={{
                    padding: "6px 10px",
                    cursor: "pointer",
                    fontSize: 13,
                    color: "var(--text)",
                    borderBottom: "1px solid var(--surface0)",
                  }}
                  onMouseEnter={(ev) => { ev.currentTarget.style.background = "var(--surface0)"; }}
                  onMouseLeave={(ev) => { ev.currentTarget.style.background = "transparent"; }}
                >
                  {e.name}/
                </div>
              ))}
            </>
          )}
        </div>

        {isGitRepo === false && (
          <div style={{
            marginTop: 8,
            padding: "6px 10px",
            borderRadius: 4,
            fontSize: 12,
            background: "rgba(243,139,168,0.15)",
            color: "var(--red)",
          }}>
            Not a git repository
          </div>
        )}

        <div style={{ display: "flex", gap: 8, marginTop: 12, justifyContent: "flex-end" }}>
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
            onClick={handleSelect}
            disabled={loading || checking}
            style={{
              padding: "8px 16px",
              background: "var(--blue)",
              color: "var(--crust)",
              border: "none",
              borderRadius: 6,
              cursor: loading || checking ? "default" : "pointer",
              opacity: loading || checking ? 0.5 : 1,
            }}
          >
            {checking ? "Checking..." : "Select"}
          </button>
        </div>
      </div>
    </div>
  );
}
