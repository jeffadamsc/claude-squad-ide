import { useEffect, useRef } from "react";
import { useSessionStore } from "../store/sessionStore";
import { api } from "../lib/wails";

export function useSessionPoller(intervalMs = 500) {
  const updateStatuses = useSessionStore((s) => s.updateStatuses);
  const intervalRef = useRef<number>();

  useEffect(() => {
    const poll = async () => {
      try {
        const statuses = await api().PollAllStatuses();
        updateStatuses(statuses);
      } catch {
        // Wails not ready yet
      }
    };

    poll();
    intervalRef.current = window.setInterval(poll, intervalMs);

    return () => {
      if (intervalRef.current) {
        window.clearInterval(intervalRef.current);
      }
    };
  }, [intervalMs, updateStatuses]);
}
