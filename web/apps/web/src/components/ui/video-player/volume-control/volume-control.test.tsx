import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { VolumeControl } from "./volume-control";

describe("VolumeControl", () => {
  it("toggles mute via the button, naming itself for the current state", async () => {
    const onMutedChange = vi.fn();
    render(
      <VolumeControl volume={0.7} muted={false} onVolumeChange={() => {}} onMutedChange={onMutedChange} />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Mute" }));
    expect(onMutedChange).toHaveBeenCalledWith(true);
  });

  it("names the button Unmute when muted", () => {
    render(<VolumeControl volume={0.7} muted onVolumeChange={() => {}} onMutedChange={() => {}} />);
    expect(screen.getByRole("button", { name: "Unmute" })).toBeInTheDocument();
  });

  it("exposes an accessible volume slider", () => {
    render(<VolumeControl volume={0.5} muted={false} onVolumeChange={() => {}} onMutedChange={() => {}} />);
    expect(screen.getByRole("slider", { name: "Volume" })).toBeInTheDocument();
  });
});
