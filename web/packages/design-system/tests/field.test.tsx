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

  it("offers no reveal button on a plain field", () => {
    render(
      <LoomarrProvider theme="dark">
        <Field label="Server address" />
      </LoomarrProvider>,
    );

    expect(screen.queryByRole("button")).toBeNull();
  });
});
