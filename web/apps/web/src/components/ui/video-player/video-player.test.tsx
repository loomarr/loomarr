import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { VideoPlayer } from "./video-player";

// The app's video surface (V39). Custom controls were a maintainer decision, which means the
// keyboard and ARIA behaviour native `<video controls>` would have given for free is this
// component's job — and therefore has to be pinned here.
//
// ⚠ jsdom implements no media pipeline: `play()` is not a function, `duration` is NaN, and nothing
// ever fires `timeupdate`. So these test the parts that are OURS — the semantics, the labels, the
// wiring — and leave "does it actually decode" to the browser, where it was verified live.
const SRC = "data:video/mp4;base64,AAAA";

describe("VideoPlayer", () => {
  it("renders a titled frame with the app's own controls", () => {
    render(<VideoPlayer src={SRC} title="Frosted Flakes" />);

    expect(screen.getByText("Frosted Flakes")).toBeInTheDocument();
    // Play/pause and mute, both named. Native controls would have supplied these.
    expect(screen.getByRole("button", { name: "Play" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Mute" })).toBeInTheDocument();
  });

  // A player under a heading that already names the thing does not repeat it — and the whole top
  // overlay goes with it rather than rendering an empty scrim over the frame.
  it("renders no title overlay without a title", () => {
    const { container } = render(<VideoPlayer src={SRC} />);
    expect(container.querySelector(".bg-gradient-to-b")).toBeNull();
  });

  // ⚠ **The mute button's NAME tracks its state.** A control permanently labelled "Mute" lies as
  // soon as the video is muted, and a screen-reader user has no other way to know which way it
  // will go. (Play/pause has the same rule, but its state is driven by media events jsdom never
  // fires, so mute is where the rule is testable.)
  it("renames the mute button once muted", () => {
    render(<VideoPlayer src={SRC} />);

    fireEvent.click(screen.getByRole("button", { name: "Mute" }));
    expect(screen.getByRole("button", { name: "Unmute" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Mute" })).not.toBeInTheDocument();
  });

  // ⚠ **`aria-valuetext` must be a TIME, and must live on the element carrying `role="slider"`.**
  // Radix puts that role on the Thumb, so attributes set on the Root land on a plain <span> and
  // are silently ignored — which is exactly what shipped first: the control announced
  // `aria-valuenow="3.4068"`, a raw float of seconds, and `aria-valuetext` read back as null.
  // Caught in the browser, not here, which is why it is pinned here now.
  it("announces the playhead as a time, on the element that owns the slider role", () => {
    render(<VideoPlayer src={SRC} title="Frosted Flakes" />);

    const slider = screen.getByRole("slider");
    expect(slider).toHaveAttribute("aria-label", "Seek");
    expect(slider).toHaveAttribute("aria-valuetext", "0:00 of 0:00");
  });

  // Before metadata arrives there is no honest time to show. A dashed placeholder says "not known
  // yet"; "0:00 / 0:00" would assert a zero-length video.
  it("shows a placeholder time until metadata loads", () => {
    render(<VideoPlayer src={SRC} />);
    expect(screen.getByText("–:–– / –:––")).toBeInTheDocument();
  });

  // `leading` is how a dialog puts its close button ON the frame rather than above it. The
  // primitive itself has no opinion about dismissal.
  it("renders a caller-supplied leading control", () => {
    render(<VideoPlayer src={SRC} leading={<button type="button">Close</button>} />);
    expect(screen.getByRole("button", { name: "Close" })).toBeInTheDocument();
  });
});
