// @vitest-environment jsdom

import { cleanup, renderUi } from "@loomarr/test-support";
import { screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";
import { toastIgnored, toastKept } from "./needs-toasts";

vi.mock("sonner", () => ({ toast: Object.assign(vi.fn(), { error: vi.fn() }) }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

// The toast's button as it holds its label.
const drawButton = () => {
  const options = vi.mocked(toast).mock.calls[0]?.[1] as { action: { label: ReactNode } };
  renderUi(<button type="button">{options.action.label}</button>);
};

const undo = () => Promise.resolve();

describe("the Needs you toast's Undo", () => {
  it.each([
    { say: toastIgnored, named: "Undo ignoring a.mkv" },
    { say: toastKept, named: "Undo keeping a.mkv" },
  ])("shows Undo and is named $named", ({ say, named }) => {
    say("a.mkv", undo);
    drawButton();

    // The shown word is hidden from the name, so the file is not announced after a second "Undo".
    expect(screen.getByRole("button", { name: named })).toBeDefined();
    expect(screen.getByText("Undo").getAttribute("aria-hidden")).toBe("true");
  });
});
