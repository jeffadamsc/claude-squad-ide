import { useState, useRef, useEffect, useCallback } from "react";
import { useSessionStore, detectLanguage } from "../../store/sessionStore";
import { fuzzyMatch, type FuzzyResult } from "../../lib/fuzzyMatch";
import { api } from "../../lib/wails";

export function QuickOpen({ sessionId }: { sessionId: string }) {
  const { fileList, quickOpenVisible, setQuickOpenVisible, openEditorFile } =
    useSessionStore();
  const [query, setQuery] = useState("");
  const [selectedIdx, setSelectedIdx] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const results = fuzzyMatch(query, fileList);

  useEffect(() => {
    if (quickOpenVisible) {
      setQuery("");
      setSelectedIdx(0);
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [quickOpenVisible]);

  useEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const item = list.children[selectedIdx] as HTMLElement | undefined;
    item?.scrollIntoView({ block: "nearest" });
  }, [selectedIdx]);

  const close = useCallback(() => {
    setQuickOpenVisible(false);
    setQuery("");
  }, [setQuickOpenVisible]);

  const openFile = useCallback(
    async (path: string) => {
      close();
      try {
        const contents = await api().ReadFile(sessionId, path);
        openEditorFile(path, contents, detectLanguage(path));
      } catch (err) {
        console.error("Failed to open file:", err);
      }
    },
    [sessionId, close, openEditorFile]
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setSelectedIdx((i) => Math.min(i + 1, results.length - 1));
          break;
        case "ArrowUp":
          e.preventDefault();
          setSelectedIdx((i) => Math.max(i - 1, 0));
          break;
        case "Enter":
          e.preventDefault();
          if (results[selectedIdx]) {
            openFile(results[selectedIdx].path);
          }
          break;
        case "Escape":
          e.preventDefault();
          close();
          break;
      }
    },
    [results, selectedIdx, openFile, close]
  );

  if (!quickOpenVisible) return null;

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 1000,
        display: "flex",
        justifyContent: "center",
        paddingTop: 48,
      }}
      onClick={close}
    >
      <div
        style={{
          width: 520,
          maxHeight: "60vh",
          background: "var(--mantle)",
          border: "1px solid var(--surface1)",
          borderRadius: 8,
          boxShadow: "0 8px 32px rgba(0,0,0,0.5)",
          overflow: "hidden",
          display: "flex",
          flexDirection: "column",
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div
          style={{
            padding: "10px 14px",
            borderBottom: "1px solid var(--surface0)",
            display: "flex",
            alignItems: "center",
            gap: 8,
          }}
        >
          <span style={{ color: "var(--overlay0)", fontSize: 14 }}>🔍</span>
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setSelectedIdx(0);
            }}
            onKeyDown={handleKeyDown}
            placeholder="Search files by name..."
            style={{
              flex: 1,
              background: "transparent",
              border: "none",
              outline: "none",
              color: "var(--text)",
              fontSize: 14,
              fontFamily: "inherit",
            }}
          />
        </div>

        <div
          ref={listRef}
          style={{ overflowY: "auto", maxHeight: "calc(60vh - 60px)" }}
        >
          {results.map((r, i) => (
            <QuickOpenItem
              key={r.path}
              result={r}
              selected={i === selectedIdx}
              onClick={() => openFile(r.path)}
            />
          ))}
          {results.length === 0 && query && (
            <div
              style={{
                padding: "16px 14px",
                color: "var(--overlay0)",
                fontSize: 13,
              }}
            >
              No matching files
            </div>
          )}
        </div>

        <div
          style={{
            padding: "6px 14px",
            borderTop: "1px solid var(--surface0)",
            display: "flex",
            justifyContent: "space-between",
            fontSize: 11,
            color: "var(--overlay0)",
          }}
        >
          <span>↑↓ navigate</span>
          <span>⏎ open &nbsp; esc close</span>
        </div>
      </div>
    </div>
  );
}

function QuickOpenItem({
  result,
  selected,
  onClick,
}: {
  result: FuzzyResult;
  selected: boolean;
  onClick: () => void;
}) {
  const path = result.path;
  const lastSlash = path.lastIndexOf("/");
  const basename = path.slice(lastSlash + 1);
  const dir = lastSlash >= 0 ? path.slice(0, lastSlash + 1) : "";

  const highlighted: React.ReactNode[] = [];
  let cursor = 0;
  for (const matchIdx of result.matches) {
    if (matchIdx > cursor) {
      highlighted.push(basename.slice(cursor, matchIdx));
    }
    highlighted.push(
      <span key={matchIdx} style={{ color: "var(--yellow)" }}>
        {basename[matchIdx]}
      </span>
    );
    cursor = matchIdx + 1;
  }
  if (cursor < basename.length) {
    highlighted.push(basename.slice(cursor));
  }

  return (
    <div
      onClick={onClick}
      style={{
        padding: "8px 14px",
        display: "flex",
        justifyContent: "space-between",
        alignItems: "center",
        background: selected ? "var(--surface0)" : "transparent",
        cursor: "pointer",
        fontSize: 13,
      }}
    >
      <span style={{ color: selected ? "var(--text)" : "var(--subtext0)" }}>
        {highlighted.length > 0 ? highlighted : basename}
      </span>
      <span style={{ color: "var(--overlay0)", fontSize: 11 }}>{dir}</span>
    </div>
  );
}
