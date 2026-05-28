import { useState, useEffect, useCallback } from "react";
import { Sidebar } from "./components/Sidebar/Sidebar";
import { TabBar } from "./components/TabBar/TabBar";
import { PaneManager } from "./components/Terminal/PaneManager";
import { ScopeLayout } from "./components/ScopeMode/ScopeLayout";
import { StatusBar } from "./components/StatusBar";
import { NewSessionDialog } from "./components/Dialogs/NewSessionDialog";
import { useSessionPoller } from "./hooks/useSessionPoller";
import { useHotkeys } from "./hooks/useHotkeys";
import { useSessionStore } from "./store/sessionStore";
import { api } from "./lib/wails";
import type { AppConfig, CreateOptions } from "./lib/wails";
import { EventsOn } from "../wailsjs/runtime/runtime";
import {
  SubmoduleWarningsDialog,
  SubmoduleWarningsPayload,
} from "./components/Dialogs/SubmoduleWarningsDialog";
import { RetrySubmoduleSetup } from "../wailsjs/go/app/SessionAPI";

export default function App() {
  const [wsPort, setWsPort] = useState(0);
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [showNewSession, setShowNewSession] = useState(false);
  const [submoduleWarnings, setSubmoduleWarnings] =
    useState<SubmoduleWarningsPayload | null>(null);
  const [submoduleRetrying, setSubmoduleRetrying] = useState(false);
  const sidebarVisible = useSessionStore((s) => s.sidebarVisible);
  const scopeMode = useSessionStore((s) => s.scopeMode);
  const setSessions = useSessionStore((s) => s.setSessions);
  const addSession = useSessionStore((s) => s.addSession);
  const markLoading = useSessionStore((s) => s.markLoading);

  useEffect(() => {
    const init = async () => {
      try {
        const [port, cfg, sessions, hosts] = await Promise.all([
          api().GetWebSocketPort(),
          api().GetConfig(),
          api().LoadSessions(),
          api().GetHosts(),
        ]);
        setWsPort(port);
        setConfig(cfg);
        setSessions(sessions);
        useSessionStore.getState().setHosts(hosts);
      } catch {
        // Wails not ready yet
      }
    };
    init();
  }, [setSessions]);

  useEffect(() => {
    const unlisten = EventsOn(
      "session:submodule-warnings",
      (payload: SubmoduleWarningsPayload) => {
        if (!payload || payload.failures.length === 0) {
          setSubmoduleWarnings(null);
          setSubmoduleRetrying(false);
        } else {
          setSubmoduleWarnings(payload);
          setSubmoduleRetrying(false);
        }
      },
    );
    return () => unlisten();
  }, []);

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
        setShowNewSession(false);
        // Mark loading BEFORE adding to the session list so the first render
        // already shows the hourglass instead of a brief yellow "ready" LED.
        markLoading(session.id);
        addSession(session);
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
        {scopeMode.active ? (
          <ScopeLayout wsPort={wsPort} />
        ) : (
          <>
            {sidebarVisible && (
              <Sidebar onNewSession={() => setShowNewSession(true)} />
            )}
            <div style={{ flex: 1, display: "flex", flexDirection: "column" }}>
              <TabBar />
              <PaneManager wsPort={wsPort} />
            </div>
          </>
        )}
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

      {submoduleWarnings && (
        <SubmoduleWarningsDialog
          payload={submoduleWarnings}
          retrying={submoduleRetrying}
          onRetry={async (names) => {
            setSubmoduleRetrying(true);
            try {
              await RetrySubmoduleSetup(submoduleWarnings.sessionId, names);
              // The follow-up event will update state. If the event never arrives
              // (e.g., backend error), the spinner stays — that's acceptable for v1.
            } catch (e) {
              console.error("RetrySubmoduleSetup failed:", e);
              setSubmoduleRetrying(false);
            }
          }}
          onOpenTerminal={() => {
            // Navigate to the session's tab if one exists, otherwise just close.
            // We look up the tab by sessionId in the store and activate it.
            if (submoduleWarnings) {
              const { tabs, setActiveTab } = useSessionStore.getState();
              const tab = tabs.find(
                (t) => t.sessionId === submoduleWarnings.sessionId,
              );
              if (tab) {
                setActiveTab(tab.id);
              }
            }
            setSubmoduleWarnings(null);
          }}
          onDismiss={() => setSubmoduleWarnings(null)}
        />
      )}
    </div>
  );
}
