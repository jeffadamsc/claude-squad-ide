export interface SubmoduleFailure {
  name: string;
  stage: string;
  error: string;
}

export interface SubmoduleWarningsPayload {
  sessionId: string;
  worktreePath: string;
  failures: SubmoduleFailure[];
}

interface Props {
  payload: SubmoduleWarningsPayload;
  retrying: boolean;
  onRetry: (names: string[]) => void;
  onOpenTerminal: () => void;
  onDismiss: () => void;
}

export function SubmoduleWarningsDialog({
  payload,
  retrying,
  onRetry,
  onOpenTerminal,
  onDismiss,
}: Props) {
  const names = payload.failures.map((f) => f.name);
  const manualCmd =
    names.length === 0
      ? ""
      : `cd '${payload.worktreePath}' && git submodule update --init -- ${names.join(" ")}`;

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.5)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        zIndex: 2000,
      }}
      onClick={onDismiss}
      role="dialog"
      aria-modal="true"
      aria-labelledby="submodule-warnings-title"
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: "var(--base)",
          border: "1px solid var(--surface0)",
          borderRadius: 8,
          padding: 24,
          width: 520,
          maxHeight: "80vh",
          overflowY: "auto",
        }}
      >
        <h3 id="submodule-warnings-title" style={{ marginBottom: 8 }}>
          Some submodules failed to set up
        </h3>
        <p style={{ color: "var(--subtext0)", marginBottom: 12 }}>
          Session <code>{payload.sessionId}</code> finished setup but the following
          submodules were not populated. The session is otherwise usable, but
          anything that depends on these submodules will be missing.
        </p>

        <ul style={{ marginBottom: 16, paddingLeft: 20 }}>
          {payload.failures.map((f) => (
            <li key={f.name} style={{ marginBottom: 6 }}>
              <strong>{f.name}</strong>{" "}
              <span style={{ color: "var(--subtext0)" }}>
                ({f.stage}) — {f.error}
              </span>
            </li>
          ))}
        </ul>

        {manualCmd && (
          <>
            <p style={{ marginBottom: 4 }}>To fix manually in a terminal:</p>
            <pre
              style={{
                background: "var(--surface0)",
                padding: 8,
                borderRadius: 4,
                overflowX: "auto",
                marginBottom: 16,
                fontSize: 12,
              }}
            >
              <code>{manualCmd}</code>
            </pre>
          </>
        )}

        <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
          <button
            onClick={onDismiss}
            style={{
              padding: "8px 16px",
              background: "var(--surface0)",
              color: "var(--text)",
              border: "none",
              borderRadius: 4,
              cursor: "pointer",
            }}
          >
            Dismiss
          </button>
          <button
            onClick={onOpenTerminal}
            style={{
              padding: "8px 16px",
              background: "var(--surface0)",
              color: "var(--text)",
              border: "none",
              borderRadius: 4,
              cursor: "pointer",
            }}
          >
            Open terminal
          </button>
          <button
            onClick={() => onRetry(names)}
            disabled={retrying}
            style={{
              padding: "8px 16px",
              background: retrying ? "var(--surface1)" : "var(--blue)",
              color: "var(--base)",
              border: "none",
              borderRadius: 4,
              cursor: retrying ? "wait" : "pointer",
            }}
          >
            {retrying ? "Retrying…" : "Retry failed"}
          </button>
        </div>
      </div>
    </div>
  );
}
