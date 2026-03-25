import { useRef } from "react";
import { useTerminal } from "../../hooks/useTerminal";
import { useSessionStore } from "../../store/sessionStore";
import "@xterm/xterm/css/xterm.css";

interface TerminalPaneProps {
  sessionId: string;
  wsPort: number;
  focused?: boolean;
  instanceId?: string;
}

export function TerminalPane({ sessionId, wsPort, focused, instanceId }: TerminalPaneProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  useTerminal(containerRef, { sessionId, wsPort });

  const status = useSessionStore((s) => instanceId ? s.statuses.get(instanceId) : undefined);
  const sshDisconnected = status?.sshConnected === false;

  return (
    <div style={{ position: "relative", flex: 1, height: "100%" }}>
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
      {sshDisconnected && (
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
