import { useEffect } from "react";
import { useSessionStore } from "../store/sessionStore";
import { api } from "../lib/wails";

interface HotkeyActions {
  onNewSession: () => void;
  onDeleteSession: () => void;
  onPushSession: () => void;
  onTogglePauseResume: () => void;
  onQuit: () => void;
}

export function useHotkeys(actions: HotkeyActions) {
  const sessions = useSessionStore((s) => s.sessions);
  const selectedIdx = useSessionStore((s) => s.selectedSidebarIdx);
  const setSelectedIdx = useSessionStore((s) => s.setSelectedSidebarIdx);
  const toggleSidebar = useSessionStore((s) => s.toggleSidebar);
  const openTab = useSessionStore((s) => s.openTab);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (!(e.ctrlKey || e.metaKey) || !e.shiftKey) return;

      switch (e.key) {
        case "N":
          e.preventDefault();
          actions.onNewSession();
          break;
        case "J":
          e.preventDefault();
          setSelectedIdx(Math.min(selectedIdx + 1, sessions.length - 1));
          break;
        case "K":
          e.preventDefault();
          setSelectedIdx(Math.max(selectedIdx - 1, 0));
          break;
        case "Enter":
          e.preventDefault();
          if (sessions[selectedIdx]) {
            const session = sessions[selectedIdx];
            api().OpenSession(session.id).then((ptyId) => {
              openTab(session.id, ptyId);
            }).catch((err) => {
              console.error("Failed to open session:", err);
            });
          }
          break;
        case "D":
          e.preventDefault();
          actions.onDeleteSession();
          break;
        case "P":
          e.preventDefault();
          actions.onPushSession();
          break;
        case "R":
          e.preventDefault();
          actions.onTogglePauseResume();
          break;
        case "B":
          e.preventDefault();
          toggleSidebar();
          break;
        case "Q":
          e.preventDefault();
          actions.onQuit();
          break;
      }
    };

    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [actions, sessions, selectedIdx, setSelectedIdx, toggleSidebar, openTab]);
}
