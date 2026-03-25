import { useState } from "react";
import type { CreateHostOptions, TestHostResult } from "../../lib/wails";
import { api } from "../../lib/wails";

interface AddHostDialogProps {
  program: string;
  onSubmit: (opts: CreateHostOptions) => void;
  onCancel: () => void;
}

export function AddHostDialog({ program, onSubmit, onCancel }: AddHostDialogProps) {
  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState(22);
  const [user, setUser] = useState("");
  const [authMethod, setAuthMethod] = useState<"password" | "key" | "key+passphrase">("key");
  const [secret, setSecret] = useState("");
  const [keyPath, setKeyPath] = useState("");
  const [testResult, setTestResult] = useState<TestHostResult | null>(null);
  const [testing, setTesting] = useState(false);
  const [tested, setTested] = useState(false);

  const inputStyle: React.CSSProperties = {
    width: "100%",
    padding: "8px 12px",
    background: "var(--surface0)",
    border: "1px solid var(--surface1)",
    borderRadius: 4,
    color: "var(--text)",
    fontSize: 13,
    outline: "none",
    boxSizing: "border-box",
  };

  const labelStyle: React.CSSProperties = {
    fontSize: 12,
    color: "var(--subtext0)",
    display: "block",
    marginTop: 12,
    marginBottom: 4,
  };

  const handleTest = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const opts: CreateHostOptions = { name, host, port, user, authMethod, keyPath, secret };
      const result = await api().TestHost(opts, program);
      setTestResult(result);
      if (result.connectionOK && result.programOK) {
        setTested(true);
      }
    } catch (e: any) {
      setTestResult({ connectionOK: false, programOK: false, message: e.message ?? "Test failed" });
    } finally {
      setTesting(false);
    }
  };

  const handleBrowse = async () => {
    try {
      const path = await api().SelectFile("~/.ssh/");
      if (path) setKeyPath(path);
    } catch {
      // User cancelled
    }
  };

  const canTest = host.trim() && user.trim() && (authMethod === "password" ? secret.trim() : keyPath.trim());

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.6)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        zIndex: 3000,
      }}
      onClick={onCancel}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: "var(--base)",
          border: "1px solid var(--surface0)",
          borderRadius: 8,
          padding: 24,
          width: 420,
          maxHeight: "80vh",
          overflowY: "auto",
        }}
      >
        <h3 style={{ marginBottom: 16, marginTop: 0 }}>Add SSH Host</h3>

        <label style={{ ...labelStyle, marginTop: 0 }}>Name</label>
        <input style={inputStyle} value={name} onChange={(e) => setName(e.target.value)} placeholder="dev-server" autoFocus />

        <label style={labelStyle}>Host</label>
        <input style={inputStyle} value={host} onChange={(e) => setHost(e.target.value)} placeholder="192.168.1.50 or hostname" />

        <label style={labelStyle}>Port</label>
        <input style={{ ...inputStyle, width: 100 }} type="number" value={port} onChange={(e) => setPort(parseInt(e.target.value) || 22)} />

        <label style={labelStyle}>User</label>
        <input style={inputStyle} value={user} onChange={(e) => setUser(e.target.value)} placeholder="deploy" />

        <label style={labelStyle}>Auth Method</label>
        <div style={{ display: "flex", gap: 16, marginTop: 4 }}>
          {(["password", "key", "key+passphrase"] as const).map((m) => (
            <label key={m} style={{ fontSize: 13, color: "var(--text)", cursor: "pointer" }}>
              <input
                type="radio"
                name="authMethod"
                checked={authMethod === m}
                onChange={() => { setAuthMethod(m); setTested(false); }}
                style={{ marginRight: 4 }}
              />
              {m === "password" ? "Password" : m === "key" ? "Private Key" : "Key + Passphrase"}
            </label>
          ))}
        </div>

        {authMethod === "password" && (
          <>
            <label style={labelStyle}>Password</label>
            <input style={inputStyle} type="password" value={secret} onChange={(e) => { setSecret(e.target.value); setTested(false); }} />
          </>
        )}

        {(authMethod === "key" || authMethod === "key+passphrase") && (
          <>
            <label style={labelStyle}>Private Key</label>
            <div style={{ display: "flex", gap: 8 }}>
              <input style={{ ...inputStyle, flex: 1 }} value={keyPath} onChange={(e) => { setKeyPath(e.target.value); setTested(false); }} placeholder="~/.ssh/id_ed25519" />
              <button
                onClick={handleBrowse}
                style={{
                  padding: "8px 12px",
                  background: "var(--surface0)",
                  color: "var(--text)",
                  border: "none",
                  borderRadius: 4,
                  cursor: "pointer",
                  fontSize: 13,
                  whiteSpace: "nowrap",
                }}
              >
                Browse
              </button>
            </div>
          </>
        )}

        {authMethod === "key+passphrase" && (
          <>
            <label style={labelStyle}>Key Passphrase</label>
            <input style={inputStyle} type="password" value={secret} onChange={(e) => { setSecret(e.target.value); setTested(false); }} />
          </>
        )}

        {testResult && (
          <div
            style={{
              marginTop: 12,
              padding: "8px 12px",
              borderRadius: 4,
              fontSize: 12,
              background: testResult.connectionOK && testResult.programOK ? "rgba(166,227,161,0.15)" : "rgba(243,139,168,0.15)",
              color: testResult.connectionOK && testResult.programOK ? "var(--green)" : "var(--red)",
            }}
          >
            {testResult.connectionOK && testResult.programOK
              ? "Connection OK, program found"
              : testResult.message}
          </div>
        )}

        <div style={{ display: "flex", gap: 8, marginTop: 20, justifyContent: "flex-end" }}>
          <button
            onClick={handleTest}
            disabled={!canTest || testing}
            style={{
              padding: "8px 16px",
              background: "var(--surface1)",
              color: "var(--text)",
              border: "none",
              borderRadius: 6,
              cursor: canTest && !testing ? "pointer" : "default",
              opacity: canTest && !testing ? 1 : 0.5,
            }}
          >
            {testing ? "Testing..." : "Test"}
          </button>
          <button
            onClick={onCancel}
            style={{
              padding: "8px 16px",
              background: "var(--surface0)",
              color: "var(--text)",
              border: "none",
              borderRadius: 6,
              cursor: "pointer",
            }}
          >
            Cancel
          </button>
          <button
            onClick={() => onSubmit({ name, host, port, user, authMethod, keyPath, secret })}
            disabled={!tested}
            style={{
              padding: "8px 16px",
              background: "var(--blue)",
              color: "var(--crust)",
              border: "none",
              borderRadius: 6,
              cursor: tested ? "pointer" : "default",
              opacity: tested ? 1 : 0.5,
            }}
          >
            OK
          </button>
        </div>
      </div>
    </div>
  );
}
