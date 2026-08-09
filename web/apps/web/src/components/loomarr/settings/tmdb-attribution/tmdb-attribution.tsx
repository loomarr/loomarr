import { cn } from "@/lib";
import type { TmdbAttributionProps } from "./tmdb-attribution.type";

// TmdbAttribution — Settings → Connections, below the TMDB block (§22, V52 phase 7).
//
// ⚠ **This is a licence obligation, not decoration.** TMDB's API terms require the notice below
// verbatim and require their logo to be shown, less prominently than the product's own branding.
// §22 records it as "a UI deliverable, not a comment" precisely because a requirement of this kind
// is easy to satisfy in prose and never ship.
//
// It lives here rather than on the surfaces that display TMDB content because those surfaces are
// transient — the icon picker is a popover, the timeline a hover card — and a notice that appears
// only while a popover is open is not "prominent" in any useful sense. The Connections page is
// permanent, is where an operator connects TMDB in the first place, and puts the notice next to
// the integration it describes.
//
// ⚠ **Rendered unconditionally, not gated on TMDB being configured.** The obligation is a statement
// about what the product uses, and an instance that has not yet pasted an API key still ships the
// code that calls the API. Gating it would also make the notice appear and vanish as an operator
// edits a field, which reads as a bug.

// TMDB_NOTICE is fixed wording from TMDB's terms.
//
// ⚠ Do not paraphrase, shorten, or re-flow this into the surrounding prose. It is quoted text, and
// the exactness is the entire point — "not endorsed, certified, or otherwise approved" is the
// operative phrase.
const TMDB_NOTICE =
  "This product uses TMDB and the TMDB APIs but is not endorsed, certified, or otherwise approved by TMDB.";

const TmdbAttribution = ({ logo, className }: TmdbAttributionProps) => (
  <aside
    // A landmark with its own label: the notice is a distinct statement about a third party, not a
    // continuation of the settings above it, and a screen-reader user landing here should be told
    // which it is rather than hearing it run on from the last field.
    aria-label="TMDB attribution"
    className={cn(
      "mt-6 flex items-start gap-3 rounded-md border border-border bg-static-900/40 px-4 py-3",
      className,
    )}
  >
    {/* The "less prominent than our own branding" half of the requirement is expressed as a size
        cap: whatever mark is supplied renders small and muted, well under the page's own heading. */}
    {logo && (
      <span className="mt-0.5 flex h-4 shrink-0 items-center opacity-80 [&>*]:h-full [&>*]:w-auto">
        {logo}
      </span>
    )}
    <p className="text-muted-foreground text-xs leading-relaxed">{TMDB_NOTICE}</p>
  </aside>
);

export { TMDB_NOTICE, TmdbAttribution };
