import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { StoppedCover } from "./stopped-cover";

const draw = (failureCause: Parameters<typeof StoppedCover>[0]["failureCause"]) => {
  const onLeave = vi.fn();
  const onRetry = vi.fn();
  render(
    <StoppedCover
      channelLabel="CH 7 · Cartoons"
      density="pointer"
      failureCause={failureCause}
      onLeave={onLeave}
      onRetry={onRetry}
    />,
  );
  return { onLeave, onRetry };
};

describe("StoppedCover", () => {
  it.each([
    ["channel", "This channel couldn't start", /couldn't start the video for this channel/],
    ["server", "Loomarr's server isn't answering", /lost touch with Loomarr/],
    ["stream", "The stream stopped.", /Trying again usually brings it back/],
  ] as const)("words a %s failure in its own plain words", (cause, title, message) => {
    draw(cause);
    expect(screen.getByText(title)).toBeTruthy();
    expect(screen.getByText(message)).toBeTruthy();
  });

  it("reads a failure with no cause as a stream that stopped, with no second line", () => {
    draw(undefined);
    expect(screen.getByText("The stream stopped.")).toBeTruthy();
    expect(screen.queryByText(/Trying again usually/)).toBeNull();
  });

  it("offers Retry and the way out", async () => {
    const { onLeave, onRetry } = draw("stream");

    await userEvent.click(screen.getByRole("button", { name: /retry|try again/i }));
    expect(onRetry).toHaveBeenCalledTimes(1);

    await userEvent.click(screen.getByRole("button", { name: /leave|close|back/i }));
    expect(onLeave).toHaveBeenCalledTimes(1);
  });
});
