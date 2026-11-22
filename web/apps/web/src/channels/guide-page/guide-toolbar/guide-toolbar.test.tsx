import { LoomarrProvider } from "@loomarr/design-system";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { GuideToolbar } from "./guide-toolbar";

const renderToolbar = (props: Partial<Parameters<typeof GuideToolbar>[0]> = {}) => {
  const handlers = {
    setDayOffset: vi.fn(),
    setHourShift: vi.fn(),
    setStartHour: vi.fn(),
    setViewOptionsOpen: vi.fn(),
    setWindowMinutes: vi.fn(),
  };
  const { unmount } = render(
    <LoomarrProvider theme="dark">
      <GuideToolbar
        anchorMs={Date.UTC(2026, 6, 25, 20, 0, 0)}
        atNow
        dayOffset={0}
        hasCustomView={false}
        startHour={null}
        timeZone="UTC"
        viewOptionsOpen={false}
        windowMinutes={240}
        {...handlers}
        {...props}
      />
    </LoomarrProvider>,
  );
  return { ...handlers, unmount };
};

describe("GuideToolbar", () => {
  it("picks a span", async () => {
    const { setWindowMinutes } = renderToolbar();
    await userEvent.click(screen.getByRole("button", { name: "Show 6 hours" }));
    expect(setWindowMinutes).toHaveBeenCalledWith(360);
  });

  it("steps the window by an hour either way", async () => {
    const { setHourShift } = renderToolbar();
    await userEvent.click(screen.getByRole("button", { name: "Forward an hour" }));
    await userEvent.click(screen.getByRole("button", { name: "Back an hour" }));
    expect(setHourShift.mock.calls.map(([update]) => update(0))).toEqual([1, -1]);
  });

  it("returns to now: no shift, today, no pinned start", async () => {
    const { setDayOffset, setHourShift, setStartHour } = renderToolbar({ atNow: false, dayOffset: 2 });
    await userEvent.click(screen.getByRole("button", { name: "NOW" }));
    expect(setHourShift.mock.calls[0]?.[0](5)).toBe(0);
    expect(setDayOffset).toHaveBeenCalledWith(0);
    expect(setStartHour).toHaveBeenCalledWith(null);
  });

  it("offers the way back from a shown day, as aired for the past and scheduled for the future", async () => {
    const past = renderToolbar({ dayOffset: -1 });
    await userEvent.click(screen.getByRole("button", { name: /AS AIRED · back to today/ }));
    expect(past.setDayOffset).toHaveBeenCalledWith(0);
  });

  it("announces a custom view on the options trigger and toggles the panel", async () => {
    const { setViewOptionsOpen } = renderToolbar({ hasCustomView: true });
    const trigger = screen.getByRole("button", { name: "View options, custom" });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    await userEvent.click(trigger);
    expect(setViewOptionsOpen.mock.calls[0]?.[0](false)).toBe(true);
  });

  it("shows the start-hour picker only while the options are open", () => {
    const closed = renderToolbar();
    expect(screen.queryByText("Start at")).toBeNull();
    closed.unmount();
    renderToolbar({ viewOptionsOpen: true });
    expect(screen.getByText("Start at")).toBeTruthy();
  });
});
