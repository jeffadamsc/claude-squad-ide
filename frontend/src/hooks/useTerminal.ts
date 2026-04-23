import { useEffect, useRef, useState, useCallback } from "react";
import { Terminal } from "@xterm/xterm";
import { AttachAddon } from "@xterm/addon-attach";
import { FitAddon } from "@xterm/addon-fit";
import { theme } from "../lib/theme";
import { useSessionStore } from "../store/sessionStore";

interface UseTerminalOptions {
  sessionId: string;
  wsPort: number;
  // When true, skip auto-reconnect after a WS close. The session is known to
  // be paused on the backend, so the PTY id is stale — retrying just produces
  // 404s. The parent shows a Resume overlay instead.
  paused?: boolean;
}

const INITIAL_RECONNECT_DELAY = 1000;
const MAX_RECONNECT_DELAY = 10000;

export function useTerminal(
  containerRef: React.RefObject<HTMLDivElement | null>,
  options: UseTerminalOptions
) {
  const termRef = useRef<Terminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const attachRef = useRef<AttachAddon | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectDelay = useRef(INITIAL_RECONNECT_DELAY);
  const intentionalClose = useRef(false);
  const renderDisposeRef = useRef<{ dispose: () => void } | null>(null);
  const renderDisposeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [disconnected, setDisconnected] = useState(false);
  const terminalFontSize = useSessionStore((s) => s.terminalFontSize);

  const { sessionId } = options;
  const pausedRef = useRef(options.paused ?? false);
  pausedRef.current = options.paused ?? false;

  // Read/write scroll lock from the store so it persists across tab switches
  const isLocked = useCallback(() => {
    return useSessionStore.getState().getScrollLocked(sessionId);
  }, [sessionId]);

  const setLocked = useCallback((locked: boolean) => {
    useSessionStore.getState().setScrollLocked(sessionId, locked);
  }, [sessionId]);

  // Check if the terminal viewport is currently scrolled to the bottom.
  // tolerance allows "close to bottom" to count (useful when data is streaming
  // and baseY grows between the wheel event and the RAF check).
  const isAtBottom = useCallback((term: Terminal, tolerance = 0) => {
    const buf = term.buffer.active;
    return buf.viewportY >= buf.baseY - tolerance;
  }, []);

  const connect = useCallback(() => {
    const container = containerRef.current;
    if (!container || !options.sessionId || !options.wsPort) return;

    let term = termRef.current;
    const fit = fitRef.current;

    if (!term) return;

    const ws = new WebSocket(
      `ws://127.0.0.1:${options.wsPort}/ws/${options.sessionId}`
    );
    ws.binaryType = "arraybuffer";

    ws.onopen = () => {
      if (attachRef.current) {
        attachRef.current.dispose();
        attachRef.current = null;
      }

      const attach = new AttachAddon(ws);
      term!.loadAddon(attach);
      attachRef.current = attach;

      reconnectDelay.current = INITIAL_RECONNECT_DELAY;
      setDisconnected(false);

      if (fit) {
        const dims = fit.proposeDimensions();
        if (dims) {
          ws.send(
            JSON.stringify({ type: "resize", rows: dims.rows, cols: dims.cols })
          );
        }
      }

      // Reconnection replays the snapshot — lock to bottom during replay
      if (term) {
        setLocked(true);
        // Clean up any previous render listener before creating a new one
        if (renderDisposeRef.current) renderDisposeRef.current.dispose();
        if (renderDisposeTimerRef.current) clearTimeout(renderDisposeTimerRef.current);
        const rd = term.onRender(() => {
          term!.scrollToBottom();
        });
        renderDisposeRef.current = rd;
        renderDisposeTimerRef.current = setTimeout(() => {
          rd.dispose();
          renderDisposeRef.current = null;
          renderDisposeTimerRef.current = null;
        }, 500);
      }
    };

    ws.onclose = () => {
      if (intentionalClose.current) return;
      setDisconnected(true);
      if (pausedRef.current) return;
      scheduleReconnect();
    };

    ws.onerror = () => {};

    wsRef.current = ws;
  }, [options.sessionId, options.wsPort, containerRef, setLocked]);

  const scheduleReconnect = useCallback(() => {
    if (reconnectTimer.current) return;
    const delay = reconnectDelay.current;
    reconnectDelay.current = Math.min(delay * 2, MAX_RECONNECT_DELAY);
    reconnectTimer.current = setTimeout(() => {
      reconnectTimer.current = null;
      connect();
    }, delay);
  }, [connect]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container || !options.sessionId || !options.wsPort) return;

    intentionalClose.current = false;

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
      fontSize: useSessionStore.getState().terminalFontSize,
      theme: {
        background: theme.crust,
        foreground: theme.text,
        cursor: theme.yellow,
        selectionBackground: theme.surface2,
      },
    });

    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(container);
    fit.fit();

    termRef.current = term;
    fitRef.current = fit;

    // Initial connection
    const ws = new WebSocket(
      `ws://127.0.0.1:${options.wsPort}/ws/${options.sessionId}`
    );
    ws.binaryType = "arraybuffer";

    ws.onopen = () => {
      const attach = new AttachAddon(ws);
      term.loadAddon(attach);
      attachRef.current = attach;

      reconnectDelay.current = INITIAL_RECONNECT_DELAY;
      setDisconnected(false);

      const dims = fit.proposeDimensions();
      if (dims) {
        ws.send(
          JSON.stringify({ type: "resize", rows: dims.rows, cols: dims.cols })
        );
      }

      // Initial connection — lock to bottom during snapshot replay
      setLocked(true);
      if (renderDisposeRef.current) renderDisposeRef.current.dispose();
      if (renderDisposeTimerRef.current) clearTimeout(renderDisposeTimerRef.current);
      const rd = term.onRender(() => {
        term.scrollToBottom();
      });
      renderDisposeRef.current = rd;
      renderDisposeTimerRef.current = setTimeout(() => {
        rd.dispose();
        renderDisposeRef.current = null;
        renderDisposeTimerRef.current = null;
      }, 500);
    };

    ws.onclose = () => {
      if (intentionalClose.current) return;
      setDisconnected(true);
      if (pausedRef.current) return;
      scheduleReconnect();
    };

    ws.onerror = () => {};

    wsRef.current = ws;

    // Detect user scrolling via wheel events on the container.
    // We listen on the container (not .xterm-viewport) because wheel events
    // target xterm's canvas/screen children and may not reach the viewport.
    const onWheel = (e: WheelEvent) => {
      if (e.deltaY < 0) {
        // User scrolled up — unlock
        console.log(`[scroll] wheel UP deltaY=${e.deltaY} locked was=${isLocked()}`);
        setLocked(false);
      } else if (e.deltaY > 0) {
        // User scrolled down — re-lock if they're at or near the bottom.
        // Use a tolerance of 3 lines because new data can arrive between the
        // wheel event and the RAF check, pushing baseY ahead of viewportY.
        requestAnimationFrame(() => {
          const atBot = isAtBottom(term, 3);
          console.log(`[scroll] wheel DOWN RAF atBottom=${atBot} viewportY=${term.buffer.active.viewportY} baseY=${term.buffer.active.baseY}`);
          if (atBot) {
            setLocked(true);
            term.scrollToBottom();
          }
        });
      }
    };
    container.addEventListener("wheel", onWheel, { passive: true });

    // Auto-scroll on new output only when locked to bottom.
    // Deferred to RAF so wheel events can set locked=false first.
    let rafPending = false;
    let writeCount = 0;
    const writeDispose = term.onWriteParsed(() => {
      writeCount++;
      if (writeCount % 20 === 1) {
        console.log(`[scroll] onWriteParsed #${writeCount} locked=${isLocked()} viewportY=${term.buffer.active.viewportY} baseY=${term.buffer.active.baseY}`);
      }
      if (!rafPending) {
        rafPending = true;
        requestAnimationFrame(() => {
          rafPending = false;
          const locked = isLocked();
          if (locked) {
            console.log(`[scroll] onWriteParsed RAF → scrollToBottom`);
            term.scrollToBottom();
          }
        });
      }
    });

    const resizeObserver = new ResizeObserver(() => {
      if (!container.offsetWidth || !container.offsetHeight) return;
      const wasLocked = isLocked();
      fit.fit();
      const dims = fit.proposeDimensions();
      const currentWs = wsRef.current;
      if (dims && dims.rows > 0 && dims.cols > 0 && currentWs && currentWs.readyState === WebSocket.OPEN) {
        currentWs.send(
          JSON.stringify({ type: "resize", rows: dims.rows, cols: dims.cols })
        );
      }
      if (wasLocked) {
        requestAnimationFrame(() => {
          term.scrollToBottom();
          requestAnimationFrame(() => {
            term.scrollToBottom();
          });
        });
      }
    });
    resizeObserver.observe(container);

    return () => {
      intentionalClose.current = true;
      writeDispose.dispose();
      container.removeEventListener("wheel", onWheel);
      if (reconnectTimer.current) {
        clearTimeout(reconnectTimer.current);
        reconnectTimer.current = null;
      }
      // Clean up render-dispose listener and its timer
      if (renderDisposeRef.current) {
        renderDisposeRef.current.dispose();
        renderDisposeRef.current = null;
      }
      if (renderDisposeTimerRef.current) {
        clearTimeout(renderDisposeTimerRef.current);
        renderDisposeTimerRef.current = null;
      }
      resizeObserver.disconnect();
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      if (attachRef.current) {
        attachRef.current.dispose();
        attachRef.current = null;
      }
      term.dispose();
      termRef.current = null;
      fitRef.current = null;
    };
  }, [options.sessionId, options.wsPort, containerRef, connect, scheduleReconnect, isLocked, setLocked, isAtBottom]);

  // Sync font size changes
  useEffect(() => {
    const term = termRef.current;
    const fit = fitRef.current;
    if (!term) return;
    const wasLocked = isLocked();
    term.options.fontSize = terminalFontSize;
    if (fit) {
      fit.fit();
      const dims = fit.proposeDimensions();
      const ws = wsRef.current;
      if (dims && dims.rows > 0 && dims.cols > 0 && ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "resize", rows: dims.rows, cols: dims.cols }));
      }
    }
    if (wasLocked) {
      requestAnimationFrame(() => {
        term.scrollToBottom();
        requestAnimationFrame(() => {
          term.scrollToBottom();
        });
      });
    }
  }, [terminalFontSize, isLocked]);

  return { termRef, wsRef, fitRef, disconnected };
}
