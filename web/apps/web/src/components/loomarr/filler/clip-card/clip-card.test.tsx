import type { ClipDTO } from "@loomarr/api";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ClipCard } from "./clip-card";

const base: ClipDTO = {
  name: "Sunny D — Dude!",
  kind: "commercial",
  durationMs: 30000,
  era: 1990,
  audience: "kids",
  category: "food",
  tagged: true,
  aiTagged: false,
  playCount: 0,
  playsCounted: true,
  path: "clip-test.mp4",
  tunarrProgramId: "clip-test",
};

describe("ClipCard", () => {
  it("renders kind, era, audience chips and a sub-minute mono duration", () => {
    render(<ClipCard clip={base} />);
    expect(screen.getByText("Commercial")).toBeInTheDocument();
    expect(screen.getByText("1990s")).toBeInTheDocument();
    expect(screen.getByText("Kids")).toBeInTheDocument();
    expect(screen.getByText("30s")).toBeInTheDocument();
  });

  it("flags an untagged clip with a Tag action", () => {
    render(
      <ClipCard clip={{ ...base, tagged: false, era: undefined, audience: undefined }} onTag={() => {}} />,
    );
    expect(screen.getByText("Untagged")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /tag clip/i })).toBeInTheDocument();
  });

  it("marks AI-suggested tags and offers a confirm", () => {
    render(<ClipCard clip={{ ...base, tagged: false, aiTagged: true }} onConfirmTags={() => {}} />);
    expect(screen.getByText("AI-tagged")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /confirm tags/i })).toBeInTheDocument();
  });

  // A tagged clip must still be editable. §10's likely tagging error is a trailer scanned
  // as a commercial — it arrives with era/audience/category filled in, so it counts as
  // "tagged" while being wrong, and kind drives pod ROLE. Gating the edit on `!tagged`
  // left precisely that clip uncorrectable from the UI.
  it("offers an edit path for an already-tagged clip", () => {
    render(<ClipCard clip={{ ...base, tagged: true }} onTag={() => {}} />);
    expect(screen.getByRole("button", { name: /edit tags/i })).toBeInTheDocument();
  });

  // ⚠ The important half of V17b. A placeholder for every frameless clip would be the wrong
  // default: on a Tunarr-backed install, or one where ffmpeg never ran, that is the ENTIRE
  // catalog, and a grid of identical grey rectangles reads as a broken page rather than an
  // absent nicety. Absence is what shipped before this phase, and it already works.
  it("renders no image when the clip has no extracted frame", () => {
    const { container } = render(<ClipCard clip={{ ...base, thumbnail: undefined }} />);
    expect(container.querySelector("img")).toBeNull();
  });

  it("renders the extracted frame when the clip has one", () => {
    const { container } = render(
      <ClipCard clip={{ ...base, path: "80s/toys/intro.mp4", thumbnail: "80s/toys/intro.jpg" }} />,
    );
    const img = container.querySelector("img");
    expect(img).not.toBeNull();
    // Built from the clip's PATH, not from `thumbnail` — the route derives the .jpg itself,
    // so passing the thumbnail path would request `intro.jpg.jpg`.
    expect(img).toHaveAttribute("src", "/v1/filler/thumb/80s/toys/intro.mp4");
    // A catalog is hundreds of cards; without this every frame is fetched on mount.
    expect(img).toHaveAttribute("loading", "lazy");
  });

  // Empty alt, deliberately: the clip's name is the very next element, so a description here
  // would have a screen reader announce the same clip twice.
  it("leaves the frame's alt empty because the name is already announced", () => {
    const { container } = render(<ClipCard clip={{ ...base, name: "Frosted Flakes", thumbnail: "a.jpg" }} />);
    expect(container.querySelector("img")).toHaveAttribute("alt", "");
    expect(screen.getByText("Frosted Flakes")).toBeInTheDocument();
  });

  // §10 era grounding (V34): an ungrounded AI year renders as a QUESTION, never as a tag —
  // and the confirm affordance appears only when the call site offers it (admin).
  it("renders an ungrounded AI era as a suggestion, not a tag", () => {
    render(<ClipCard clip={{ ...base, era: undefined, suggestedEra: 1985 }} />);
    expect(screen.getByText("1985s?")).toBeInTheDocument();
    // No confirm without the handler — a member sees the question but cannot answer it.
    expect(screen.queryByRole("button", { name: /confirm 1985/i })).not.toBeInTheDocument();
  });

  it("offers the admin a one-click era confirm", () => {
    const onConfirmEra = vi.fn();
    render(<ClipCard clip={{ ...base, era: undefined, suggestedEra: 1985 }} onConfirmEra={onConfirmEra} />);
    fireEvent.click(screen.getByRole("button", { name: /confirm 1985/i }));
    expect(onConfirmEra).toHaveBeenCalledOnce();
  });

  // ⚠ These four pin fields the API has always sent and the card never rendered — a clip
  // that never airs looked identical to one on every break. The last case is the one that
  // matters most: `playsCounted:false` is NOT zero plays.
  it("reads as never played when the count is zero", () => {
    render(<ClipCard clip={{ ...base, playCount: 0, playsCounted: true }} />);
    expect(screen.getByText(/never played/i)).toBeInTheDocument();
  });

  it("shows the play count and when it last aired", () => {
    render(
      <ClipCard clip={{ ...base, playCount: 12, playsCounted: true, lastPlayedAt: new Date().toISOString() }} />,
    );
    expect(screen.getByText(/12 plays/)).toBeInTheDocument();
    expect(screen.getByText(/just now/i)).toBeInTheDocument();
  });

  it("singularises a single play", () => {
    render(<ClipCard clip={{ ...base, playCount: 1, playsCounted: true }} />);
    expect(screen.getByText(/1 play\b/)).toBeInTheDocument();
    expect(screen.queryByText(/1 plays/)).not.toBeInTheDocument();
  });

  // ⚠ The distinction the DTO's own comment warns about: an install whose playout is
  // Tunarr-backed cannot observe airings, so rendering "0 plays" would assert something
  // Loomarr does not know. Sabotaging the branch to fall through to the zero case makes
  // this fail — which is the point of asserting the absence too.
  it("says plays are not counted rather than claiming zero", () => {
    render(<ClipCard clip={{ ...base, playCount: 0, playsCounted: false }} />);
    expect(screen.getByText(/plays aren't counted/i)).toBeInTheDocument();
    expect(screen.queryByText(/never played/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/0 plays/)).not.toBeInTheDocument();
  });

  // Quality is display-only unless filler.min_quality is set (off by default), so it is a
  // neutral fact — a test pins that it renders at all, since nothing did before.
  it("shows the clip's resolution when the probe found one", () => {
    render(<ClipCard clip={{ ...base, quality: "480p" }} />);
    expect(screen.getByText("480p")).toBeInTheDocument();
  });

  // Inline retag (the v2 mock's cycleEra/cycleAud/cycleCat).
  it("cycles a tag to the next value on click", () => {
    const onCycle = vi.fn();
    render(<ClipCard clip={{ ...base, era: 1990 }} onCycle={onCycle} />);
    fireEvent.click(screen.getByRole("button", { name: /change the era/i }));
    expect(onCycle).toHaveBeenCalledWith({ era: 2000 });
  });

  // ⚠ UNSET must be reachable. Cycling that only advances through values leaves a wrongly
  // tagged clip un-blankable without opening the dialog — and §10 says the likely error IS
  // a mis-tagged clip (a trailer scanned as a commercial).
  it("cycles through unset rather than trapping a wrong tag", () => {
    const onCycle = vi.fn();
    render(<ClipCard clip={{ ...base, era: 2020 }} onCycle={onCycle} />);
    fireEvent.click(screen.getByRole("button", { name: /change the era/i }));
    expect(onCycle).toHaveBeenCalledWith({ era: 0 });
  });

  it("emits only the field that changed, so the caller supplies the siblings", () => {
    const onCycle = vi.fn();
    render(<ClipCard clip={{ ...base, audience: "kids", category: "food" }} onCycle={onCycle} />);
    fireEvent.click(screen.getByRole("button", { name: /change the audience/i }));
    expect(onCycle).toHaveBeenCalledWith({ audience: "family" });
  });

  // ⚠ A member gets the tags as plain badges, never as controls: every retag route 403s
  // server-side (§11, §19), and a button that always fails is worse than no button.
  it("renders tags as static badges when retagging is not offered", () => {
    render(<ClipCard clip={{ ...base, era: 1990 }} />);
    expect(screen.getByText("1990s")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /change the era/i })).not.toBeInTheDocument();
  });

  // The split entry point (§10 V34): present only when offered (admin), and disabled while
  // detection runs so a minutes-long decode can't be queued twice.
  it("offers split detection and shows its pending state", () => {
    const onSplit = vi.fn();
    render(<ClipCard clip={base} onSplit={onSplit} />);
    fireEvent.click(screen.getByRole("button", { name: /split into clips/i }));
    expect(onSplit).toHaveBeenCalledOnce();

    render(<ClipCard clip={base} onSplit={onSplit} splitPending />);
    expect(screen.getByRole("button", { name: /splitting/i })).toBeDisabled();
  });
});
