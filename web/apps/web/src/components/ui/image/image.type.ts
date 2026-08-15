import type { ImageDTO } from "@loomarr/api/models/imageDTO";
import type { ReactNode } from "react";

interface ImageProps {
  /**
   * The image's record, straight from `GET /v1/images/{hash}`.
   *
   * ⚠ The whole DTO rather than a hash, and that is the point of the primitive. Real
   * `width`/`height` are what let the browser reserve the box before a byte arrives, and they
   * only exist because the server measured the original — a component taking just a hash would
   * have to guess them from the role, which is wrong for every operator-uploaded logo.
   */
  image: ImageDTO;

  /**
   * Alt text. Required, and `""` is a legitimate value meaning "decorative" — a clip still
   * beside its own title says nothing a screen reader needs twice.
   *
   * ⚠ Required rather than optional so the decision is made deliberately at each call site.
   * An optional prop is one the next surface omits, and a missing `alt` is an axe violation at
   * SERIOUS impact that no visual baseline can see.
   */
  alt: string;

  /**
   * The CSS `sizes` attribute — how wide this image will actually be rendered.
   *
   * ⚠ **Ship an explicit value; never `sizes="auto"`.** Chrome and Firefox support it and
   * **Safari does not, in any version** (§22). On Safari the browser would fall back to assuming
   * 100vw and download the largest rung for a 40px chip. It is an Interop 2026 focus — revisit
   * then, not now.
   */
  sizes: string;

  /**
   * Eager, high-priority, synchronously-decoded loading for an image that is (or may be) the
   * Largest Contentful Paint.
   *
   * ⚠ **Lazy-loading the LCP image is the most common self-inflicted image regression on the
   * web**, and a blanket "lazy-load everything" rule walks straight into it. The default here is
   * lazy + async + low, which is right for the ninety images below the fold; the first row of any
   * poster grid sets this.
   *
   * `priority` also turns OFF async decoding, because `decoding="async"` can defer the very paint
   * being measured — the opposite of what the flag is for.
   */
  priority?: boolean;

  /**
   * What to render instead when the bytes cannot be loaded.
   *
   * ⚠ **Three states, not two.** Omitted ⇒ the built-in flat block in the image's dominant colour.
   * A node ⇒ that node (a channel's monogram, the icon field's glyph). **`null` ⇒ render NOTHING**,
   * for a caller layering this over something that should show through — the clip card's hover
   * loop sits on its still, and a colour block there would be a visible fault where the honest
   * state is "this clip has no preview".
   *
   * The implementation tests `!== undefined` rather than using `??` precisely so `null` can mean
   * the third thing. See the note in image.tsx.
   */
  fallback?: ReactNode;

  /** Extra classes for the rendered element. */
  className?: string;
}

export type { ImageProps };
