// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ClientShell, type ClientShellProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type ClickableNode = { click(): void; textContent?: string | null };
type QueryRoot = { querySelectorAll(selector: string): ArrayLike<ClickableNode> };
type Container = Parameters<typeof createRoot>[0] &
  QueryRoot & { remove(): void; textContent?: string | null };
const testDocument = (
  globalThis as unknown as {
    document: { body: { append(node: unknown): void }; createElement(tagName: string): Container };
  }
).document;
const buttonNamed = (container: QueryRoot, name: string) => {
  const button = Array.from(container.querySelectorAll('[role="button"], button')).find(
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

const mount = async () => {
  const container = testDocument.createElement("div");
  testDocument.body.append(container);
  const root = createRoot(container);
  const render = (active: ClientShellProps["active"]) =>
    act(async () => {
      root.render(
        <LoomarrProvider>
          <ClientShell active={active} density="touch" onDisconnect={vi.fn()} onNavigate={vi.fn()}>
            {active === "requests" ? () => <Requests /> : <>Tonight&apos;s guide</>}
          </ClientShell>
        </LoomarrProvider>,
      );
    });
  await render("requests");
  return { container, render, unmount: () => act(async () => root.unmount()) };
};

describe("client shell", () => {
  afterEach(() => {
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
});
