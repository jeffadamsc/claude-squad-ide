import { TerminalPane } from "../Terminal/TerminalPane";

interface ClaudeTerminalProps {
  sessionId: string;
  ptyId: string;
  wsPort: number;
}

export function ClaudeTerminal({
  sessionId,
  ptyId,
  wsPort,
}: ClaudeTerminalProps) {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        height: "100%",
        background: "var(--crust)",
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
          alignItems: "center",
          gap: 6,
          background: "var(--base)",
        }}
      >
        <span style={{ color: "#cba6f7" }}>{"\u2B24"}</span>
        Claude Code
      </div>
      <div style={{ flex: 1 }}>
        <TerminalPane
          sessionId={ptyId}
          wsPort={wsPort}
          focused={true}
          instanceId={sessionId}
        />
      </div>
    </div>
  );
}
