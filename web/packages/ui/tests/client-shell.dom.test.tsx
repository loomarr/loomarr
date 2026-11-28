// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ClientShell, type ClientShellProps } from "../index";

const buttonNamed = (container: ParentNode, name: string) => {
  const button = Array.from(container.querySelectorAll<HTMLElement>('[role="button"], button')).find(
    (candidate) => candidate.textContent?.trim() === name,
  );
  if (!button) throw new Error(`missing button: ${name}`);
  return button;
};

let broken = true;
const Requests = () => {
  // What the phone's Needs you did with `"acquisitions": null` from the server.
  if (broken) throw new TypeError("Cannot read properties of null (reading 'length')");
  return <>Channel requests</>;
};

const mount = async (initial: ClientShellProps["active"] = "requests") => {
  const shell = (active: ClientShellProps["active"]) => (
    <ClientShell active={active} onNavigate={vi.fn()}>
      {active === "requests" ? () => <Requests /> : `Tonight's ${active}`}
    </ClientShell>
  );
  const { container, rerender, unmount } = renderUi(shell(initial));
  return {
    container,
    render: (active: ClientShellProps["active"]) => act(async () => rerender(shell(active))),
    unmount: () => act(async () => unmount()),
  };
};

describe("client shell", () => {
  afterEach(() => {
    cleanup();
    broken = true;
    vi.restoreAllMocks();
  });

  it("keeps one destination's render error in that destination, with the tab bar and a retry", async () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    const { container, unmount } = await mount();

    expect(container.textContent).toContain("Something went wrong");
    expect(container.querySelectorAll('[role="alert"]')).toHaveLength(1);
    // The tab bar survives, so the person can still go somewhere else.
    expect(container.querySelectorAll('[role="tab"]').length).toBeGreaterThan(0);

    broken = false;
    await act(async () => buttonNamed(container, "Try again").click());
    expect(container.textContent).toContain("Channel requests");
    expect(container.textContent).not.toContain("Something went wrong");
    await unmount();
  });

  it("clears a failed destination when the person changes tab", async () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    const { container, render, unmount } = await mount();
    expect(container.textContent).toContain("Something went wrong");

    await render("guide");
    expect(container.textContent).toContain("Tonight's guide");
    expect(container.textContent).not.toContain("Something went wrong");
    await unmount();
  });

  // The boundary resets on a prop, not a key: a key would remount the screen and its tab bar on
  // every tab change, dropping focus from the tab just pressed.
  it("keeps the tab bar mounted between healthy destinations", async () => {
    const { container, render, unmount } = await mount("guide");
    const tabs = Array.from(container.querySelectorAll('[role="tab"]'));

    await render("watching");
    expect(container.textContent).toContain("Tonight's watching");
    const after = Array.from(container.querySelectorAll('[role="tab"]'));
    expect(tabs.length).toBeGreaterThan(0);
    // The same nodes, not equal-looking new ones.
    expect(after.length === tabs.length && after.every((tab, index) => tab === tabs[index])).toBe(true);
    await unmount();
  });
});
