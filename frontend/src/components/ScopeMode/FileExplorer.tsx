import { useState, useEffect, useCallback, useRef } from "react";
import { useSessionStore } from "../../store/sessionStore";
import { FileTreeItem } from "./FileTreeItem";
import { FileContextMenu, ContextMenuAction } from "./FileContextMenu";
import { api } from "../../lib/wails";
import { detectLanguage } from "../../store/sessionStore";

interface FileExplorerProps {
  sessionId: string;
}

interface ContextMenuState {
  x: number;
  y: number;
  path: string;
  isDir: boolean;
  parentDir: string;
}

interface InlineInputState {
  parentDir: string;
  type: "file" | "folder";
  /** When renaming, the original path */
  renamingPath?: string;
  renamingName?: string;
}

export function FileExplorer({ sessionId }: FileExplorerProps) {
  const explorerTree = useSessionStore((s) => s.explorerTree);
  const setExplorerEntries = useSessionStore((s) => s.setExplorerEntries);
  const clearExplorerTree = useSessionStore((s) => s.clearExplorerTree);
  const openEditorFile = useSessionStore((s) => s.openEditorFile);
  const activeEditorFile = useSessionStore((s) => s.activeEditorFile);
  const openDiffTab = useSessionStore((s) => s.openDiffTab);
  const [expandedDirs, setExpandedDirs] = useState<Set<string>>(
    new Set(["."])
  );
  const [loadingDirs, setLoadingDirs] = useState<Set<string>>(new Set());
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
  const [inlineInput, setInlineInput] = useState<InlineInputState | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<{ path: string; name: string } | null>(null);

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

  // Reload a directory and all its expanded ancestors
  const refreshDir = useCallback(
    async (dirPath: string) => {
      await loadDir(dirPath);
    },
    [loadDir]
  );

  useEffect(() => {
    loadDir(".");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId]);

  // Auto-refresh expanded directories periodically to pick up external changes
  useEffect(() => {
    const interval = setInterval(() => {
      for (const dir of expandedDirs) {
        loadDir(dir);
      }
    }, 5000);
    return () => clearInterval(interval);
  }, [expandedDirs, loadDir]);

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

  const parentOf = (path: string): string => {
    const idx = path.lastIndexOf("/");
    return idx > 0 ? path.substring(0, idx) : ".";
  };

  const handleContextMenu = (
    e: React.MouseEvent,
    path: string,
    isDir: boolean
  ) => {
    e.preventDefault();
    e.stopPropagation();
    setContextMenu({
      x: e.clientX,
      y: e.clientY,
      path,
      isDir,
      parentDir: isDir ? path : parentOf(path),
    });
  };

  const handleBackgroundContextMenu = (e: React.MouseEvent) => {
    e.preventDefault();
    setContextMenu({
      x: e.clientX,
      y: e.clientY,
      path: ".",
      isDir: true,
      parentDir: ".",
    });
  };

  const startNewFile = (parentDir: string) => {
    // Expand parent dir if not expanded
    setExpandedDirs((prev) => {
      const next = new Set(prev);
      next.add(parentDir);
      return next;
    });
    if (!explorerTree.has(parentDir)) loadDir(parentDir);
    setInlineInput({ parentDir, type: "file" });
  };

  const startNewFolder = (parentDir: string) => {
    setExpandedDirs((prev) => {
      const next = new Set(prev);
      next.add(parentDir);
      return next;
    });
    if (!explorerTree.has(parentDir)) loadDir(parentDir);
    setInlineInput({ parentDir, type: "folder" });
  };

  const startRename = (path: string, isDir: boolean) => {
    const name = path.includes("/") ? path.split("/").pop()! : path;
    const parent = parentOf(path);
    setInlineInput({
      parentDir: parent,
      type: isDir ? "folder" : "file",
      renamingPath: path,
      renamingName: name,
    });
  };

  const handleInlineSubmit = async (value: string) => {
    if (!inlineInput || !value.trim()) {
      setInlineInput(null);
      return;
    }

    const name = value.trim();
    const parentDir = inlineInput.parentDir;

    try {
      if (inlineInput.renamingPath) {
        // Rename
        const newPath =
          parentDir === "." ? name : `${parentDir}/${name}`;
        await api().RenamePath(sessionId, inlineInput.renamingPath, newPath);
        await refreshDir(parentDir);
      } else if (inlineInput.type === "file") {
        const filePath =
          parentDir === "." ? name : `${parentDir}/${name}`;
        await api().CreateFile(sessionId, filePath);
        await refreshDir(parentDir);
        // Open the new file in editor
        openEditorFile(filePath, "", detectLanguage(filePath));
      } else {
        const dirPath =
          parentDir === "." ? name : `${parentDir}/${name}`;
        await api().CreateDirectory(sessionId, dirPath);
        await refreshDir(parentDir);
      }
    } catch (err) {
      console.error("File operation failed:", err);
    }

    setInlineInput(null);
  };

  const handleDelete = (path: string) => {
    const name = path.includes("/") ? path.split("/").pop()! : path;
    setDeleteConfirm({ path, name });
  };

  const confirmDelete = async () => {
    if (!deleteConfirm) return;
    try {
      await api().DeletePath(sessionId, deleteConfirm.path);
      await refreshDir(parentOf(deleteConfirm.path));
    } catch (err) {
      console.error("Delete failed:", err);
    }
    setDeleteConfirm(null);
  };

  const handleCopyPath = (path: string) => {
    navigator.clipboard.writeText(path).catch(console.error);
  };

  const getContextMenuActions = (): ContextMenuAction[] => {
    if (!contextMenu) return [];
    const { path, isDir, parentDir } = contextMenu;
    const actions: ContextMenuAction[] = [];

    actions.push({
      label: "New File...",
      onClick: () => startNewFile(isDir ? path : parentDir),
    });
    actions.push({
      label: "New Folder...",
      onClick: () => startNewFolder(isDir ? path : parentDir),
    });

    if (path !== ".") {
      actions.push({ label: "", onClick: () => {}, separator: true });
      actions.push({
        label: "Rename...",
        onClick: () => startRename(path, isDir),
      });
      actions.push({
        label: "Copy Path",
        onClick: () => handleCopyPath(path),
      });
      actions.push({ label: "", onClick: () => {}, separator: true });
      actions.push({
        label: "Delete",
        onClick: () => handleDelete(path),
        danger: true,
      });
    }

    return actions;
  };

  const renderTree = (dirPath: string, depth: number): JSX.Element[] => {
    const entries = explorerTree.get(dirPath);
    if (!entries) return [];
    const sorted = [...entries].sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    const elements: JSX.Element[] = [];

    // Show inline input at the top of this directory if creating here
    if (
      inlineInput &&
      !inlineInput.renamingPath &&
      inlineInput.parentDir === dirPath
    ) {
      elements.push(
        <InlineNameInput
          key="__new__"
          depth={depth}
          isDir={inlineInput.type === "folder"}
          defaultValue=""
          onSubmit={handleInlineSubmit}
          onCancel={() => setInlineInput(null)}
        />
      );
    }

    for (const entry of sorted) {
      const entryPath =
        dirPath === "." ? entry.name : `${dirPath}/${entry.name}`;

      // Show inline rename input instead of normal item
      if (inlineInput?.renamingPath === entryPath) {
        elements.push(
          <InlineNameInput
            key={entryPath + "__rename"}
            depth={depth}
            isDir={entry.isDir}
            defaultValue={inlineInput.renamingName || entry.name}
            onSubmit={handleInlineSubmit}
            onCancel={() => setInlineInput(null)}
          />
        );
      } else {
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
            onContextMenu={(e) => handleContextMenu(e, entryPath, entry.isDir)}
          />
        );
      }
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
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          <span
            onClick={() => startNewFile(".")}
            style={{ cursor: "pointer", fontSize: 13 }}
            title="New file"
          >
            +
          </span>
          <span
            onClick={() => startNewFolder(".")}
            style={{ cursor: "pointer", fontSize: 12 }}
            title="New folder"
          >
            {"\uD83D\uDCC1"}
          </span>
          <span
            onClick={() => openDiffTab()}
            style={{ cursor: "pointer", fontSize: 12 }}
            title="Show changes"
          >
            {"\u0394"}
          </span>
          <span
            onClick={handleRefresh}
            style={{ cursor: "pointer", fontSize: 13 }}
          >
            {"\u21BB"}
          </span>
        </div>
      </div>
      <div
        style={{ flex: 1, overflowY: "auto", paddingTop: 4 }}
        onContextMenu={handleBackgroundContextMenu}
      >
        {renderTree(".", 0)}
      </div>
      {contextMenu && (
        <FileContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          actions={getContextMenuActions()}
          onClose={() => setContextMenu(null)}
        />
      )}
      {deleteConfirm && (
        <DeleteConfirmDialog
          name={deleteConfirm.name}
          onConfirm={confirmDelete}
          onCancel={() => setDeleteConfirm(null)}
        />
      )}
    </div>
  );
}

function InlineNameInput({
  depth,
  isDir,
  defaultValue,
  onSubmit,
  onCancel,
}: {
  depth: number;
  isDir: boolean;
  defaultValue: string;
  onSubmit: (value: string) => void;
  onCancel: () => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const input = inputRef.current;
    if (!input) return;
    input.focus();
    if (defaultValue) {
      // Select filename without extension for rename
      const dotIdx = defaultValue.lastIndexOf(".");
      if (dotIdx > 0 && !isDir) {
        input.setSelectionRange(0, dotIdx);
      } else {
        input.select();
      }
    }
  }, [defaultValue, isDir]);

  return (
    <div
      style={{
        padding: "1px 8px",
        paddingLeft: depth * 16 + 8,
        display: "flex",
        alignItems: "center",
        gap: 4,
      }}
    >
      {isDir ? (
        <span style={{ color: "#cba6f7", fontSize: 11, width: 14, textAlign: "center" }}>
          {"\u25B6"}
        </span>
      ) : (
        <span style={{ width: 14 }} />
      )}
      <input
        ref={inputRef}
        defaultValue={defaultValue}
        style={{
          flex: 1,
          background: "var(--surface0)",
          border: "1px solid var(--blue)",
          borderRadius: 2,
          color: "var(--text)",
          fontSize: 13,
          padding: "1px 4px",
          outline: "none",
          fontFamily: "inherit",
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            onSubmit((e.target as HTMLInputElement).value);
          } else if (e.key === "Escape") {
            onCancel();
          }
        }}
        onBlur={(e) => {
          const val = e.target.value.trim();
          if (val && val !== defaultValue) {
            onSubmit(val);
          } else {
            onCancel();
          }
        }}
      />
    </div>
  );
}

function DeleteConfirmDialog({
  name,
  onConfirm,
  onCancel,
}: {
  name: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onCancel();
      if (e.key === "Enter") onConfirm();
    };
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [onConfirm, onCancel]);

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 1100,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "rgba(0,0,0,0.5)",
      }}
      onClick={onCancel}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: "var(--surface0)",
          border: "1px solid var(--surface1)",
          borderRadius: 8,
          padding: "16px 20px",
          minWidth: 280,
          boxShadow: "0 8px 24px rgba(0,0,0,0.5)",
        }}
      >
        <div style={{ fontSize: 13, color: "var(--text)", marginBottom: 16 }}>
          Delete <strong>{name}</strong>?
        </div>
        <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
          <button
            onClick={onCancel}
            style={{
              padding: "4px 12px",
              fontSize: 12,
              background: "var(--surface1)",
              color: "var(--text)",
              border: "1px solid var(--surface2)",
              borderRadius: 4,
              cursor: "pointer",
            }}
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            autoFocus
            style={{
              padding: "4px 12px",
              fontSize: 12,
              background: "var(--red)",
              color: "var(--base)",
              border: "none",
              borderRadius: 4,
              cursor: "pointer",
            }}
          >
            Delete
          </button>
        </div>
      </div>
    </div>
  );
}
