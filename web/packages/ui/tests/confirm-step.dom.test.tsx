// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ConfirmStep } from "../index";

const mount = async (node: ReactNode) => {
  await act(async () => {
    renderUi(node);
  });
};
const button = (name: string) => {
  const found = [...document.body.querySelectorAll<HTMLElement>('[role="button"], button')].find(
    (b) => b.textContent === name,
  );
  if (!found) throw new Error(`no ${name} button`);
  return found;
};
const press = async (name: string) => {
  await act(async () => button(name).click());
};

afterEach(cleanup);

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

    expect(document.body.textContent).toContain("Remove Bedroom TV? It has to be paired again.");
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
