import { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { AttachAddon } from "@xterm/addon-attach";
import { FitAddon } from "@xterm/addon-fit";
import { theme } from "../lib/theme";

interface UseTerminalOptions {
  sessionId: string;
  wsPort: number;
}

export function useTerminal(
  containerRef: React.RefObject<HTMLDivElement | null>,
  options: UseTerminalOptions
) {
  const termRef = useRef<Terminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitRef = useRef<FitAddon | null>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container || !options.sessionId || !options.wsPort) return;

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

    const ws = new WebSocket(
      `ws://127.0.0.1:${options.wsPort}/ws/${options.sessionId}`
    );
    ws.binaryType = "arraybuffer";

    ws.onopen = () => {
      const attach = new AttachAddon(ws);
      term.loadAddon(attach);

      const dims = fit.proposeDimensions();
      if (dims) {
        ws.send(
          JSON.stringify({ type: "resize", rows: dims.rows, cols: dims.cols })
        );
      }
    };

    const resizeObserver = new ResizeObserver(() => {
      fit.fit();
      const dims = fit.proposeDimensions();
      if (dims && ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({ type: "resize", rows: dims.rows, cols: dims.cols })
        );
      }
    });
    resizeObserver.observe(container);

    termRef.current = term;
    wsRef.current = ws;
    fitRef.current = fit;

    return () => {
      resizeObserver.disconnect();
      ws.close();
      term.dispose();
      termRef.current = null;
      wsRef.current = null;
      fitRef.current = null;
    };
  }, [options.sessionId, options.wsPort, containerRef]);

  return { termRef, wsRef, fitRef };
}
