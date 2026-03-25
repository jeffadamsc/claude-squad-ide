import { useState, useEffect, useCallback } from "react";
import { useSessionStore } from "../../store/sessionStore";
import { FileTreeItem } from "./FileTreeItem";
import { api } from "../../lib/wails";
import { detectLanguage } from "../../store/sessionStore";

interface FileExplorerProps {
  sessionId: string;
}

export function FileExplorer({ sessionId }: FileExplorerProps) {
  const explorerTree = useSessionStore((s) => s.explorerTree);
  const setExplorerEntries = useSessionStore((s) => s.setExplorerEntries);
  const clearExplorerTree = useSessionStore((s) => s.clearExplorerTree);
  const openEditorFile = useSessionStore((s) => s.openEditorFile);
  const activeEditorFile = useSessionStore((s) => s.activeEditorFile);
  const [expandedDirs, setExpandedDirs] = useState<Set<string>>(
    new Set(["."])
  );
  const [loadingDirs, setLoadingDirs] = useState<Set<string>>(new Set());

  const loadDir = useCallback(
    async (dirPath: string) => {
      if (loadingDirs.has(dirPath)) return;
      setLoadingDirs((prev) => {
        const next = new Set(prev);
        next.add(dirPath);
        return next;
      });
      try {
        const entries = await api().ListDirectory(sessionId, dirPath);
        setExplorerEntries(dirPath, entries);
      } catch (err) {
        console.error("Failed to list directory:", err);
      } finally {
        setLoadingDirs((prev) => {
          const next = new Set(prev);
          next.delete(dirPath);
          return next;
        });
      }
    },
    [sessionId, setExplorerEntries, loadingDirs]
  );

  useEffect(() => {
    loadDir(".");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId]);

  const handleToggleDir = (dirPath: string) => {
    const next = new Set(expandedDirs);
    if (next.has(dirPath)) {
      next.delete(dirPath);
    } else {
      next.add(dirPath);
      if (!explorerTree.has(dirPath)) loadDir(dirPath);
    }
    setExpandedDirs(next);
  };

  const handleFileClick = async (filePath: string) => {
    try {
      const contents = await api().ReadFile(sessionId, filePath);
      openEditorFile(filePath, contents, detectLanguage(filePath));
    } catch (err) {
      console.error("Failed to read file:", err);
    }
  };

  const handleRefresh = () => {
    clearExplorerTree();
    setExpandedDirs(new Set(["."]));
    loadDir(".");
  };

  const renderTree = (dirPath: string, depth: number): JSX.Element[] => {
    const entries = explorerTree.get(dirPath);
    if (!entries) return [];
    // Sort: dirs first, then files, alphabetical within each group
    const sorted = [...entries].sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    const elements: JSX.Element[] = [];
    for (const entry of sorted) {
      const entryPath =
        dirPath === "." ? entry.name : `${dirPath}/${entry.name}`;
      elements.push(
        <FileTreeItem
          key={entryPath}
          name={entry.name}
          isDir={entry.isDir}
          depth={depth}
          expanded={expandedDirs.has(entryPath)}
          selected={activeEditorFile === entryPath}
          onClick={() =>
            entry.isDir
              ? handleToggleDir(entryPath)
              : handleFileClick(entryPath)
          }
        />
      );
      if (entry.isDir && expandedDirs.has(entryPath)) {
        elements.push(...renderTree(entryPath, depth + 1));
      }
    }
    return elements;
  };

  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        height: "100%",
        background: "var(--base)",
      }}
    >
      <div
        style={{
          padding: "8px 10px",
          fontSize: 11,
          color: "var(--subtext0)",
          textTransform: "uppercase",
          borderBottom: "1px solid var(--surface0)",
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
        }}
      >
        <span>Explorer</span>
        <span
          onClick={handleRefresh}
          style={{ cursor: "pointer", fontSize: 13 }}
        >
          {"\u21BB"}
        </span>
      </div>
      <div style={{ flex: 1, overflowY: "auto", paddingTop: 4 }}>
        {renderTree(".", 0)}
      </div>
    </div>
  );
}
