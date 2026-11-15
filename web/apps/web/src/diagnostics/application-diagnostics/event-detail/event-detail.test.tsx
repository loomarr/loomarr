import type { EventView } from "@loomarr/api/models/eventView";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { EventDetail } from "./event-detail";

const event = (over: Partial<EventView> = {}): EventView => ({
  id: "event-1",
  occurredAt: 1_780_000_000_000,
  receivedAt: 1_780_000_000_100,
  level: "error",
  source: "web",
  subsystem: "player",
  event: "player.media_error",
  message: "Playback failed",
  attributes: { transport: "hls_js" },
  ...over,
});

describe("EventDetail", () => {
  it("heads the detail with the level, source, message and subsystem", () => {
    render(<EventDetail event={event()} />);
    expect(screen.getByText(/Error · Web player/)).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Playback failed" })).toBeInTheDocument();
    expect(screen.getByText(/· player$/)).toBeInTheDocument();
  });

  it("falls back to the event name when there is no message", () => {
    render(<EventDetail event={event({ message: undefined, event: "playout.transition_late" })} />);
    expect(screen.getByRole("heading", { name: "Playout Transition Late" })).toBeInTheDocument();
  });

  it("copies the complete event as JSON", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    render(<EventDetail event={event()} />);
    await userEvent.click(screen.getByRole("button", { name: "Copy log details" }));
    expect(writeText).toHaveBeenCalledWith(JSON.stringify(event(), null, 2));
    expect(await screen.findByRole("button", { name: "Copied log details" })).toBeInTheDocument();
  });

  it("opens the related process, Channel and Job only when it has both the id and the handler", async () => {
    const onOpenProcess = vi.fn();
    const onOpenRelated = vi.fn();
    render(
      <EventDetail
        event={event({ processRunId: "process-1", channelId: "channel-1", jobId: "job-1" })}
        onOpenProcess={onOpenProcess}
        onOpenRelated={onOpenRelated}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Open process output" }));
    await userEvent.click(screen.getByRole("button", { name: "Open related Channel" }));
    await userEvent.click(screen.getByRole("button", { name: "Open related Job" }));
    expect(onOpenProcess).toHaveBeenCalledWith("process-1");
    expect(onOpenRelated).toHaveBeenCalledWith("channel", "channel-1");
    expect(onOpenRelated).toHaveBeenCalledWith("job", "job-1");
  });

  it("offers no related actions without handlers", () => {
    render(<EventDetail event={event({ processRunId: "process-1", channelId: "channel-1" })} />);
    expect(screen.queryByRole("button", { name: /^Open/ })).not.toBeInTheDocument();
  });

  it("lists the correlations it has and the structured attributes", async () => {
    render(<EventDetail event={event({ requestId: "request-1", instanceId: "instance-1" })} />);
    await userEvent.click(screen.getByText("Technical details"));
    expect(screen.getByText("player.media_error")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Copy Request id/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Copy Instance/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Copy Job/ })).not.toBeInTheDocument();
    expect(screen.getByText(/"transport": "hls_js"/)).toBeInTheDocument();
  });

  it("says so when there are no structured attributes", async () => {
    render(<EventDetail event={event({ attributes: {} })} />);
    await userEvent.click(screen.getByText("Technical details"));
    expect(screen.getByText("No structured attributes.")).toBeInTheDocument();
  });
});
