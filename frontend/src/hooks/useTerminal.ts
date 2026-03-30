import { useEffect, useRef, useState, useCallback } from "react";
import { Terminal } from "@xterm/xterm";
import { AttachAddon } from "@xterm/addon-attach";
import { FitAddon } from "@xterm/addon-fit";
import { theme } from "../lib/theme";

interface UseTerminalOptions {
  sessionId: string;
  wsPort: number;
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
  const [disconnected, setDisconnected] = useState(false);

  const connect = useCallback(() => {
    const container = containerRef.current;
    if (!container || !options.sessionId || !options.wsPort) return;

    let term = termRef.current;
    const fit = fitRef.current;

    // Create terminal if it doesn't exist yet
    if (!term) return;

    const ws = new WebSocket(
      `ws://127.0.0.1:${options.wsPort}/ws/${options.sessionId}`
    );
    ws.binaryType = "arraybuffer";

    ws.onopen = () => {
      // Detach old addon if any
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
    };

    ws.onclose = () => {
      if (intentionalClose.current) return;
      setDisconnected(true);
      scheduleReconnect();
    };

    ws.onerror = () => {
      // onclose will fire after this, which handles reconnect
    };

    wsRef.current = ws;
  }, [options.sessionId, options.wsPort, containerRef]);

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
      fontSize: 13,
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
    };

    ws.onclose = () => {
      if (intentionalClose.current) return;
      setDisconnected(true);
      scheduleReconnect();
    };

    ws.onerror = () => {};

    wsRef.current = ws;

    const resizeObserver = new ResizeObserver(() => {
      // Skip resize when container is hidden (display:none gives 0 dimensions)
      if (!container.offsetWidth || !container.offsetHeight) return;
      fit.fit();
      const dims = fit.proposeDimensions();
      const currentWs = wsRef.current;
      if (dims && dims.rows > 0 && dims.cols > 0 && currentWs && currentWs.readyState === WebSocket.OPEN) {
        currentWs.send(
          JSON.stringify({ type: "resize", rows: dims.rows, cols: dims.cols })
        );
      }
    });
    resizeObserver.observe(container);

    return () => {
      intentionalClose.current = true;
      if (reconnectTimer.current) {
        clearTimeout(reconnectTimer.current);
        reconnectTimer.current = null;
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
  }, [options.sessionId, options.wsPort, containerRef, connect, scheduleReconnect]);

  return { termRef, wsRef, fitRef, disconnected };
}
