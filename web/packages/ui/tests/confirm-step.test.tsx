// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ConfirmStep } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Button = { click(): void; getAttribute(name: string): string | null; textContent: string | null };
type Doc = {
  body: {
    append(node: unknown): void;
    querySelectorAll(s: string): Button[];
    replaceChildren(): void;
    textContent: string | null;
  };
  createElement(tag: string): Parameters<typeof createRoot>[0];
};
const doc = (globalThis as unknown as { document: Doc }).document;

const mount = async (node: ReactNode) => {
  const container = doc.createElement("div");
  doc.body.append(container);
  const root = createRoot(container);
  await act(async () => {
    root.render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);
  });
};
const button = (name: string) => {
  const found = [...doc.body.querySelectorAll('[role="button"], button')].find((b) => b.textContent === name);
  if (!found) throw new Error(`no ${name} button`);
  return found;
};
const press = async (name: string) => {
  await act(async () => button(name).click());
};

afterEach(() => {
  doc.body.replaceChildren();
});

describe("ConfirmStep", () => {
  it("states the consequence and confirms under the action's own name", async () => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    await mount(
      <ConfirmStep
        confirmLabel="Remove device"
        message="Remove Bedroom TV? It has to be paired again."
        onCancel={onCancel}
        onConfirm={onConfirm}
      />,
    );

    expect(doc.body.textContent).toContain("Remove Bedroom TV? It has to be paired again.");
    await press("Remove device");
    expect(onConfirm).toHaveBeenCalledOnce();
    expect(onCancel).not.toHaveBeenCalled();
  });

  it("backs out without confirming", async () => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    await mount(
      <ConfirmStep confirmLabel="Sign out" message="Sign out?" onCancel={onCancel} onConfirm={onConfirm} />,
    );

    await press("Cancel");
    expect(onCancel).toHaveBeenCalledOnce();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("holds the confirm while the action is already running", async () => {
    const onConfirm = vi.fn();
    await mount(
      <ConfirmStep
        confirmLabel="Sign out"
        disabled
        message="Sign out?"
        onCancel={vi.fn()}
        onConfirm={onConfirm}
      />,
    );

    await press("Sign out");
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
