import { Allotment } from "allotment";
import "allotment/dist/style.css";
import { TerminalPane } from "./TerminalPane";

interface SplitContainerProps {
  sessionIds: string[];
  wsPort: number;
  direction: "horizontal" | "vertical";
  focusedIdx: number;
}

export function SplitContainer({
  sessionIds,
  wsPort,
  direction,
  focusedIdx,
}: SplitContainerProps) {
  return (
    <Allotment vertical={direction === "vertical"}>
      {sessionIds.map((id, idx) => (
        <Allotment.Pane key={id}>
          <TerminalPane
            sessionId={id}
            wsPort={wsPort}
            focused={idx === focusedIdx}
          />
        </Allotment.Pane>
      ))}
    </Allotment>
  );
}
