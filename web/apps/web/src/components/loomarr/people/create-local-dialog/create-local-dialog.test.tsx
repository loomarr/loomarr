import { ApiError } from "@loomarr/api";
import type { CreateLocalUserInputBody } from "@loomarr/api/models/createLocalUserInputBody";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CreateLocalDialog } from "./create-local-dialog";

describe("CreateLocalDialog", () => {
  it("creates a trimmed local member through the focused dialog", async () => {
    const onCreate = vi.fn<(input: CreateLocalUserInputBody) => Promise<void>>().mockResolvedValue();
    render(<CreateLocalDialog onCreate={onCreate} />);

    expect(screen.queryByRole("dialog", { name: "Add local account" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Add local account" }));
    expect(screen.getByText(/non-reversible Argon2id verifier/i)).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("Username"), "  newcomer  ");
    await userEvent.type(screen.getByLabelText("Password"), "a-good-password");
    expect(screen.getByRole("combobox", { name: "Role" })).toHaveTextContent("Member");
    await userEvent.click(screen.getByRole("button", { name: "Create account" }));

    expect(onCreate).toHaveBeenCalledWith({
      username: "newcomer",
      password: "a-good-password",
      role: "member",
    });
  });

  it("rejects an empty username and a short password before creating", async () => {
    const onCreate = vi.fn<(input: CreateLocalUserInputBody) => Promise<void>>().mockResolvedValue();
    render(<CreateLocalDialog onCreate={onCreate} />);
    await userEvent.click(screen.getByRole("button", { name: "Add local account" }));

    await userEvent.click(screen.getByRole("button", { name: "Create account" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Pick a username.");
    expect(onCreate).not.toHaveBeenCalled();

    await userEvent.type(screen.getByLabelText("Username"), "newcomer");
    await userEvent.type(screen.getByLabelText("Password"), "short");
    await userEvent.click(screen.getByRole("button", { name: "Create account" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Use at least 8 characters.");
    expect(onCreate).not.toHaveBeenCalled();
  });

  it("shows the server problem and disables submission while creation is pending", async () => {
    const onCreate = vi.fn<(input: CreateLocalUserInputBody) => Promise<void>>().mockResolvedValue();
    const error = new ApiError(409, {
      title: "Username already exists",
      detail: "Choose another username.",
      status: 409,
    });

    render(<CreateLocalDialog defaultOpen error={error} creating onCreate={onCreate} />);

    expect(screen.getByRole("alert")).toHaveTextContent("Choose another username.");
    expect(screen.getByRole("button", { name: "Creating…" })).toBeDisabled();
    expect(screen.getByLabelText("Username")).toBeDisabled();
    expect(screen.getByLabelText("Password")).toBeDisabled();
    expect(screen.getByRole("combobox", { name: "Role" })).toBeDisabled();
  });

  it("creates an explicitly selected admin, closes, clears credentials, and returns focus", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn<(input: CreateLocalUserInputBody) => Promise<void>>().mockResolvedValue();
    render(<CreateLocalDialog onCreate={onCreate} />);

    const trigger = screen.getByRole("button", { name: "Add local account" });
    await user.click(trigger);
    await user.type(screen.getByLabelText("Username"), "operator");
    await user.type(screen.getByLabelText("Password"), "admin-password");
    await user.click(screen.getByRole("combobox", { name: "Role" }));
    await screen.findByRole("option", { name: "Admin" });
    await user.keyboard("a{Enter}");
    await user.click(screen.getByRole("button", { name: "Create account" }));

    expect(onCreate).toHaveBeenCalledWith({
      username: "operator",
      password: "admin-password",
      role: "admin",
    });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(trigger).toHaveFocus();

    await user.click(trigger);
    expect(screen.getByLabelText("Username")).toHaveValue("");
    expect(screen.getByLabelText("Password")).toHaveValue("");
    expect(screen.getByRole("combobox", { name: "Role" })).toHaveTextContent("Member");
  });

  it("clears the password and returns focus after Cancel or Escape", async () => {
    const user = userEvent.setup();
    render(<CreateLocalDialog onCreate={() => {}} />);
    const trigger = screen.getByRole("button", { name: "Add local account" });

    await user.click(trigger);
    await user.type(screen.getByLabelText("Password"), "never-retain-this");
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(trigger).toHaveFocus();
    await user.click(trigger);
    expect(screen.getByLabelText("Password")).toHaveValue("");

    await user.type(screen.getByLabelText("Password"), "or-this-one");
    await user.keyboard("{Escape}");
    expect(trigger).toHaveFocus();
    await user.click(trigger);
    expect(screen.getByLabelText("Password")).toHaveValue("");
  });

  it("keeps correctable values in the dialog when creation fails", async () => {
    const onCreate = vi.fn<(input: CreateLocalUserInputBody) => Promise<void>>().mockRejectedValue(
      new ApiError(409, {
        title: "Username already exists",
        detail: "Choose another username.",
        status: 409,
      }),
    );
    render(<CreateLocalDialog onCreate={onCreate} />);

    await userEvent.click(screen.getByRole("button", { name: "Add local account" }));
    await userEvent.type(screen.getByLabelText("Username"), "duplicate");
    await userEvent.type(screen.getByLabelText("Password"), "correctable-password");
    await userEvent.click(screen.getByRole("button", { name: "Create account" }));

    await waitFor(() => expect(onCreate).toHaveBeenCalledOnce());
    expect(screen.getByRole("dialog", { name: "Add local account" })).toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toHaveValue("duplicate");
    expect(screen.getByLabelText("Password")).toHaveValue("correctable-password");
  });
});
