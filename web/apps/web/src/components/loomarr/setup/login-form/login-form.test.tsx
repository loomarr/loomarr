import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LoginForm } from "./login-form";

describe("LoginForm", () => {
  it("submits the entered credentials", async () => {
    const onSubmit = vi.fn();
    render(<LoginForm onSubmit={onSubmit} />);

    await userEvent.type(screen.getByLabelText("Username"), "ada");
    await userEvent.type(screen.getByLabelText("Password"), "hunter2!");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ username: "ada", password: "hunter2!" }));
  });

  it("blocks submit and shows a field error when empty", async () => {
    const onSubmit = vi.fn();
    render(<LoginForm onSubmit={onSubmit} />);

    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));

    expect(onSubmit).not.toHaveBeenCalled();
    expect(await screen.findByText(/enter your username/i)).toBeInTheDocument();
  });

  it("renders a block-level error from the caller", () => {
    render(<LoginForm onSubmit={vi.fn()} error={new Error("Invalid credentials")} />);
    expect(screen.getByRole("alert")).toHaveTextContent(/invalid credentials/i);
  });

  it("disables the submit button while a sign-in is pending", () => {
    render(<LoginForm onSubmit={vi.fn()} isPending />);
    expect(screen.getByRole("button", { name: /signing in/i })).toBeDisabled();
  });
});
