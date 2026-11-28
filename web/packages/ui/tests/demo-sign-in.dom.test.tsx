// @vitest-environment jsdom

import { act, cleanup, renderUi, screen } from "@loomarr/test-support";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DemoSignIn } from "../index";

afterEach(cleanup);

const button = (name: string) => {
  const found = screen.getAllByRole("button").find((node) => node.textContent?.trim() === name);
  if (!found) throw new Error(`missing button: ${name}`);
  return found;
};

describe("DemoSignIn", () => {
  it("says it's the demo and signs in as the server's account in one press", async () => {
    const onSignIn = vi.fn();
    const { container } = renderUi(<DemoSignIn onSignIn={onSignIn} username="guest" />);

    expect(container.querySelector('[role="region"]')?.getAttribute("aria-label")).toBe("Loomarr demo");
    expect(container.textContent).toContain("This is the Loomarr demo");
    await act(async () => button("Sign in as guest").click());

    expect(onSignIn).toHaveBeenCalledOnce();
  });

  it("holds the button while signing in", async () => {
    const onSignIn = vi.fn();
    renderUi(<DemoSignIn busy onSignIn={onSignIn} username="guest" />);
    const pending = button("Signing in…");

    await act(async () => pending.click());

    expect(pending.getAttribute("aria-disabled")).toBe("true");
    expect(onSignIn).not.toHaveBeenCalled();
  });
});
