import type { ImageDTO } from "@loomarr/api";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Image } from "./image";

const PNG =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQzwAEjP8ZAAoDAv8T7QhZAAAAAElFTkSuQmCC";

const dto = (over: Partial<ImageDTO> = {}): ImageDTO => ({
  hash: "9f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d65039f2b1c4e8a7d6503",
  role: "poster",
  width: 500,
  height: 750,
  placeholder: "1QcSHQRnh493V4dIh4eXh1h4kJUI",
  dominantHex: "#7a5230",
  animated: false,
  srcSetWebp: `${PNG} 342w, ${PNG} 500w`,
  srcSetAvif: `${PNG} 342w, ${PNG} 500w`,
  src: PNG,
  ...over,
});

describe("Image", () => {
  // ⚠ The reason the component takes the whole DTO rather than a hash. Without real dimensions
  // the browser cannot reserve the box, and the poster grid shifts every card down as each image
  // arrives — cumulative layout shift no visual baseline can catch, because a snapshot is taken
  // after everything has settled.
  it("carries real measured dimensions so the box is reserved before any byte arrives", () => {
    render(<Image image={dto()} alt="Poster" sizes="320px" />);
    const img = screen.getByAltText("Poster");
    expect(img).toHaveAttribute("width", "500");
    expect(img).toHaveAttribute("height", "750");
    expect(img.style.aspectRatio).toBe("500 / 750");
  });

  it("offers both formats when the AVIF job has produced renditions", () => {
    const { container } = render(<Image image={dto()} alt="Poster" sizes="320px" />);
    const types = [...container.querySelectorAll("source")].map((s) => s.getAttribute("type"));
    expect(types).toEqual(["image/avif", "image/webp"]);
  });

  // ⚠ The NORMAL state for up to an hour after an image lands, not an edge case: AVIF is
  // job-produced (§22), so the source must be OMITTED rather than emitted and left to 404.
  // `<picture>` then takes WebP natively and nothing waits.
  it("omits the AVIF source entirely when no AVIF rendition exists yet", () => {
    const { container } = render(<Image image={dto({ srcSetAvif: "" })} alt="Poster" sizes="320px" />);
    const types = [...container.querySelectorAll("source")].map((s) => s.getAttribute("type"));
    expect(types).toEqual(["image/webp"]);
  });

  // ⚠ Explicit `sizes`, never `sizes="auto"` — Safari supports it in NO version, and would fall
  // back to assuming 100vw and download the largest rung for a 40px chip.
  it("puts the caller's sizes on every source", () => {
    const { container } = render(<Image image={dto()} alt="Poster" sizes="(max-width: 640px) 50vw, 320px" />);
    for (const source of container.querySelectorAll("source")) {
      expect(source.getAttribute("sizes")).toBe("(max-width: 640px) 50vw, 320px");
    }
  });

  describe("priority", () => {
    it("defaults to lazy, async and low — right for the ninety images below the fold", () => {
      render(<Image image={dto()} alt="Poster" sizes="320px" />);
      const img = screen.getByAltText("Poster");
      expect(img).toHaveAttribute("loading", "lazy");
      expect(img).toHaveAttribute("fetchpriority", "low");
      expect(img).toHaveAttribute("decoding", "async");
    });

    // ⚠ All three flip together. Eager loading at a LOW fetch priority still queues behind other
    // work, and `decoding="async"` can defer the very paint being measured — so a half-applied
    // priority is worse than none, because it reads as handled.
    it("flips all three together for the LCP image", () => {
      render(<Image image={dto()} alt="Poster" sizes="320px" priority />);
      const img = screen.getByAltText("Poster");
      expect(img).toHaveAttribute("loading", "eager");
      expect(img).toHaveAttribute("fetchpriority", "high");
      expect(img).toHaveAttribute("decoding", "sync");
    });
  });

  describe("the failure path", () => {
    it("falls back to a flat block in the image's own dominant colour", () => {
      const { container } = render(<Image image={dto()} alt="Poster" sizes="320px" />);
      fireEvent.error(screen.getByAltText("Poster"));

      expect(screen.queryByAltText("Poster")).not.toBeInTheDocument();
      const block = container.querySelector('[role="presentation"]');
      expect(block).toBeInTheDocument();
      // The right aspect, so a grid does not reflow when one image fails.
      expect((block as HTMLElement).style.aspectRatio).toBe("500 / 750");
      expect((block as HTMLElement).style.backgroundColor).toBe("rgb(122, 82, 48)");
    });

    it("renders a caller's designed empty state instead when one is supplied", () => {
      render(<Image image={dto()} alt="Poster" sizes="320px" fallback={<span>AH</span>} />);
      fireEvent.error(screen.getByAltText("Poster"));
      expect(screen.getByText("AH")).toBeInTheDocument();
    });

    // ⚠ React reconciles by POSITION, not by prop identity — so a poster grid that paginates,
    // filters or sorts hands a different `image` to the same component instance. If the failure
    // flag is plain `useState` it survives that swap, and a perfectly good image renders as a
    // colour block forever. Worse, the block reads the NEW image's dominantHex, so it looks like
    // a deliberate empty state rather than a stuck one.
    it("recovers when a different image is rendered into the same slot", () => {
      const { rerender } = render(<Image image={dto()} alt="A" sizes="320px" />);
      fireEvent.error(screen.getByAltText("A"));
      expect(screen.queryByAltText("A")).not.toBeInTheDocument();

      rerender(<Image image={dto({ hash: "0".repeat(64), src: PNG })} alt="B" sizes="320px" />);
      expect(screen.getByAltText("B")).toBeInTheDocument();
    });

    // The other half of keying the failure to a hash, and a deliberate choice rather than a
    // side effect: coming BACK to an image that already failed keeps the fallback instead of
    // re-requesting. A `logo` is an operator-pasted arbitrary URL (§22), so a failure is usually
    // permanent, and retrying known-bad bytes on every re-render is the worse default.
    it("does not re-request an image that already failed in this instance", () => {
      const stable = dto();
      const { rerender } = render(<Image image={stable} alt="A" sizes="320px" />);
      fireEvent.error(screen.getByAltText("A"));

      rerender(<Image image={dto({ hash: "0".repeat(64) })} alt="B" sizes="320px" />);
      expect(screen.getByAltText("B")).toBeInTheDocument();

      rerender(<Image image={stable} alt="A" sizes="320px" />);
      expect(screen.queryByAltText("A")).not.toBeInTheDocument();
    });

    // ⚠ The fallback must NOT keep the blurred placeholder showing. A permanent blur is
    // indistinguishable from a slow network, forever — which turns §22's deliberately-accepted
    // "operator uploads do not survive losing /data/images" into something that looks like a bug
    // that might still resolve itself. It has to look like a designed empty state.
    it("does not leave the blurred placeholder in place as the failure state", () => {
      const { container } = render(<Image image={dto()} alt="Poster" sizes="320px" />);
      fireEvent.error(screen.getByAltText("Poster"));
      const block = container.querySelector('[role="presentation"]') as HTMLElement;
      expect(block.style.backgroundImage).toBe("");
    });
  });

  describe("the placeholder", () => {
    it("decodes the ThumbHash behind the image", () => {
      render(<Image image={dto()} alt="Poster" sizes="320px" />);
      const img = screen.getByAltText("Poster");
      expect(img.style.backgroundImage).toContain("data:image/png;base64,");
    });

    // ⚠ A ThumbHash is a nicety. A corrupt one must degrade to "no blur" rather than throwing and
    // taking down the surface it decorates — the dominant colour is still there underneath.
    it("survives a malformed placeholder and falls back to the dominant colour", () => {
      render(<Image image={dto({ placeholder: "!!!not-base64!!!" })} alt="Poster" sizes="320px" />);
      const img = screen.getByAltText("Poster");
      expect(img.style.backgroundImage).toBe("");
      expect(img.style.backgroundColor).toBe("rgb(122, 82, 48)");
    });

    // The state between `Adopt` and the fetch job running: no bytes, so no measurements either.
    it("renders without a placeholder or a dominant colour", () => {
      render(<Image image={dto({ placeholder: "", dominantHex: "" })} alt="Poster" sizes="320px" />);
      const img = screen.getByAltText("Poster");
      expect(img.style.backgroundImage).toBe("");
      expect(img.style.backgroundColor).toBe("");
    });
  });

  // `alt=""` is a legitimate, deliberate value: a clip still beside its own title says nothing a
  // screen reader needs twice. What must never happen is the attribute being absent.
  it("always emits an alt attribute, including the decorative empty one", () => {
    const { container } = render(<Image image={dto()} alt="" sizes="320px" />);
    const img = container.querySelector("img");
    expect(img?.hasAttribute("alt")).toBe(true);
    expect(img?.getAttribute("alt")).toBe("");
  });
});
