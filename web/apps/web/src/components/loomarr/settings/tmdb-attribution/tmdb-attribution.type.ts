interface TmdbAttributionProps {
  /**
   * The TMDB logo, as a rendered node (an <img> or inline <svg>).
   *
   * ⚠ **A prop rather than a bundled asset, because the asset is TMDB's trademark and this
   * repository does not carry one.** §22 requires the logo be shown and be *less prominent* than
   * Loomarr's own branding; the component owns the "less prominent" half — the sizing and the
   * placement — and the operator's build supplies the mark itself. Passing nothing renders the
   * notice alone, which is the more important half of the obligation and is never wrong to show.
   */
  logo?: React.ReactNode;
  className?: string;
}

export type { TmdbAttributionProps };
