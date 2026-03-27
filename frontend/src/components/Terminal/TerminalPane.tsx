import { useRef, useEffect, useState, useCallback } from "react";
import { useTerminal } from "../../hooks/useTerminal";
import { useSessionStore } from "../../store/sessionStore";
import { OnFileDrop, OnFileDropOff } from "../../../wailsjs/runtime/runtime";
import "@xterm/xterm/css/xterm.css";

interface TerminalPaneProps {
  sessionId: string;
  wsPort: number;
  focused?: boolean;
  instanceId?: string;
}

/**
 * Shell-escape a file path for safe pasting into a terminal.
 * Wraps in single quotes and escapes any embedded single quotes.
 */
function shellEscape(path: string): string {
  if (/^[a-zA-Z0-9_./@:-]+$/.test(path)) return path;
  return "'" + path.replace(/'/g, "'\\''") + "'";
}

export function TerminalPane({ sessionId, wsPort, focused, instanceId }: TerminalPaneProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const { disconnected, wsRef } = useTerminal(containerRef, { sessionId, wsPort });
  const [dragOver, setDragOver] = useState(false);

  const status = useSessionStore((s) => instanceId ? s.statuses.get(instanceId) : undefined);
  const sshDisconnected = status?.sshConnected === false;

  const showOverlay = sshDisconnected || disconnected;

  // Handle file drops via Wails runtime
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    OnFileDrop((x: number, y: number, paths: string[]) => {
      const rect = container.getBoundingClientRect();
      // Check if the drop landed on this terminal
      if (x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom) {
        const ws = wsRef.current;
        if (ws && ws.readyState === WebSocket.OPEN && paths.length > 0) {
          const text = paths.map(shellEscape).join(" ");
          const encoder = new TextEncoder();
          ws.send(encoder.encode(text));
        }
      }
      setDragOver(false);
    }, false);

    return () => {
      OnFileDropOff();
    };
  }, [wsRef]);

  // Track drag-over state via native DOM events for visual feedback
  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(true);
  }, []);

  const handleDragLeave = useCallback(() => {
    setDragOver(false);
  }, []);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
  }, []);

  return (
    <div style={{ position: "relative", flex: 1, height: "100%" }}>
      <div
        ref={containerRef}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        style={{
          flex: 1,
          height: "100%",
          border: dragOver
            ? "2px solid var(--blue)"
            : focused
              ? "2px solid var(--blue)"
              : "2px solid transparent",
          borderRadius: 2,
          overflow: "hidden",
          "--wails-drop-target": "drop",
        } as React.CSSProperties}
      />
      {dragOver && !showOverlay && (
        <div
          style={{
            position: "absolute",
            inset: 0,
            background: "rgba(137, 180, 250, 0.08)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            zIndex: 10,
            borderRadius: 2,
            pointerEvents: "none",
          }}
        >
          <div style={{ color: "var(--blue)", fontSize: 13, opacity: 0.9 }}>
            Drop file to insert path
          </div>
        </div>
      )}
      {showOverlay && (
        <div
          style={{
            position: "absolute",
            inset: 0,
            background: "rgba(0,0,0,0.6)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            flexDirection: "column",
            gap: 12,
            zIndex: 10,
            borderRadius: 2,
          }}
        >
          <div style={{ fontSize: 24 }}>{"\u23F3"}</div>
          <div style={{ color: "var(--subtext0)", fontSize: 13 }}>
            Connection lost — reconnecting...
          </div>
        </div>
      )}
    </div>
  );
}
