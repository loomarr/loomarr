import { useMemo, useState } from "react";
import { thumbHashToDataURL } from "thumbhash";
import { cn } from "@/lib";
import type { ImageProps } from "./image.type";

// Image — THE way this app renders anything served by the image service (§22, V52 phase 4).
//
// ⚠ **No surface hand-writes an `<img>` against `/v1/images`.** Before this, four different
// arrangements each solved a piece of the problem and none solved all of it: one hardcoded
// `w500` shipped the same poster to a 40px hover chip and a full channel tile, there was no
// `srcset` anywhere in the app because nothing could produce one, and TMDB posters loaded
// straight from a third-party origin in the operator's browser.
//
// Three properties are REQUIRED here rather than optional, and each one exists because its
// absence is a specific, common regression:
//
//  1. **Explicit width/height** → the browser derives `aspect-ratio` and reserves the box, so
//     cumulative layout shift is zero. Free, because the API returns real measured dimensions.
//  2. **A `priority` escape hatch** → lazy-loading the LCP image is the most common
//     self-inflicted image regression on the web, and a blanket lazy rule walks into it.
//  3. **A built-in error fallback** → `logo` values can be operator-pasted arbitrary URLs, and
//     an upload whose bytes are gone must not render as a broken image.

// PLACEHOLDER_DECODE_FAILED is what a malformed `placeholder` yields. ⚠ Swallowed rather than
// thrown: a ThumbHash is a nicety, and a corrupt one must degrade to "no blur" rather than
// taking down the surface it decorates. The dominant colour is still there underneath.
const decodePlaceholder = (placeholder: string): string | undefined => {
  if (!placeholder) return undefined;
  try {
    // base64 → bytes. `atob` is fine here: the value is server-generated and ~25 bytes.
    const binary = atob(placeholder);
    const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0));
    // ⚠ `thumbHashToDataURL`, NOT `thumbHashToRGBA` + a canvas. The library builds a PNG by
    // hand in pure JS, so this works in jsdom and in a Storybook build with no canvas — and
    // costs no main-thread canvas allocation per image on a fifty-poster grid.
    return thumbHashToDataURL(bytes);
  } catch {
    return undefined;
  }
};

const Image = ({ image, alt, sizes, priority = false, fallback, className }: ImageProps) => {
  // ⚠ The failure carries the identity of WHICH image failed, rather than being a bare boolean.
  // React reconciles by POSITION, so a grid that paginates, filters or sorts hands a different
  // `image` to this same instance — and a plain `useState(false)` survives that swap, rendering
  // a perfectly good image as a colour block forever. It would not even look broken: the block
  // reads the NEW image's `dominantHex`, so it reads as a deliberate empty state.
  //
  // Deriving `failed` by comparison needs no effect and no reset — the moment the hash changes
  // the comparison is false again, in the same render.
  //
  // ⚠ Sticky per image, deliberately. A `logo` is an operator-pasted arbitrary URL (§22), so a
  // failure is usually permanent, and re-requesting known-bad bytes on every re-render is the
  // worse default; a genuine network blip costs a colour block until the surface remounts.
  const [failedHash, setFailedHash] = useState<string | null>(null);
  const failed = failedHash === image.hash;

  // Memoised on the placeholder itself, which is also the value read — the two can never
  // disagree. Keying on `image.hash` would look equivalent (the hash IS the identity, and a
  // ThumbHash is derived from the same bytes) but it is the classic stale-closure shape: the
  // fetch job re-keys a row from `url:`-hash to content-hash and back-fills `placeholder`, so a
  // hash-keyed memo would hold the previous row's blur until something else re-rendered.
  const blurURL = useMemo(() => decodePlaceholder(image.placeholder), [image.placeholder]);

  const aspect = `${image.width} / ${image.height}`;

  if (failed) {
    // ⚠ **A flat block in the image's own dominant colour — never a broken-image glyph, and
    // never the blurred placeholder left in place.** The blur is the tempting option and is the
    // wrong one: it is indistinguishable from a slow network, forever, which turns §22's
    // deliberately-accepted "operator uploads do not survive losing /data/images" into something
    // that looks like a bug that might still resolve itself.
    //
    // Callers with a real designed empty state — a channel's monogram, the icon field's glyph —
    // pass `fallback` and get that instead. The default exists so a surface that never thought
    // about failure still renders something that looks deliberate.
    // ⚠ `!== undefined`, NOT `??`. The two are different questions: `??` cannot tell "I did not
    // specify a fallback" apart from "I explicitly want NOTHING rendered", and collapses both to
    // the default block. That distinction is real — the clip card's hover loop stacks ON its still
    // and must vanish on failure to reveal the still beneath, where a colour block would be a
    // visible fault over the frame. Passing `null` says so; omitting the prop still gets the
    // default. (`<></>` expressed it too, and Biome rejects a useless fragment — correctly, since
    // the empty fragment was working around this API rather than saying what it meant.)
    if (fallback !== undefined) return <>{fallback}</>;
    return (
      <div
        // `presentation` rather than `img`: there is no image here to describe. A `role="img"`
        // with the original alt text would announce artwork that is not being shown.
        role="presentation"
        style={{ aspectRatio: aspect, backgroundColor: image.dominantHex || undefined }}
        className={cn("w-full bg-static-800", className)}
      />
    );
  }

  return (
    // ⚠ `contents` (display: contents) so the <picture> does NOT participate in layout — the
    // <img> becomes the direct flex/grid child of whatever the caller rendered this into.
    //
    // Without it this primitive silently mis-sizes wherever the caller sizes it with classes.
    // `className` lands on the <img>, so `size-full` resolves against the <picture>, which has
    // no size of its own and shrink-wraps — MEASURED at 1×1 in the channel icon field's 64px
    // preview, against 62×62 for the plain <img> beside it.
    //
    // ⚠ Two defects overlapped here and each hid the other's evidence: the story's data-URI
    // `srcset` also meant the image never loaded (naturalWidth 0, issue #210), so the placeholder
    // rendered either way and the screenshot looked identical with and without this line. Only
    // measuring both `naturalWidth` AND the bounding box separated them. Do not "simplify" this
    // away on the strength of a screenshot.
    //
    // <picture> exists only to host the <source> elements and carries no semantics of its own,
    // so removing it from layout costs nothing — the <img> keeps its role and its alt text.
    <picture className="contents">
      {/*
        ⚠ The AVIF source is omitted when the job has not caught up, rather than emitted and
        left to 404. §22 makes AVIF coverage EVENTUALLY CONSISTENT on purpose — every encode
        forks a multithreaded ffmpeg, so a cold grid of fifty posters would fork fifty at once —
        and `<picture>` handles that natively: no source, browser takes WebP, nothing waits and
        no surface has to know whether the job has run.
      */}
      {image.srcSetAvif ? <source type="image/avif" srcSet={image.srcSetAvif} sizes={sizes} /> : null}
      <source type="image/webp" srcSet={image.srcSetWebp} sizes={sizes} />
      <img
        src={image.src}
        alt={alt}
        // Real measured dimensions, so the browser reserves the box before a byte arrives.
        width={image.width}
        height={image.height}
        // ⚠ `priority` flips all three together and that is deliberate. Eager loading with a low
        // fetch priority still queues behind other work, and `decoding="async"` can defer the
        // very paint being measured — so a half-applied "priority" is worse than none, because
        // it reads as handled.
        loading={priority ? "eager" : "lazy"}
        fetchPriority={priority ? "high" : "low"}
        decoding={priority ? "sync" : "async"}
        onError={() => setFailedHash(image.hash)}
        style={{
          aspectRatio: aspect,
          // The ThumbHash sits BEHIND the image rather than in front of it, so there is nothing
          // to un-swap: the moment the real bytes decode they simply cover it. A foreground
          // placeholder needs an onLoad to remove, which is one more state and one more way to
          // get stuck showing a blur over a loaded image.
          backgroundImage: blurURL ? `url(${blurURL})` : undefined,
          backgroundColor: blurURL ? undefined : image.dominantHex || undefined,
          backgroundSize: "cover",
          backgroundPosition: "center",
        }}
        className={cn("h-auto w-full object-cover", className)}
      />
    </picture>
  );
};

export { Image };
