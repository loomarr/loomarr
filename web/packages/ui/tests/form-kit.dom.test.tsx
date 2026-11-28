// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { RequestChannel } from "../index";

const doc = document;

const mount = async (node: ReactNode) => {
  await act(async () => {
    renderUi(node);
  });
};
afterEach(cleanup);

const typeInto = async (input: HTMLTextAreaElement, text: string) => {
  const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
  await act(async () => {
    setter?.call(input, text);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
};
const press = async (name: string) => {
  const found = Array.from(doc.body.querySelectorAll<HTMLElement>('[role="button"]')).find(
    (node) => node.textContent?.trim() === name,
  );
  if (!found) throw new Error(`missing button: ${name}`);
  await act(async () => {
    found.click();
  });
};

const request = (onSubmitBrief: () => void) => (
  <RequestChannel
    brief={{ kind: "idle" }}
    ideas={[]}
    ideasStatus="ready"
    nowMs={0}
    onBack={vi.fn()}
    onHideIdea={vi.fn()}
    onRequestIdea={vi.fn()}
    onRetryIdeas={vi.fn()}
    onSubmitBrief={onSubmitBrief}
    onUndoHide={vi.fn()}
    viewer="member"
  />
);

describe("a shared form (useUiForm)", () => {
  it("says what the schema refuses on submit, then lets the viewer fix it and send", async () => {
    const onSubmitBrief = vi.fn();
    await mount(request(onSubmitBrief));

    await press("Suggest titles");
    expect(onSubmitBrief).not.toHaveBeenCalled();
    expect(doc.body.textContent).toContain("Describe the channel you want (a sentence is plenty).");

    const input = doc.body.querySelector("textarea");
    if (!input) throw new Error("no textarea");
    await typeInto(input, "Saturday-morning cartoons, bright and silly");
    expect(doc.body.textContent).not.toContain("a sentence is plenty");
    await press("Suggest titles");
    expect(onSubmitBrief).toHaveBeenCalledTimes(1);
  });
});
