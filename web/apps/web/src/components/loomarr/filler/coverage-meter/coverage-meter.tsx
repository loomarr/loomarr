import type { CoverageDTOLevel } from "@loomarr/api";
import { Link } from "@tanstack/react-router";
import { Caption, StatusDot, type StatusTone } from "@/components/ui";
import { cn } from "@/lib";
import type { CoverageMeterProps } from "./coverage-meter.type";

// CoverageMeter — which rung of the §10 fallback ladder this channel's breaks draw from (V29b).
//
// ⚠ **What this deliberately does NOT render.** The v2 mock draws catalog-composition shares
// as "Breaks resolve exactly N% of the time". That number is not computable from a catalog: how
// often a break resolves at a given rung depends on which breaks occur, and the mock's own JS
// invents it by recomputing buckets inline with five mutually inconsistent era/audience
// predicates. Building it as drawn ships a meter that disagrees with what airs, which is the
// exact bug §10's shared assembler exists to prevent.
//
// So this shows what the ladder actually answers: the rung a break WOULD be filled from, and how
// much material each rung holds. Everything here comes from `filler.Coverage`, which calls the
// same `candidatePools` that `Assemble` calls — the agreement is pinned by a Go test, not by
// this component agreeing to be careful.
const LEVEL_COPY: Record<CoverageDTOLevel, { tone: StatusTone; label: string; hint: string }> = {
  exact: {
    tone: "ok",
    label: "Exact match",
    hint: "Breaks fill with commercials from this channel's own era and audience.",
  },
  widened: {
    tone: "pending",
    label: "Widened to the decade",
    hint: "Nothing matches the exact year, so breaks draw from the same decade.",
  },
  audience: {
    tone: "warn",
    label: "Audience only",
    hint: "No era match at all — breaks draw from any commercial for this audience.",
  },
  bumper_card: {
    tone: "error",
    label: "No commercials",
    hint: "Nothing in the catalog fits, so breaks run on the built-in card alone.",
  },
};

// The degraded rendering for a level this build does not know — see the note in the component.
// Deliberately not alarming: an unrecognised rung is a version skew between server and client,
// not a broken catalog, and telling an operator their commercials are missing would send them
// to fix something that is fine.
const UNKNOWN_LEVEL = {
  tone: "off",
  label: "Coverage unavailable",
  hint: "This build doesn't recognise the coverage level the server reported.",
} as const;

// Rung labels, for the breakdown. Keyed by the same union the server sends, so a new rung is a
// compile error here rather than a blank row.
const RUNG_LABEL: Record<CoverageDTOLevel, string> = {
  exact: "Exact era + audience",
  widened: "Same decade",
  audience: "Any era, right audience",
  bumper_card: "Built-in card",
};

const CoverageMeter = ({ coverage, className }: CoverageMeterProps) => {
  // ⚠ Falls back rather than indexing blind. `LEVEL_COPY[level]` returning undefined crashed
  // the whole Filler section on `.tone` — found when this landed and five unrelated
  // ChannelFiller tests went red. The trigger there was a stub payload, but the real one is a
  // server that adds a ladder rung: the generated union updates only when someone regenerates,
  // and an unknown level must degrade to "we cannot say" rather than take the page down.
  const level = LEVEL_COPY[coverage.level] ?? UNKNOWN_LEVEL;
  // ⚠ `?? []` because huma types every Go slice as nullable, so the generated DTO says
  // `CoverageRungDTO[] | null` even though the handler always sends `[]`. The same is true of
  // PodPoolDTO.entries, which channel-filler unwraps the same way — following the existing
  // convention rather than diverging from it.
  const rungs = coverage.rungs ?? [];
  // The widest rung is the denominator: the rungs NEST, so the bar shows how much of the
  // available material each tighter rung captures. Guarded because a catalog with nothing in
  // it makes every rung 0.
  const widest = coverage.total || 0;

  // "Thin" is defined by the ladder's own answer, not by a number someone picked (F4).
  //
  // A channel is thin when its breaks CANNOT be filled from era- and audience-matched
  // commercials — i.e. the ladder had to widen, or gave up entirely. That is exactly the
  // condition an operator can fix by acquiring clips, and it is the condition the ladder
  // already computes, so the CTA appears for the same reason the level does.
  //
  // ⚠ Not "fewer than N clips". A channel with three perfectly-matched commercials is not
  // asking for help; one with two hundred clips that all miss its era is. A count threshold
  // gets both cases backwards.
  const isThin = coverage.level !== "exact";

  return (
    <section className={cn("flex flex-col gap-3 rounded-lg border border-border p-4", className)}>
      <div className="flex items-start gap-2">
        <StatusDot tone={level.tone} label="" className="mt-1.5" />
        <div className="min-w-0">
          <p className="font-medium text-sm">{level.label}</p>
          <p className="mt-0.5 text-muted-foreground text-xs">{level.hint}</p>
        </div>
      </div>

      {rungs.length > 0 && (
        <ul className="flex flex-col gap-1.5">
          {rungs.map((rung) => {
            // Percentage of the widest rung, NOT of some invented total — see the header.
            const share = widest > 0 ? Math.round((rung.clips / widest) * 100) : 0;
            return (
              <li key={rung.level} className="flex items-center gap-2">
                <span className="w-44 shrink-0 truncate text-xs">{RUNG_LABEL[rung.level] ?? rung.level}</span>
                <span className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-static-800">
                  <span
                    className={cn(
                      "block h-full rounded-full",
                      rung.level === coverage.level ? "bg-lock" : "bg-static-500",
                    )}
                    style={{ width: `${share}%` }}
                  />
                </span>
                {/* tabular-nums so the counts do not jitter as the catalog changes. */}
                <Caption className="w-10 shrink-0 text-right tabular-nums">{rung.clips}</Caption>
              </li>
            );
          })}
        </ul>
      )}

      {/* ⚠ Absent rungs are NOT drawn at zero. Under the strict-era setting there is no widened
          rung, and a 0 there reads as a catalog gap to go fix rather than a setting the
          operator chose — so the list simply omits it, and this line says why the total is
          what it is. */}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Caption>
          {widest === 0
            ? "No eligible commercials for this channel."
            : `${widest} eligible ${widest === 1 ? "commercial" : "commercials"}`}
        </Caption>

        {/* "Find clips" (F4 gap flagging, V17d) — shown only when the catalog is actually thin
            for THIS channel, because a CTA on a healthy channel is noise that teaches an
            operator to stop reading the banner.
            ⚠ It routes to the existing Filler catalog rather than starting an acquisition of
            its own. The page already has the ingest panel and the tagging tools; a second
            entry point that downloaded clips directly would be a parallel path to the same
            job, and the useful thing here is noticing the gap, not re-implementing the fix. */}
        {isThin && (
          <Link
            to="/filler"
            className="text-signal text-xs underline-offset-2 hover:underline"
            data-testid="find-clips"
          >
            Find clips →
          </Link>
        )}
      </div>
    </section>
  );
};

export { CoverageMeter };
