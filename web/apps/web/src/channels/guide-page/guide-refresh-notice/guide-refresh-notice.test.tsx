import { LoomarrProvider } from "@loomarr/design-system";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { GuideRefreshNotice } from "./guide-refresh-notice";

const anchorMs = Date.UTC(2026, 6, 25, 20, 0, 0);
const base = { anchorMs, dayOffset: 0, from: anchorMs, to: anchorMs + 4 * 3_600_000, timeZone: "UTC" };

const renderNotice = (props: Partial<Parameters<typeof GuideRefreshNotice>[0]> = {}) => {
  const onRetry = vi.fn();
  render(
    <LoomarrProvider theme="dark">
      <GuideRefreshNotice {...base} isFetching={false} onRetry={onRetry} {...props} />
    </LoomarrProvider>,
  );
  return onRetry;
};

describe("GuideRefreshNotice", () => {
  it("says the Guide is from a moment ago when no earlier window is shown", () => {
    renderNotice();
    expect(screen.getByText("Couldn't refresh the Guide")).toBeTruthy();
    expect(screen.getByText("Showing what Loomarr had a moment ago.")).toBeTruthy();
  });

  it("names the window that failed and the one still on screen", () => {
    renderNotice({ showing: { fromMs: anchorMs - 3_600_000, toMs: anchorMs + 3_600_000 } });
    expect(screen.getByText(/didn't load\. Showing .*the last one that did\./)).toBeTruthy();
  });

  it("retries on press, and ignores a press while a retry runs", async () => {
    const onRetry = renderNotice();
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("reads Trying again while fetching and does not retry again", async () => {
    const onRetry = renderNotice({ isFetching: true });
    await userEvent.click(screen.getByRole("button", { name: "Trying again…" }));
    expect(onRetry).not.toHaveBeenCalled();
  });
});
