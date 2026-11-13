import { screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { PausedCover } from "./paused-cover";

const draw = (isAdmin: boolean) =>
  render(
    <PausedCover
      channelLabel="CH 7 · Cartoons"
      channelName="Cartoons"
      density="pointer"
      isAdmin={isAdmin}
      onLeave={vi.fn()}
    />,
  );

describe("PausedCover", () => {
  it("names the channel as off air", () => {
    draw(false);
    expect(screen.getByText("Cartoons is off air")).toBeTruthy();
  });

  it("tells an admin the way back, and a member only that there is nothing to watch", () => {
    const admin = draw(true);
    expect(screen.getByText(/Resume it under Danger zone/)).toBeTruthy();
    admin.unmount();

    draw(false);
    expect(screen.getByText("This channel is paused, so there's nothing to watch.")).toBeTruthy();
    expect(screen.queryByText(/Danger zone/)).toBeNull();
  });
});
