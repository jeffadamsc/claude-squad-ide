import { useRef } from "react";
import { useTerminal } from "../../hooks/useTerminal";
import "@xterm/xterm/css/xterm.css";

interface TerminalPaneProps {
  sessionId: string;
  wsPort: number;
  focused?: boolean;
}

export function TerminalPane({ sessionId, wsPort, focused }: TerminalPaneProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  useTerminal(containerRef, { sessionId, wsPort });

  return (
    <div
      ref={containerRef}
      style={{
        flex: 1,
        height: "100%",
        border: focused ? "2px solid var(--blue)" : "2px solid transparent",
        borderRadius: 2,
        overflow: "hidden",
      }}
    />
  );
}
