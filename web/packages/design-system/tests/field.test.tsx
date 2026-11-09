// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FormEvent } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Field, LoomarrProvider } from "../index";

afterEach(cleanup);

describe("Field", () => {
  it("focuses the input when its label is clicked", async () => {
    render(
      <LoomarrProvider theme="dark">
        <Field label="Server address" />
      </LoomarrProvider>,
    );

    await userEvent.click(screen.getByText("Server address"));

    expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Server address" }));
  });

  it("is a labelled text area when multiline, keeping the newlines typed into it", async () => {
    const onChangeText = vi.fn();
    render(
      <LoomarrProvider theme="dark">
        <Field label="Channel brief" multiline numberOfLines={3} onChangeText={onChangeText} />
      </LoomarrProvider>,
    );
    const brief = screen.getByRole("textbox", { name: "Channel brief" });

    await userEvent.type(brief, "Cartoons{Enter}Cereal ads");

    expect(brief.tagName).toBe("TEXTAREA");
    expect(brief.getAttribute("rows")).toBe("3");
    expect(onChangeText).toHaveBeenLastCalledWith("Cartoons\nCereal ads");
  });

  it("shows and hides a password without submitting its form", async () => {
    const onSubmit = vi.fn((event: FormEvent) => event.preventDefault());
    render(
      <LoomarrProvider theme="dark">
        <form onSubmit={onSubmit}>
          <Field id="password" label="Password" secureTextEntry />
        </form>
      </LoomarrProvider>,
    );
    const input = document.getElementById("password");
    expect(input?.getAttribute("type")).toBe("password");

    await userEvent.click(screen.getByRole("button", { name: "Show password" }));
    expect(screen.getByRole("textbox", { name: "Password" })).toBe(input);
    const hide = screen.getByRole("button", { name: "Hide password" });
    expect(hide.getAttribute("aria-pressed")).toBe("true");

    await userEvent.click(hide);
    expect(input?.getAttribute("type")).toBe("password");
    expect(onSubmit).not.toHaveBeenCalled();
  });

  // React Native Web's TextInput stops every keydown at the input, so a dialog listening on the
  // document never heard Escape from inside a Field and stayed open.
  it("lets Escape reach a dialog listening on the document", async () => {
    const onEscape = vi.fn();
    const listener = (event: KeyboardEvent) => {
      if (event.key === "Escape") onEscape();
    };
    document.addEventListener("keydown", listener);
    render(
      <LoomarrProvider theme="dark">
        <Field label="Server address" />
      </LoomarrProvider>,
    );

    await userEvent.type(screen.getByRole("textbox", { name: "Server address" }), "a{Escape}");
    document.removeEventListener("keydown", listener);

    expect(onEscape).toHaveBeenCalledOnce();
  });

  // The text a screen reader joins into the input's description: what its aria-describedby ids resolve to.
  const describedBy = (input: HTMLElement) =>
    (input.getAttribute("aria-describedby") ?? "")
      .split(" ")
      .filter(Boolean)
      .map((id) => document.getElementById(id)?.textContent)
      .join(" ");

  it("describes the input by its help line", () => {
    render(
      <LoomarrProvider theme="dark">
        <Field description="Use the address your server shows." label="Server address" />
      </LoomarrProvider>,
    );

    expect(describedBy(screen.getByRole("textbox", { name: "Server address" }))).toBe(
      "Use the address your server shows.",
    );
  });

  it("describes the input by its error line instead of the help line", () => {
    render(
      <LoomarrProvider theme="dark">
        <Field
          description="Use the address your server shows."
          error="Enter an HTTPS address."
          label="Server address"
        />
      </LoomarrProvider>,
    );

    const input = screen.getByRole("textbox", { name: "Server address" });
    expect(describedBy(input)).toBe("Enter an HTTPS address.");
    expect(input.getAttribute("aria-invalid")).toBe("true");
  });

  it("keeps a description the caller points at, ahead of its own line", () => {
    const callerProps = { "aria-describedby": "elsewhere" };
    render(
      <LoomarrProvider theme="dark">
        <p id="elsewhere">Shown by the form.</p>
        <Field {...callerProps} description="Field help." label="Server address" />
      </LoomarrProvider>,
    );

    expect(describedBy(screen.getByRole("textbox", { name: "Server address" }))).toBe(
      "Shown by the form. Field help.",
    );
  });

  it("keeps two fields' descriptions apart and leaves a bare field undescribed", () => {
    render(
      <LoomarrProvider theme="dark">
        <Field description="First help." label="First" />
        <Field description="Second help." label="Second" />
        <Field label="Bare" />
      </LoomarrProvider>,
    );

    expect(describedBy(screen.getByRole("textbox", { name: "First" }))).toBe("First help.");
    expect(describedBy(screen.getByRole("textbox", { name: "Second" }))).toBe("Second help.");
    expect(screen.getByRole("textbox", { name: "Bare" }).hasAttribute("aria-describedby")).toBe(false);
  });

  it("offers no reveal button on a plain field", () => {
    render(
      <LoomarrProvider theme="dark">
        <Field label="Server address" />
      </LoomarrProvider>,
    );

    expect(screen.queryByRole("button")).toBeNull();
  });
});
