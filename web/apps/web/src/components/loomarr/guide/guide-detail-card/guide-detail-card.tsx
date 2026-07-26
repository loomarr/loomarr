import type { PodPoolDTOMatchLevel } from "@loomarr/api";
import { Badge } from "@/components/ui";
import { cn } from "@/lib";
import type { GuideDetailCardProps } from "./guide-detail-card.type";

// GuideDetailCard — what a guide block actually IS (§12, the v2 mock's hover card).
//
// Two shapes, because a break and a programme prompt different questions:
//
//   a programme → "what is this, and is it really going to play?"  → metadata + provenance
//   a break     → "why are my commercials these ones?"             → the clips + the ladder
//
// The break case is the one that could not exist before V13b: the API had only a channel-wide
// pool, so nothing could say what plays in THIS break at THIS time.

// How far down the §10 fallback ladder assembly went. `exact` is the quiet case and says
// nothing; every other level explains why the break is not era-perfect, which is the answer to
// "why are my commercials wrong".
const MATCH: Record<PodPoolDTOMatchLevel, { label: string; variant: "caution" | "neutral" } | null> = {
  exact: null,
  widened: { label: "Era widened", variant: "caution" },
  audience: { label: "Any era", variant: "caution" },
  bumper_card: { label: "Bumper only", variant: "neutral" },
};

// Why the ladder ended where it did, in words rather than a level name. The mock states this
// outright, and it is the difference between "WIDENED" as a mystery token and as an
// explanation.
const MATCH_WHY: Record<PodPoolDTOMatchLevel, string> = {
  exact: "Exact match on this channel's era and audience.",
  widened: "No exact match, so the ladder widened the era to fill the break.",
  audience: "No era-appropriate clips, so the ladder fell back to any era.",
  bumper_card: "Nothing in the catalog matched, so the break plays the bumper card rather than dead air.",
};

const CLIP_DOT: Record<string, string> = {
  commercial: "bg-signal",
  bumper: "bg-tune",
  station_id: "bg-suggest-300",
  psa: "bg-signal-400",
  trailer: "bg-signal-400",
  interstitial: "bg-static-500",
};

const fmtClock = (ms: number) =>
  new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" }).format(new Date(ms));

// Durations read as a runtime ("1h 47m", "22m"), not as a decimal count of minutes.
const fmtRuntime = (ms: number): string => {
  const total = Math.round(ms / 60_000);
  const h = Math.floor(total / 60);
  const m = total % 60;
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
};

const fmtClip = (ms: number): string => {
  const total = Math.round(ms / 1000);
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, "0")}`;
};

const GuideDetailCard = ({ airing, className }: GuideDetailCardProps) => {
  if (!airing) return null;

  const when = `${fmtClock(airing.startMs)}–${fmtClock(airing.stopMs)}`;
  const pod = airing.pod;
  const isBreak = airing.kind === "filler" && pod;
  const chip = pod ? MATCH[pod.matchLevel] : null;

  // A programme's second line: the facts that identify it, minus whatever this item lacks.
  const meta = [
    airing.year ? String(airing.year) : null,
    airing.rating || null,
    airing.genres?.length ? airing.genres.slice(0, 2).join(" · ") : null,
    airing.runtimeMs ? fmtRuntime(airing.runtimeMs) : null,
  ]
    .filter(Boolean)
    .join(" · ");

  const episode =
    airing.season && airing.episode
      ? `S${String(airing.season).padStart(2, "0")}E${String(airing.episode).padStart(2, "0")}`
      : null;

  return (
    <aside
      data-testid="guide-detail-card"
      className={cn("w-80 rounded-lg border border-border bg-static-900 p-3 shadow-lg", className)}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          {airing.series && <div className="truncate text-static-400 text-xs">{airing.series}</div>}
          <div className="truncate font-medium text-sm">{airing.title || "Commercials"}</div>
        </div>
        {/* State, not decoration: a pending slot is the case a viewer most needs named. */}
        <Badge variant={airing.kind === "pending" ? "caution" : "neutral"}>
          {airing.kind === "pending" ? "Pending slot" : airing.kind === "filler" ? "Break" : "Scheduled"}
        </Badge>
      </div>

      <div className="mt-1 font-mono text-static-400 text-xs">
        {when}
        {episode ? ` · ${episode}` : ""}
      </div>

      {isBreak ? (
        <div className="mt-3">
          <div className="flex items-center justify-between gap-2">
            <span className="font-mono text-[10px] text-static-400 uppercase tracking-wide">
              {`Pod · ${fmtClip(pod.totalMs)}`}
            </span>
            {chip && <Badge variant={chip.variant}>{chip.label}</Badge>}
          </div>
          <ul className="mt-2 flex flex-col gap-1">
            {(pod.entries ?? []).map((c, i) => (
              // Position is identity inside a pod: one clip may legitimately appear twice,
              // and the embedded fallback card has no path at all.
              // biome-ignore lint/suspicious/noArrayIndexKey: position is identity in a pod
              <li key={`${c.path ?? "card"}-${i}`} className="flex items-center gap-2 text-xs">
                <span className={cn("size-1.5 shrink-0 rounded-full", CLIP_DOT[c.kind] ?? "bg-static-500")} />
                <span className="min-w-0 flex-1 truncate">{c.name}</span>
                {/* Era and quality explain a grainy advert as authentic rather than broken. */}
                {c.era ? (
                  <span className="shrink-0 font-mono text-[10px] text-static-400">{c.era}</span>
                ) : null}
                {c.quality ? (
                  <span className="shrink-0 font-mono text-[10px] text-static-400">{c.quality}</span>
                ) : null}
                <span className="shrink-0 font-mono text-[10px] text-static-400">
                  {fmtClip(c.durationMs)}
                </span>
              </li>
            ))}
          </ul>
          <p className="mt-2 text-[11px] text-muted-foreground leading-snug">{MATCH_WHY[pod.matchLevel]}</p>
        </div>
      ) : (
        <>
          {meta && <div className="mt-2 text-static-400 text-xs">{meta}</div>}
          {airing.description && (
            <p className="mt-2 line-clamp-4 text-[11px] text-muted-foreground leading-snug">
              {airing.description}
            </p>
          )}
        </>
      )}

      {/* Provenance last: it is the "is this real yet?" line, and it applies to both shapes —
          a break has none, a pending slot leans on it entirely. */}
      {airing.provenance && (
        <div className="mt-3 border-border/60 border-t pt-2 font-mono text-[10px] text-static-400">
          {airing.provenance}
        </div>
      )}
    </aside>
  );
};

export { GuideDetailCard };
