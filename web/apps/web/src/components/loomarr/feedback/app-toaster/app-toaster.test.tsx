import { act, cleanup, render, screen } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppToaster } from "./app-toaster";

// The phone layout is everything narrower than 48rem: a `(min-width: 48rem)` query that misses.
const viewport = (phone: boolean) =>
  vi.stubGlobal("matchMedia", (query: string) => ({
    addEventListener: vi.fn(),
    matches: !phone,
    media: query,
    removeEventListener: vi.fn(),
  }));

// Sonner draws its region once a toast exists, and says which edge it is on.
const raise = async () => {
  render(<AppToaster />);
  act(() => {
    toast.error("Couldn't save your changes");
  });
  const region = (await screen.findByText("Couldn't save your changes")).closest("[data-sonner-toaster]");
  if (!(region instanceof HTMLElement)) throw new Error("the toast has no region");
  return region;
};

describe("AppToaster", () => {
  afterEach(() => {
    act(() => {
      toast.dismiss();
    });
    cleanup();
    vi.unstubAllGlobals();
  });

  it("keeps toasts off the phone's bottom band, where Save changes sits (#3439)", async () => {
    viewport(true);

    const region = await raise();

    expect(region.dataset.yPosition).toBe("top");
    expect(region.dataset.xPosition).toBe("center");
    // Under the shell's 49px header row, at both of Sonner's widths.
    expect(region.style.getPropertyValue("--offset-top")).toBe("57px");
    expect(region.style.getPropertyValue("--mobile-offset-top")).toBe("57px");
  });

  it("draws toasts bottom-centre on a wide layout, above persistent bottom actions", async () => {
    viewport(false);

    const region = await raise();

    expect(region.dataset.yPosition).toBe("bottom");
    expect(region.dataset.xPosition).toBe("center");
    expect(region.style.getPropertyValue("--offset-bottom")).toBe("84px");
  });
});
