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
  hash: "hash-clip-test",
  tunarrProgramId: "clip-test",
};

// Artwork is an image-service record since V52 phase 8 — `thumbnail`/`preview` and the
// /v1/filler/thumb|hover routes they addressed are retired (§22).
const imageRecord = (hash: string, animated = false) => ({
  hash,
  role: animated ? "thumb" : "thumb",
  width: 500,
  height: 281,
  placeholder: "1QcSHQRnh493V4dIh4eXh1h4kJUI",
  dominantHex: "#2b4a5e",
  animated,
  srcSetWebp: `/v1/images/${hash}/w342.webp 342w, /v1/images/${hash}/w500.webp 500w`,
  srcSetAvif: "",
  src: `/v1/images/${hash}/w500.jpg`,
});

const framedImage = imageRecord("aaaa1111");
const hoverImage = imageRecord("bbbb2222", true);

describe("ClipCard", () => {
  it("renders kind, era, audience chips and a sub-minute mono duration", () => {
    render(<ClipCard clip={base} />);
    expect(screen.getByText("Commercial")).toBeInTheDocument();
    expect(screen.getByText("1990s")).toBeInTheDocument();
    expect(screen.getByText("Kids")).toBeInTheDocument();
    expect(screen.getByText("30s")).toBeInTheDocument();
  });

  it("keeps missing optional details quiet for a Ready clip", () => {
    render(
      <ClipCard
        clip={{ ...base, kind: "unclassified", tagged: false, era: undefined, audience: undefined }}
        onOpen={() => {}}
      />,
    );
    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.queryByText(/Untagged|Geography unknown|Unclassified/i)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /tag clip|use in a channel/i })).not.toBeInTheDocument();
  });
  // §10 V45a: the "AI-tagged" badge was removed (it told an operator nothing actionable). The
  // confirm-tags affordance for an AI-suggested clip remains — that IS the useful action.
  it("does not turn automatic metadata into a confirmation chore", () => {
    render(<ClipCard clip={{ ...base, tagged: false, aiTagged: true }} onOpen={() => {}} />);
    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /confirm tags/i })).not.toBeInTheDocument();
  });
  // A tagged clip must still be editable. §10's likely tagging error is a trailer scanned
  // as a commercial — it arrives with era/audience/category filled in, so it counts as
  // "tagged" while being wrong, and kind drives pod ROLE. Gating the edit on `!tagged`
  // left precisely that clip uncorrectable from the UI.
  it("opens inspection for already-tagged clips", () => {
    const onOpen = vi.fn();
    render(<ClipCard clip={base} onOpen={onOpen} />);
    fireEvent.click(screen.getByRole("button", { name: "View details for Sunny D — Dude!" }));
    expect(onOpen).toHaveBeenCalledOnce();
  });
  // ⚠ The important half of V17b. A placeholder for every frameless clip would be the wrong
  // default: on a Tunarr-backed install, or one where ffmpeg never ran, that is the ENTIRE
  // catalog, and a grid of identical grey rectangles reads as a broken page rather than an
  // absent nicety. Absence is what shipped before this phase, and it already works.
  it("renders no image when the clip has no extracted frame", () => {
    const { container } = render(<ClipCard clip={{ ...base, thumbImage: undefined }} />);
    expect(container.querySelector("img")).toBeNull();
  });

  it("renders the extracted frame when the clip has one", () => {
    const { container } = render(
      <ClipCard clip={{ ...base, hash: "hash-intro", thumbImage: framedImage }} />,
    );
    const img = container.querySelector("img");
    expect(img).not.toBeNull();
    // ⚠ Addressed by the IMAGE's content hash, not the clip's. Until V52 phase 8 this asserted
    // `/v1/filler/thumb/{clipHash}` — a route that derived the file from the clip server-side. retired-ok
    // Artwork now has its own identity, which is what makes the URL immutably cacheable.
    expect(img).toHaveAttribute("src", framedImage.src);
    // A catalog is hundreds of cards; without this every frame is fetched on mount.
    expect(img).toHaveAttribute("loading", "lazy");
  });

  // Empty alt, deliberately: the clip's name is the very next element, so a description here
  // would have a screen reader announce the same clip twice.
  it("leaves the frame's alt empty because the name is already announced", () => {
    const { container } = render(
      <ClipCard clip={{ ...base, name: "Frosted Flakes", thumbImage: framedImage }} />,
    );
    expect(container.querySelector("img")).toHaveAttribute("alt", "");
    expect(screen.getByText("Frosted Flakes")).toBeInTheDocument();
  });

  // §10 era grounding (V34): an ungrounded AI year renders as a QUESTION, never as a tag —
  // and the confirm affordance appears only when the call site offers it (admin).
  it("does not display an ungrounded year as a known fact", () => {
    render(<ClipCard clip={{ ...base, era: undefined, suggestedEra: 1985 }} />);
    expect(screen.queryByText(/1985/)).not.toBeInTheDocument();
  });
  it("does not offer one-click affirmation of an ungrounded year", () => {
    render(<ClipCard clip={{ ...base, era: undefined, suggestedEra: 1985 }} onOpen={() => {}} />);
    expect(screen.queryByRole("button", { name: /confirm/i })).not.toBeInTheDocument();
  });
  // ⚠ These four pin fields the API has always sent and the card never rendered — a clip
  // that never airs looked identical to one on every break. The last case is the one that
  // matters most: `playsCounted:false` is NOT zero plays.
  it("reads as never aired when the count is zero", () => {
    render(<ClipCard clip={{ ...base, playCount: 0, playsCounted: true }} />);
    expect(screen.getByText(/never aired/i)).toBeInTheDocument();
  });

  // §10 V45a: the counter is AIRINGS (broadcast), so the copy is "aired"/"airings", not "played"
  // — "played" read as "have I watched this" next to a Play button that never moves the count.
  it("shows the airing count and when it last aired", () => {
    render(
      <ClipCard
        clip={{ ...base, playCount: 12, playsCounted: true, lastPlayedAt: new Date().toISOString() }}
      />,
    );
    expect(screen.getByText(/12 airings/)).toBeInTheDocument();
    expect(screen.getByText(/just now/i)).toBeInTheDocument();
  });

  it("singularises a single airing", () => {
    render(<ClipCard clip={{ ...base, playCount: 1, playsCounted: true }} />);
    expect(screen.getByText(/1 airing\b/)).toBeInTheDocument();
    expect(screen.queryByText(/1 airings/)).not.toBeInTheDocument();
  });

  // ⚠ The distinction the DTO's own comment warns about: an install whose playout is
  // Tunarr-backed cannot observe airings, so rendering "0 airings" would assert something
  // Loomarr does not know. Sabotaging the branch to fall through to the zero case makes
  // this fail — which is the point of asserting the absence too.
  it("says airings are not counted rather than claiming zero", () => {
    render(<ClipCard clip={{ ...base, playCount: 0, playsCounted: false }} />);
    expect(screen.getByText(/airings aren't counted/i)).toBeInTheDocument();
    expect(screen.queryByText(/never aired/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/0 airings/)).not.toBeInTheDocument();
  });

  // Quality is display-only unless filler.min_quality is set (off by default), so it is a
  // neutral fact — a test pins that it renders at all, since nothing did before.
  it("shows the clip's resolution when the probe found one", () => {
    render(<ClipCard clip={{ ...base, quality: "480p" }} />);
    expect(screen.getByText("480p")).toBeInTheDocument();
  });

  // Inline retag (the v2 mock's cycleEra/cycleAud/cycleCat).
  it("renders the year as a read-only fact", () => {
    render(<ClipCard clip={{ ...base, era: 1990 }} onOpen={() => {}} />);
    expect(screen.getByText("1990s")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /change the era/i })).not.toBeInTheDocument();
  });
  // ⚠ UNSET must be reachable. Cycling that only advances through values leaves a wrongly
  // tagged clip un-blankable without opening the dialog — and §10 says the likely error IS
  // a mis-tagged clip (a trailer scanned as a commercial).
  it("does not invent an unset year control", () => {
    render(<ClipCard clip={{ ...base, era: 0 }} onOpen={() => {}} />);
    expect(screen.queryByRole("button", { name: /era/i })).not.toBeInTheDocument();
  });
  it("keeps audience facts noninteractive while inspection remains accessible", () => {
    render(<ClipCard clip={base} onOpen={() => {}} />);
    expect(screen.getByText("Kids")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /change the audience/i })).not.toBeInTheDocument();
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

  // The hover preview and its play button (V39).
  describe("preview and play", () => {
    const framed = { ...base, hash: "hash-intro", thumbImage: framedImage };

    // ⚠ **The name says WHICH clip.** A grid of buttons all called "Play" is meaningless in a
    // screen reader's element list and unusable by voice control ("click play" — which one?).
    it("names the play button after the clip", () => {
      const onPlay = vi.fn();
      render(<ClipCard clip={{ ...framed, name: "Frosted Flakes" }} onOpen={onPlay} />);

      fireEvent.click(screen.getByRole("button", { name: "Preview Frosted Flakes" }));
      expect(onPlay).toHaveBeenCalledOnce();
    });

    // ⚠ **The button is in the DOM before any hover, revealed by opacity.** Rendering it only on
    // hover would make it unreachable by Tab — a keyboard user could never play a clip. This is
    // the assertion that catches a "simplification" to conditional rendering.
    it("keeps the play button focusable without a hover", () => {
      render(<ClipCard clip={framed} onOpen={() => {}} />);
      // Queried with no pointer events fired at all.
      expect(screen.getByRole("button", { name: /^preview/i })).toBeInTheDocument();
    });

    // The animation is NOT fetched on mount. A catalog is hundreds of cards, and mounting every
    // preview would pull the whole grid's worth of webp immediately — the exact cost the still
    // exists to avoid.
    it("loads the animation only once hovered", () => {
      const { container } = render(<ClipCard clip={{ ...framed, hoverImage }} onOpen={() => {}} />);

      expect(container.querySelector('img[src*="bbbb2222"]')).toBeNull();

      fireEvent.mouseEnter(container.querySelector(".group") as HTMLElement);
      const preview = container.querySelector('img[src*="bbbb2222"]');
      // ⚠ The ANIMATION's own content hash, not the clip's. Until V52 phase 8 this was
      // `/v1/filler/hover/{clipHash}`, a route that derived the .webp from the clip server-side; retired-ok
      // the hover loop is an image-service image now and carries its own identity.
      expect(preview).toHaveAttribute("src", hoverImage.src);
    });

    // ⚠ **Unmounting on leave is what makes the animation RESTART on the next hover.** An
    // animated WebP begins at frame 0 when it decodes and then runs on its own — the browser
    // keeps it going while the element lives, whether or not anyone is looking. Hiding it instead
    // of unmounting meant a second hover picked the loop up mid-advert. (Maintainer, 2026-08-03.)
    //
    // This is the test that catches a "keep it mounted for caching" optimisation reintroducing it.
    it("unmounts the animation on leave so the next hover starts it again", () => {
      const { container } = render(<ClipCard clip={{ ...framed, hoverImage }} onOpen={() => {}} />);
      const frame = container.querySelector(".group") as HTMLElement;

      fireEvent.mouseEnter(frame);
      expect(container.querySelector('img[src*="bbbb2222"]')).toBeInTheDocument();

      fireEvent.mouseLeave(frame);
      expect(container.querySelector('img[src*="bbbb2222"]')).toBeNull();

      // ...and it comes back, freshly decoded from frame 0.
      fireEvent.mouseEnter(frame);
      expect(container.querySelector('img[src*="bbbb2222"]')).toBeInTheDocument();
    });

    // ⚠ A clip with no rendered preview must not request one. That is EVERY clip on an install
    // that has not re-synced since V39, so a card that asked anyway would 404 once per hover
    // across the whole catalog.
    it("does not request an animation for a clip that has none", () => {
      const { container } = render(<ClipCard clip={framed} onOpen={() => {}} />);
      fireEvent.mouseEnter(container.querySelector(".group") as HTMLElement);

      expect(container.querySelector('img[src*="bbbb2222"]')).toBeNull();
      // ...and the still is still there, which is the whole fallback.
      expect(container.querySelector('img[src*="aaaa1111"]')).toBeInTheDocument();
    });

    // ⚠ **Without a frame there is nowhere for the disc to sit** — and on a Tunarr-backed install,
    // or one where ffmpeg never ran, that is the ENTIRE catalog. The action row carries the
    // fallback, or the feature is invisible on exactly those installs.
    it("opens a frameless clip through its title without another action row", () => {
      const onOpen = vi.fn();
      render(<ClipCard clip={{ ...base, thumbImage: undefined }} onOpen={onOpen} />);
      fireEvent.click(screen.getByRole("button", { name: "View details for Sunny D — Dude!" }));
      expect(onOpen).toHaveBeenCalledOnce();
      expect(screen.queryByRole("button", { name: /preview/i })).not.toBeInTheDocument();
    });
    // ...and NOT both at once: two "Play X" buttons on one card is noise on screen and a real
    // problem in a screen reader's element list.
    it("offers exactly one play control when there is a thumbnail", () => {
      render(<ClipCard clip={framed} onOpen={() => {}} />);
      expect(screen.getAllByRole("button", { name: /preview/i })).toHaveLength(1);
    });

    // No handler, no control anywhere — the honest degraded state for a caller with nowhere to
    // open a player.
    it("renders no play control without a handler", () => {
      render(<ClipCard clip={framed} />);
      expect(screen.queryByRole("button", { name: /preview/i })).not.toBeInTheDocument();
    });
  });
});
