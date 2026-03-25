import { useState, useEffect, useCallback } from "react";
import { Sidebar } from "./components/Sidebar/Sidebar";
import { TabBar } from "./components/TabBar/TabBar";
import { PaneManager } from "./components/Terminal/PaneManager";
import { StatusBar } from "./components/StatusBar";
import { NewSessionDialog } from "./components/Dialogs/NewSessionDialog";
import { useSessionPoller } from "./hooks/useSessionPoller";
import { useHotkeys } from "./hooks/useHotkeys";
import { useSessionStore } from "./store/sessionStore";
import { api } from "./lib/wails";
import type { AppConfig, CreateOptions } from "./lib/wails";

export default function App() {
  const [wsPort, setWsPort] = useState(0);
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [showNewSession, setShowNewSession] = useState(false);
  const sidebarVisible = useSessionStore((s) => s.sidebarVisible);
  const setSessions = useSessionStore((s) => s.setSessions);
  const addSession = useSessionStore((s) => s.addSession);
  const markLoading = useSessionStore((s) => s.markLoading);

  useEffect(() => {
    const init = async () => {
      try {
        const [port, cfg, sessions] = await Promise.all([
          api().GetWebSocketPort(),
          api().GetConfig(),
          api().LoadSessions(),
        ]);
        setWsPort(port);
        setConfig(cfg);
        setSessions(sessions);
      } catch {
        // Wails not ready yet
      }
    };
    init();
  }, [setSessions]);

  useSessionPoller(500);

  useHotkeys({
    onNewSession: () => setShowNewSession(true),
    onDeleteSession: () => {
      // TODO: confirm dialog + kill selected session
    },
    onPushSession: () => {
      // TODO: push selected session
    },
    onTogglePauseResume: () => {
      // TODO: toggle pause/resume on selected session
    },
    onQuit: () => {
      window.close();
    },
  });

  const handleCreateSession = useCallback(
    async (opts: CreateOptions) => {
      try {
        const session = await api().CreateSession(opts);
        addSession(session);
        setShowNewSession(false);
        // Start the session in the background — poller picks up status changes
        markLoading(session.id);
        api().StartSession(session.id).catch((err) => {
          console.error("Failed to start session:", err);
        });
      } catch (err) {
        console.error("Failed to create session:", err);
      }
    },
    [addSession, markLoading]
  );

  return (
    <div style={{ display: "flex", height: "100vh", flexDirection: "column" }}>
      <div style={{ display: "flex", flex: 1, overflow: "hidden" }}>
        {sidebarVisible && (
          <Sidebar onNewSession={() => setShowNewSession(true)} />
        )}
        <div style={{ flex: 1, display: "flex", flexDirection: "column" }}>
          <TabBar />
          <PaneManager wsPort={wsPort} />
        </div>
      </div>
      <StatusBar />

      {showNewSession && config && (
        <NewSessionDialog
          onSubmit={handleCreateSession}
          onCancel={() => setShowNewSession(false)}
          profiles={config.Profiles}
          defaultWorkDir={config.DefaultWorkDir}
        />
      )}
    </div>
  );
}
