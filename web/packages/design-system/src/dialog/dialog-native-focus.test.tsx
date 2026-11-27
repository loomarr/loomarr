// @vitest-environment jsdom
import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Action } from "../interaction";
import { LoomarrProvider } from "../provider";
// The native adapter by its file name: "./dialog" resolves to dialog.web.tsx under vitest. This test
// lives beside the adapter because a package's tests/ folder may only reach its entry points.
// @ts-expect-error TS5097: the extension is what picks dialog.tsx over dialog.web.tsx.
import { Dialog, DialogContent, DialogTitle } from "./dialog.tsx";

// A native Modal never blurs the control that opened it, so on TV the opener's amber focus border
// showed through the scrim (#1909). react-native-web's Modal traps focus and so blurs the opener by
// itself, which would hide the bug here; this stands in for the native one, which renders its
// children and leaves focus alone.
vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  const NativeLikeModal = ({ children, visible }: { children?: ReactNode; visible?: boolean }) =>
    visible ? children : null;
  return { ...actual, Modal: NativeLikeModal };
});

// The test names only the node surface it reads.
type ButtonNode = {
  focus(): void;
  style: { borderTopColor: string };
  textContent: string | null;
};
type Host = Parameters<typeof createRoot>[0] & { remove(): void };
const testDocument = (
  globalThis as unknown as {
    document: {
      activeElement: unknown;
      body: { append(node: unknown): void };
      createElement(tagName: string): Host;
      querySelectorAll(selector: string): ArrayLike<ButtonNode>;
    };
  }
).document;

// The opener drops its focus look while the dialog is open, and the dialog's own actions keep theirs;
// when the dialog closes the opener's ring is back.
describe("the native Dialog and its opener's focus (#1909)", () => {
  let host: Host;
  let root: Root;
  const amber = "rgb(255, 176, 32)";
  const byText = (text: string) =>
    Array.from(testDocument.querySelectorAll('[role="button"]')).find(
      (el) => el.textContent === text,
    ) as ButtonNode;

  beforeEach(() => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    host = testDocument.createElement("div");
    testDocument.body.append(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
  });

  const render = (open: boolean) =>
    act(() => {
      root.render(
        <LoomarrProvider>
          <Action density="tv" tone="secondary">
            Disconnect
          </Action>
          <Dialog open={open}>
            <DialogContent density="tv">
              <DialogTitle>Disconnect this TV?</DialogTitle>
              <Action density="tv" tone="secondary">
                Stay connected
              </Action>
            </DialogContent>
          </Dialog>
        </LoomarrProvider>,
      );
    });

  it("drops the opener's focus look behind the open dialog, and restores it on close", () => {
    render(false);
    act(() => byText("Disconnect").focus());
    expect(byText("Disconnect").style.borderTopColor).toBe(amber);

    render(true);
    // The native-like Modal left focus on the opener, as on TV.
    expect(testDocument.activeElement).toBe(byText("Disconnect"));
    expect(byText("Disconnect").style.borderTopColor).not.toBe(amber);
    act(() => byText("Stay connected").focus());
    expect(byText("Stay connected").style.borderTopColor).toBe(amber);

    render(false);
    act(() => byText("Disconnect").focus());
    expect(byText("Disconnect").style.borderTopColor).toBe(amber);
  });
});
