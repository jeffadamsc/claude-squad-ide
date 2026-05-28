import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { NewSessionDialog } from "./NewSessionDialog";
import { useSessionStore } from "../../store/sessionStore";

const defaultProps = {
  onSubmit: vi.fn(),
  onCancel: vi.fn(),
  profiles: [{ Name: "Claude", Program: "claude" }],
  defaultWorkDir: "/tmp",
};

beforeEach(() => {
  useSessionStore.setState({ hosts: [] });
  vi.clearAllMocks();
});

describe("NewSessionDialog", () => {
  describe("default behavior (submitting=false)", () => {
    it("shows 'Create' text on the submit button", () => {
      render(<NewSessionDialog {...defaultProps} />);
      expect(screen.getByRole("button", { name: "Create" })).toBeInTheDocument();
    });

    it("button is disabled when title is empty (canCreate=false)", () => {
      render(<NewSessionDialog {...defaultProps} />);
      const btn = screen.getByRole("button", { name: "Create" });
      expect(btn).toBeDisabled();
    });

    it("button is enabled when title is filled in (canCreate=true)", () => {
      render(<NewSessionDialog {...defaultProps} />);
      const titleInput = screen.getByPlaceholderText("fix-auth-bug");
      fireEvent.change(titleInput, { target: { value: "my-session" } });
      const btn = screen.getByRole("button", { name: "Create" });
      expect(btn).not.toBeDisabled();
    });
  });

  describe("submitting=true", () => {
    it("shows 'Creating...' text on the submit button", () => {
      render(<NewSessionDialog {...defaultProps} submitting />);
      expect(screen.getByRole("button", { name: "Creating..." })).toBeInTheDocument();
    });

    it("button is disabled when form is empty", () => {
      render(<NewSessionDialog {...defaultProps} submitting />);
      const btn = screen.getByRole("button", { name: "Creating..." });
      expect(btn).toBeDisabled();
    });

    it("button is disabled even when form is valid", () => {
      render(<NewSessionDialog {...defaultProps} submitting />);
      const titleInput = screen.getByPlaceholderText("fix-auth-bug");
      fireEvent.change(titleInput, { target: { value: "my-session" } });
      const btn = screen.getByRole("button", { name: "Creating..." });
      expect(btn).toBeDisabled();
    });

    it("does not call onSubmit when button is clicked while submitting", () => {
      render(<NewSessionDialog {...defaultProps} submitting />);
      const titleInput = screen.getByPlaceholderText("fix-auth-bug");
      fireEvent.change(titleInput, { target: { value: "my-session" } });
      const btn = screen.getByRole("button", { name: "Creating..." });
      fireEvent.click(btn);
      expect(defaultProps.onSubmit).not.toHaveBeenCalled();
    });
  });
});
