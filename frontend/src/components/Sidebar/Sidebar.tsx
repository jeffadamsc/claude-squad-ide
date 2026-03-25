import { useState, useCallback, useRef } from "react";
import { useSessionStore } from "../../store/sessionStore";
import { SessionItem } from "./SessionItem";
import { ContextMenu } from "./ContextMenu";
import { api } from "../../lib/wails";

interface SidebarProps {
  onNewSession: () => void;
}

export function Sidebar({ onNewSession }: SidebarProps) {
  const sessions = useSessionStore((s) => s.sessions);
  const statuses = useSessionStore((s) => s.statuses);
  const selectedIdx = useSessionStore((s) => s.selectedSidebarIdx);
  const setSelectedIdx = useSessionStore((s) => s.setSelectedSidebarIdx);
  const openTab = useSessionStore((s) => s.openTab);
  const removeSession = useSessionStore((s) => s.removeSession);
  const loadingSessionIds = useSessionStore((s) => s.loadingSessionIds);
  const flashSessionIds = useSessionStore((s) => s.flashSessionIds);

  const [openingSessionId, setOpeningSessionId] = useState<string | null>(null);
  const openingRef = useRef<Set<string>>(new Set());

  const [contextMenu, setContextMenu] = useState<{
    x: number;
    y: number;
    sessionId: string;
  } | null>(null);

  const handleContextMenu = useCallback(
    (e: React.MouseEvent, sessionId: string) => {
      e.preventDefault();
      setContextMenu({ x: e.clientX, y: e.clientY, sessionId });
    },
    []
  );

  const contextSession = sessions.find((s) => s.id === contextMenu?.sessionId);

  return (
    <div
      style={{
        width: 220,
        background: "var(--base)",
        borderRight: "1px solid var(--surface0)",
        display: "flex",
        flexDirection: "column",
        height: "100%",
      }}
    >
      <div
        style={{
          padding: "12px 16px",
          fontWeight: "bold",
          fontSize: 14,
          borderBottom: "1px solid var(--surface0)",
        }}
      >
        Claude Squad
      </div>

      <div style={{ flex: 1, overflowY: "auto", padding: 8 }}>
        {sessions.map((session, idx) => (
          <SessionItem
            key={session.id}
            session={session}
            status={statuses.get(session.id)}
            selected={idx === selectedIdx}
            loading={openingSessionId === session.id || loadingSessionIds.has(session.id)}
            flash={flashSessionIds.has(session.id)}
            onClick={async () => {
              if (openingRef.current.has(session.id)) return;
              setSelectedIdx(idx);
              openingRef.current.add(session.id);
              setOpeningSessionId(session.id);
              try {
                const ptyId = await api().OpenSession(session.id);
                openTab(session.id, ptyId);
              } catch (err) {
                console.error("Failed to open session:", err);
              } finally {
                openingRef.current.delete(session.id);
                setOpeningSessionId(null);
              }
            }}
            onContextMenu={(e) => handleContextMenu(e, session.id)}
          />
        ))}
      </div>

      <div style={{ padding: 12, borderTop: "1px solid var(--surface0)" }}>
        <button
          onClick={onNewSession}
          style={{
            width: "100%",
            padding: 8,
            background: "var(--surface0)",
            color: "var(--text)",
            border: "none",
            borderRadius: 6,
            cursor: "pointer",
            fontSize: 13,
          }}
        >
          + New Session
        </button>
      </div>

      {contextMenu && contextSession && (
        <ContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          onClose={() => setContextMenu(null)}
          items={[
            {
              label: "Open",
              onClick: async () => {
                try {
                  const ptyId = await api().OpenSession(contextSession.id);
                  openTab(contextSession.id, ptyId);
                } catch (err) {
                  console.error("Failed to open session:", err);
                }
              },
            },
            {
              label:
                contextSession.status === "paused" ? "Resume" : "Pause",
              onClick: () => {
                if (contextSession.status === "paused") {
                  api().ResumeSession(contextSession.id);
                } else {
                  api().PauseSession(contextSession.id);
                }
              },
            },
            {
              label: "Delete",
              danger: true,
              onClick: () => {
                api().KillSession(contextSession.id);
                removeSession(contextSession.id);
              },
            },
          ]}
        />
      )}
    </div>
  );
}
