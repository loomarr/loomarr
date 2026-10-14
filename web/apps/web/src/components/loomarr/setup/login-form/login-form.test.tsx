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

  it("links local users to self-service password recovery", () => {
    render(<LoginForm onSubmit={vi.fn()} />);
    expect(screen.getByRole("link", { name: "Forgot password?" })).toHaveAttribute(
      "href",
      "/forgot-password",
    );
  });
});

describe("LoginForm — single sign-on (§11, V8)", () => {
  // ⚠ Offered ONLY when the route supplies a handler, which happens only when the server
  // reports a configured provider. The same posture as dev-login: a button pointing at a
  // route that is not mounted is worse than no button.
  it("offers no SSO button by default", () => {
    render(<LoginForm onSubmit={() => {}} />);
    expect(screen.queryByRole("button", { name: /sign in with your provider/i })).not.toBeInTheDocument();
  });

  it("offers SSO when the server says it is configured", async () => {
    const onSso = vi.fn();
    render(<LoginForm onSubmit={() => {}} onSso={onSso} />);

    await userEvent.click(screen.getByRole("button", { name: /sign in with your provider/i }));

    expect(onSso).toHaveBeenCalledOnce();
  });

  // ⚠ Loomarr's own sign-in must remain usable alongside SSO — an install whose provider is
  // down must not look like one nobody can enter (§11).
  it("keeps the password form when SSO is offered", () => {
    render(<LoginForm onSubmit={() => {}} onSso={() => {}} />);
    // Exact labels, not a regex: the SSO fallback copy also contains the word "password",
    // and a loose matcher found both.
    expect(screen.getByLabelText("Username")).toBeInTheDocument();
    expect(screen.getByLabelText("Password")).toBeInTheDocument();
  });

  // ⚠ THE REASON-CODE GATE. The callback sends a code, never a message, so no server text can
  // be reflected onto this page. The copy lives in the frontend.
  it("turns a reason code into copy", () => {
    render(<LoginForm onSubmit={() => {}} onSso={() => {}} ssoError="sso_no_account" />);
    expect(screen.getByRole("alert")).toHaveTextContent(/no account here for you yet/i);
  });

  // ⚠ An unrecognised code must NOT be echoed — echoing it is exactly the phishing surface
  // the code vocabulary exists to close.
  it("never echoes an unrecognised code", () => {
    const attack = "Your session expired, call 555-0100";
    render(<LoginForm onSubmit={() => {}} onSso={() => {}} ssoError={attack} />);

    const alert = screen.getByRole("alert");
    expect(alert).not.toHaveTextContent(attack);
    expect(alert).toHaveTextContent(/didn't work/i);
  });
});
