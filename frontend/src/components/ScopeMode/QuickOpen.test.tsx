import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QuickOpen } from "./QuickOpen";
import { useSessionStore } from "../../store/sessionStore";
import { mockSessionAPI } from "../../test/setup";

beforeEach(() => {
  useSessionStore.setState({
    fileList: [
      "main.go",
      "app/bindings.go",
      "app/indexer.go",
      "frontend/src/App.tsx",
      "frontend/src/lib/wails.ts",
      "README.md",
    ],
    quickOpenVisible: false,
    openEditorFiles: [],
    activeEditorFile: null,
  });
  vi.clearAllMocks();
  mockSessionAPI.ReadFile.mockResolvedValue("file contents");
});

describe("QuickOpen", () => {
  it("does not render when not visible", () => {
    const { container } = render(<QuickOpen sessionId="s1" />);
    expect(container.innerHTML).toBe("");
  });

  it("renders overlay when visible", () => {
    useSessionStore.setState({ quickOpenVisible: true });
    render(<QuickOpen sessionId="s1" />);

    expect(screen.getByPlaceholderText("Search files by name...")).toBeInTheDocument();
    expect(screen.getByText("↑↓ navigate")).toBeInTheDocument();
  });

  it("shows all files when query is empty", () => {
    useSessionStore.setState({ quickOpenVisible: true });
    render(<QuickOpen sessionId="s1" />);

    expect(screen.getByText("main.go")).toBeInTheDocument();
    expect(screen.getByText("README.md")).toBeInTheDocument();
  });

  it("filters files as user types", async () => {
    useSessionStore.setState({ quickOpenVisible: true });
    render(<QuickOpen sessionId="s1" />);

    const input = screen.getByPlaceholderText("Search files by name...");
    await userEvent.type(input, "bind");

    // The filename is split across highlighted spans, so check the directory path instead
    expect(screen.getByText("app/")).toBeInTheDocument();
    expect(screen.queryByText("README.md")).not.toBeInTheDocument();
  });

  it("shows 'No matching files' for unmatched query", async () => {
    useSessionStore.setState({ quickOpenVisible: true });
    render(<QuickOpen sessionId="s1" />);

    const input = screen.getByPlaceholderText("Search files by name...");
    await userEvent.type(input, "zzzznotafile");

    expect(screen.getByText("No matching files")).toBeInTheDocument();
  });

  it("closes on Escape", async () => {
    useSessionStore.setState({ quickOpenVisible: true });
    render(<QuickOpen sessionId="s1" />);

    const input = screen.getByPlaceholderText("Search files by name...");
    fireEvent.keyDown(input, { key: "Escape" });

    expect(useSessionStore.getState().quickOpenVisible).toBe(false);
  });

  it("closes on backdrop click", () => {
    useSessionStore.setState({ quickOpenVisible: true });
    const { container } = render(<QuickOpen sessionId="s1" />);

    // Click the backdrop (outermost div)
    const backdrop = container.firstChild as HTMLElement;
    fireEvent.click(backdrop);

    expect(useSessionStore.getState().quickOpenVisible).toBe(false);
  });

  it("opens file on Enter and calls ReadFile", async () => {
    useSessionStore.setState({ quickOpenVisible: true });
    render(<QuickOpen sessionId="s1" />);

    const input = screen.getByPlaceholderText("Search files by name...");
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => {
      expect(mockSessionAPI.ReadFile).toHaveBeenCalledWith("s1", "main.go");
    });
  });

  it("navigates results with arrow keys", async () => {
    useSessionStore.setState({ quickOpenVisible: true });
    render(<QuickOpen sessionId="s1" />);

    const input = screen.getByPlaceholderText("Search files by name...");

    // Arrow down to second item
    fireEvent.keyDown(input, { key: "ArrowDown" });
    // Press Enter on second item
    fireEvent.keyDown(input, { key: "Enter" });

    await waitFor(() => {
      // Second file in the list should be opened
      expect(mockSessionAPI.ReadFile).toHaveBeenCalled();
      const calledPath = mockSessionAPI.ReadFile.mock.calls[0][1];
      expect(calledPath).not.toBe("main.go"); // not the first item
    });
  });
});
