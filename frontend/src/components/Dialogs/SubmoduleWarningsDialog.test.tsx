import { describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { SubmoduleWarningsDialog } from "./SubmoduleWarningsDialog";

const samplePayload = {
  sessionId: "vistar",
  worktreePath: "/Users/me/.claude-squad/worktrees/jadams/vistar_abc",
  failures: [
    { name: "verve-backend-infra", stage: "fetch", error: "command timed out" },
    { name: "verve-portal", stage: "verify", error: "directory empty" },
  ],
};

describe("SubmoduleWarningsDialog", () => {
  it("renders the list of failed submodules", () => {
    render(
      <SubmoduleWarningsDialog
        payload={samplePayload}
        retrying={false}
        onRetry={() => {}}
        onOpenTerminal={() => {}}
        onDismiss={() => {}}
      />,
    );
    expect(screen.getByText("verve-backend-infra")).toBeInTheDocument();
    expect(screen.getByText("verve-portal")).toBeInTheDocument();
    expect(screen.getByText(/fetch/)).toBeInTheDocument();
    expect(screen.getByText(/verify/)).toBeInTheDocument();
  });

  it("calls onRetry with the failed submodule names when Retry clicked", () => {
    const onRetry = vi.fn();
    render(
      <SubmoduleWarningsDialog
        payload={samplePayload}
        retrying={false}
        onRetry={onRetry}
        onOpenTerminal={() => {}}
        onDismiss={() => {}}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /retry/i }));
    expect(onRetry).toHaveBeenCalledWith(["verve-backend-infra", "verve-portal"]);
  });

  it("disables Retry while retrying=true", () => {
    render(
      <SubmoduleWarningsDialog
        payload={samplePayload}
        retrying={true}
        onRetry={() => {}}
        onOpenTerminal={() => {}}
        onDismiss={() => {}}
      />,
    );
    const btn = screen.getByRole("button", { name: /retry/i });
    expect(btn).toBeDisabled();
  });

  it("calls onOpenTerminal when Open Terminal clicked", () => {
    const onOpenTerminal = vi.fn();
    render(
      <SubmoduleWarningsDialog
        payload={samplePayload}
        retrying={false}
        onRetry={() => {}}
        onOpenTerminal={onOpenTerminal}
        onDismiss={() => {}}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /open terminal/i }));
    expect(onOpenTerminal).toHaveBeenCalledOnce();
  });

  it("calls onDismiss when Dismiss clicked", () => {
    const onDismiss = vi.fn();
    render(
      <SubmoduleWarningsDialog
        payload={samplePayload}
        retrying={false}
        onRetry={() => {}}
        onOpenTerminal={() => {}}
        onDismiss={onDismiss}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /dismiss/i }));
    expect(onDismiss).toHaveBeenCalledOnce();
  });

  it("shows the manual-fix shell command including all failure names", () => {
    render(
      <SubmoduleWarningsDialog
        payload={samplePayload}
        retrying={false}
        onRetry={() => {}}
        onOpenTerminal={() => {}}
        onDismiss={() => {}}
      />,
    );
    const matches = screen.getAllByText(
      (_, el) =>
        !!el?.textContent?.includes("git submodule update --init -- verve-backend-infra verve-portal"),
    );
    expect(matches.length).toBeGreaterThan(0);
    const quotedPath = screen.getAllByText(
      (_, el) =>
        !!el?.textContent?.includes("cd '/Users/me/.claude-squad/worktrees/jadams/vistar_abc'"),
    );
    expect(quotedPath.length).toBeGreaterThan(0);
  });
});
