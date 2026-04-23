import { useRef, useEffect, useState, useCallback } from "react";
import { useTerminal } from "../../hooks/useTerminal";
import { useSessionStore } from "../../store/sessionStore";
import { OnFileDrop, OnFileDropOff } from "../../../wailsjs/runtime/runtime";
import { api } from "../../lib/wails";
import "@xterm/xterm/css/xterm.css";

interface TerminalPaneProps {
  sessionId: string;
  wsPort: number;
  focused?: boolean;
  instanceId?: string;
}

// Cadence at which TerminalPane pings the backend to mark the session as
// "currently being viewed", so the auto-pauser doesn't kill it. The smallest
// supported idleTimeout setting is 15 minutes, so 60s is plenty of margin
// while staying well under any reasonable IPC noise threshold.
const KEEPALIVE_INTERVAL_MS = 60_000;

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
  const status = useSessionStore((s) => instanceId ? s.statuses.get(instanceId) : undefined);
  const isPaused = status?.status === "paused";
  const { disconnected, wsRef, fitRef, termRef } = useTerminal(containerRef, { sessionId, wsPort, paused: isPaused });
  const [dragOver, setDragOver] = useState(false);
  const [resuming, setResuming] = useState(false);
  const zoomTerminalFont = useSessionStore((s) => s.zoomTerminalFont);
  const openTab = useSessionStore((s) => s.openTab);

  // Keepalive: while this pane is mounted and the tab is visible, periodically
  // touch LastViewed on the backend so the auto-pauser doesn't kill the
  // session out from under us. Skipped when paused (nothing to keep alive)
  // and when the page is hidden (user isn't actually looking).
  useEffect(() => {
    if (!instanceId || isPaused) return;

    const ping = () => {
      if (document.visibilityState !== "visible") return;
      api().TouchSession(instanceId).catch(() => {
        // TouchSession is best-effort; ignore transient failures.
      });
    };

    ping();
    const id = window.setInterval(ping, KEEPALIVE_INTERVAL_MS);
    const onVisibility = () => { if (document.visibilityState === "visible") ping(); };
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.clearInterval(id);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [instanceId, isPaused]);

  // Handle Cmd+/Cmd- for terminal font zoom
  useEffect(() => {
    if (!focused) return;
    const handler = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey)) return;
      if (e.key === "=" || e.key === "+") {
        e.preventDefault();
        zoomTerminalFont(1);
      } else if (e.key === "-") {
        e.preventDefault();
        zoomTerminalFont(-1);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [focused, zoomTerminalFont]);

  // Refit terminal when tab becomes visible again — only scroll if locked to bottom
  useEffect(() => {
    if (focused && fitRef.current) {
      const locked = useSessionStore.getState().getScrollLocked(sessionId);
      console.log(`[scroll] TerminalPane focused effect: locked=${locked} sessionId=${sessionId}`);
      requestAnimationFrame(() => {
        fitRef.current?.fit();
        if (termRef.current && locked) {
          const term = termRef.current;
          term.scrollToBottom();
          requestAnimationFrame(() => {
            term.scrollToBottom();
          });
        }
      });
    }
  }, [focused, fitRef, termRef, sessionId]);

  // Focus terminal when it's the active pane and window regains focus
  useEffect(() => {
    if (!focused) return;
    const term = termRef.current;
    if (term) term.focus();

    const onWindowFocus = () => {
      if (termRef.current) {
        termRef.current.focus();
      }
    };
    window.addEventListener("focus", onWindowFocus);
    return () => window.removeEventListener("focus", onWindowFocus);
  }, [focused, termRef]);

  const sshDisconnected = status?.sshConnected === false;

  const showOverlay = sshDisconnected || disconnected || isPaused;

  const handleResume = useCallback(async () => {
    if (!instanceId || resuming) return;
    setResuming(true);
    try {
      const newPtyId = await api().OpenSession(instanceId);
      openTab(instanceId, newPtyId);
    } catch (err) {
      console.error("Failed to resume session:", err);
    } finally {
      setResuming(false);
    }
  }, [instanceId, resuming, openTab]);

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
          {isPaused ? (
            <>
              <div style={{ fontSize: 24 }}>{"⏸"}</div>
              <div style={{ color: "var(--subtext0)", fontSize: 13 }}>
                Session paused due to inactivity
              </div>
              <button
                onClick={handleResume}
                disabled={resuming}
                style={{
                  padding: "6px 14px",
                  background: "var(--blue)",
                  color: "var(--base)",
                  border: "none",
                  borderRadius: 4,
                  fontSize: 13,
                  cursor: resuming ? "default" : "pointer",
                  opacity: resuming ? 0.6 : 1,
                }}
              >
                {resuming ? "Resuming..." : "Resume"}
              </button>
            </>
          ) : (
            <>
              <div style={{ fontSize: 24 }}>{"⏳"}</div>
              <div style={{ color: "var(--subtext0)", fontSize: 13 }}>
                Connection lost — reconnecting...
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}
