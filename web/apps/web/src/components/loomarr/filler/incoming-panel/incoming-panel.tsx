import type { IncomingAskDTO } from "@loomarr/api";
import { formatClipDuration, pluralize } from "@loomarr/core";
import { Link } from "@tanstack/react-router";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { Badge, Button, Caption } from "@/components/ui";
import { cn } from "@/lib";
import type { IncomingPanelProps } from "./incoming-panel.type";

// IncomingPanel — what has been downloaded but is not yet filed (V35).
//
// Two halves of ONE read: clips whose tags need a human, and compilations mid-split.
//
// ⚠ **The "no confidence bar" rule is RETIRED (V38), not weakened.** It stood for two phases and
// was correct throughout: the mock drew a bar, the tagger measured nothing, and rendering a number
// no code produced would have been a lie about how sure Loomarr was. V38 built what the rule was
// waiting for — a GROUNDING-CAPPED score, where verifiable facts about the clip's own text set a
// ceiling the model may only lower. So the bar is real now.
//
// ⚠ The reason line STAYS beside it. A number says how sure; only the sentence says why, and an
// operator deciding whether to trust a 45 needs the "the year isn't written anywhere" part.

// ConfidenceMeter renders the grounding-capped score (V38).
//
// ⚠ The colour is a THRESHOLD read, not a gradient: the operator's real question is "would this
// have filed itself?", so the bands mirror the settable range (50-95). A continuous ramp would
// look precise about a number whose precision is not the point.
//
// ⚠ Two lint rules pushed this shape, in sequence, and both were right: `aria-label` on a bare
// <span> is silently ignored (ARIA only names elements that have a role), and once a role was
// added, `useSemanticElements` asked why the native element was not used instead.
//
// ⚠ **A styled div carrying `role="meter"`, not a native `<meter>`** — the native element cannot
// render a themed bar. Its fill lives in `::-webkit-meter-*` pseudo-elements that Tailwind classes
// cannot reach, `appearance-none` strips the platform rendering without supplying a replacement,
// and a `<meter>`'s children are FALLBACK content that supporting browsers ignore. Measured: with
// the native element the bar drew the browser's own green (142,193,81) rather than `lock`
// (61,214,140), i.e. our styling was not applying at all.
//
// The explicit role plus the aria-value trio carries exactly what the native element would have
// contributed to assistive tech, which is the half that matters.
//
// ⚠ The bar is DELIBERATELY 40px wide and 1px tall, so scores differ by only a few pixels — 17px
// at 40, 30px at 72, 38px at 92 (measured). It reads as a rough band, not a precise readout, which
// is the honest rendering of a number whose precision is not the point. The colour-coded numeral
// beside it is what an operator actually reads.
const ConfidenceMeter = ({ score }: { score: number }) => {
  const pct = Math.max(0, Math.min(100, score));
  const tone = score >= 85 ? "bg-lock" : score >= 60 ? "bg-signal" : "bg-onair-300";
  const text = score >= 85 ? "text-lock" : score >= 60 ? "text-signal" : "text-onair-300";
  return (
    <span className="flex items-center gap-1.5" title="How sure Loomarr is about these tags">
      {/* biome-ignore lint/a11y/useSemanticElements: a native <meter> cannot render a themed bar
          — see the note above; the role + aria-value trio carry the same semantics. */}
      <span
        role="meter"
        aria-valuenow={score}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={`Confidence ${score} out of 100`}
        className="block h-1 w-10 overflow-hidden rounded-full bg-static-800"
      >
        <span className={cn("block h-full rounded-full", tone)} style={{ width: `${pct}%` }} />
      </span>
      <span className={cn("font-mono text-2xs tabular-nums", text)}>{score}</span>
    </span>
  );
};

// A clip whose era was guessed has a proposed answer to confirm; one with no tags at all has
// nothing to confirm. Keeping the two actions distinct is why `askReason` on the server keeps
// them distinct — one button on two questions is how an operator confirms a guess they never saw.
const AskRow = ({
  ask,
  busy,
  onConfirmEra,
  onEditTags,
  onDismiss,
  onFile,
}: {
  ask: IncomingAskDTO;
  busy: boolean;
} & Pick<IncomingPanelProps, "onConfirmEra" | "onEditTags" | "onDismiss" | "onFile">) => {
  const guessed = (ask.suggestedEra ?? 0) > 0;

  return (
    <li className="flex flex-wrap items-start gap-4 rounded-lg border border-border p-4">
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <div className="flex flex-wrap items-baseline gap-x-2">
          <span className="truncate font-medium text-sm">{ask.name}</span>
          {/* ⚠ formatClipDuration, NOT formatDuration. The latter is minute-granular because it
              formats PROGRAMME runtimes; filler clips are 5-60 seconds, so every row in this
              queue rendered "0m". Caught by reading the baseline image — no test asserted on the
              duration text, and "0m" is a plausible-looking string. */}
          <Caption className="tabular-nums">{formatClipDuration(ask.durationMs)}</Caption>
        </div>

        {/* Where it came from, so an operator working through forty of these can tell which
            source is producing junk. */}
        {ask.from && <Caption>from {ask.from}</Caption>}

        <div className="flex flex-wrap items-center gap-1.5">
          {/* The guess is visually distinct from a confirmed tag: an operator scanning this
              list must be able to tell what Loomarr decided from what it is asking about. */}
          {guessed && <Badge variant="suggest">{`guessed ${ask.suggestedEra}`}</Badge>}
          {ask.era ? <Badge variant="neutral">{String(ask.era)}</Badge> : null}
          {ask.audience && <Badge variant="neutral">{ask.audience}</Badge>}
          {/* `IncomingAskDTO.category` is the same DERIVED primary-product-leaf shadow as
              `ClipDTO.category` (§10 V45a) — this queue's summary DTO carries no `tags` array
              of its own, so the shadow is the only tag signal there is to show, read-only. */}
          {ask.category && <Badge variant="neutral">{ask.category}</Badge>}
        </div>

        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          {/* 0 means never scored — a clip the tagger has not reached, or one catalogued before
              V38. A 0-width bar would read as "no confidence" rather than "no measurement",
              which is a different and false claim. */}
          {(ask.confidence ?? 0) > 0 && <ConfidenceMeter score={ask.confidence ?? 0} />}
          <p className="text-muted-foreground text-xs">{ask.reason}</p>
        </div>
      </div>

      <div className="flex shrink-0 flex-wrap gap-2">
        {guessed && onConfirmEra && (
          <Button size="sm" disabled={busy} onClick={() => onConfirmEra(ask)}>
            Looks right
          </Button>
        )}
        {onEditTags && (
          <Button variant="outline" size="sm" disabled={busy} onClick={() => onEditTags(ask)}>
            {guessed ? "Not right" : "Add tags"}
          </Button>
        )}
        {/* File it as it stands — the tags are right enough, whatever the score said. Distinct
            from "Looks right", which CONFIRMS a guessed era before filing. */}
        {onFile && !guessed && (
          <Button variant="outline" size="sm" disabled={busy} onClick={() => onFile(ask)}>
            Use it
          </Button>
        )}
        {onDismiss && (
          <Button variant="ghost" size="sm" disabled={busy} onClick={() => onDismiss(ask)}>
            Don't use it
          </Button>
        )}
      </div>
    </li>
  );
};

const IncomingPanel = ({
  asks,
  reels,
  recentlyFiled,
  onConfirmEra,
  onEditTags,
  onDismiss,
  onFile,
  onFileAllAsSuggested,
  onSendBack,
  busyPath,
  className,
}: IncomingPanelProps) => {
  const nothingWaiting = asks.length === 0 && reels.length === 0;
  // "File all as suggested" only means something when something HAS a suggestion — otherwise it
  // is a button that files clips as whatever they already are, which "Use it" already does per
  // row and which nobody would expect from that label.
  const anyGuessed = asks.some((a) => (a.suggestedEra ?? 0) > 0);

  return (
    <div className={cn("flex flex-col gap-6", className)}>
      {/* ⚠ Two claims were removed from this empty state, both stale (V38b).
          "Everything downloaded so far has been filed" described activity that never happened on
          an install with no downloader — and it sat directly above a panel saying downloading was
          unavailable, so the page contradicted itself.
          "when Loomarr can't work out what they are" described the pre-V38 heuristic, where the
          queue was INFERRED from missing tags. Holding is a STATE now: a clip waits because it
          was downloaded, including ones Loomarr understood perfectly. */}
      {nothingWaiting && (
        <EmptyState
          title="Nothing needs you"
          description="Nothing is waiting to be checked. Clips you download land here first, before they start playing."
        />
      )}

      {asks.length > 0 && (
        <section className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-3">
            <h2 className="font-medium text-sm">
              {pluralize(asks.length, "clip")} {asks.length === 1 ? "needs" : "need"} a decision
            </h2>
            {onFileAllAsSuggested && anyGuessed && (
              <Button
                variant="outline"
                size="sm"
                className="ml-auto"
                onClick={onFileAllAsSuggested}
                title="File each of these, confirming the era Loomarr proposed for it"
              >
                File all as suggested
              </Button>
            )}
          </div>
          <ul className="flex flex-col gap-2">
            {asks.map((ask) => (
              <AskRow
                key={ask.path}
                ask={ask}
                busy={busyPath === ask.path}
                {...(onConfirmEra ? { onConfirmEra } : {})}
                {...(onEditTags ? { onEditTags } : {})}
                {...(onDismiss ? { onDismiss } : {})}
                {...(onFile ? { onFile } : {})}
              />
            ))}
          </ul>
        </section>
      )}

      {/* The audit half (§10 V38): what was filed with nobody looking. ⚠ It renders even when the
          queue above is empty — "nothing needs you" and "here is what I did without asking" are
          different statements, and an operator who did not expect auto-filing needs the second
          one most on exactly the install where the first is true. */}
      {recentlyFiled && recentlyFiled.length > 0 && (
        <section className="flex flex-col gap-3">
          <h2 className="font-medium text-sm">
            Loomarr filed {pluralize(recentlyFiled.length, "clip")} without asking
          </h2>
          <Caption>
            These are already playing. Least-confident first — send any of them back if they look wrong.
          </Caption>
          <ul className="flex flex-col gap-2">
            {recentlyFiled.map((clip) => (
              <li
                key={clip.path}
                className="flex flex-wrap items-center gap-4 rounded-lg border border-border p-4"
              >
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <span className="truncate font-medium text-sm">{clip.name}</span>
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                    {(clip.confidence ?? 0) > 0 && <ConfidenceMeter score={clip.confidence ?? 0} />}
                    <p className="text-muted-foreground text-xs">{clip.reason}</p>
                  </div>
                </div>
                {onSendBack && (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={busyPath === clip.path}
                    onClick={() => onSendBack(clip)}
                    title="Take it out of rotation and put it back in the queue. The file stays."
                  >
                    Send it back
                  </Button>
                )}
              </li>
            ))}
          </ul>
        </section>
      )}

      {reels.length > 0 && (
        <section className="flex flex-col gap-3">
          <h2 className="font-medium text-sm">{pluralize(reels.length, "compilation")} to review</h2>
          <ul className="flex flex-col gap-2">
            {reels.map((reel) => (
              <li
                key={reel.proposalId}
                className="flex flex-wrap items-center gap-4 rounded-lg border border-border p-4"
              >
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  {/* The compilation's NAME, not its identity — a 64-character hash is not a row
                      title. The server falls back to the hash when the clip is gone, so a
                      deleted compilation still renders as something an operator can act on. */}
                  <span className="truncate font-medium text-sm">{reel.clipName}</span>
                  <Caption>
                    {pluralize(reel.segments, "clip")} found
                    {/* A reel of twelve clean segments and one with three problems are
                        different amounts of work, and the queue says which before it is
                        opened rather than after. */}
                    {reel.needsAttention > 0 && ` · ${reel.needsAttention} need a look`}
                  </Caption>
                </div>
                {/* ⚠ Links to the SIBLING route, not a nested one. The catalog page renders no
                    <Outlet/>, so nesting the review under /filler would make the whole surface
                    unreachable while every unit test stayed green (PROGRESS.md records the
                    near-miss). This tab is an additional door to that route, not a replacement. */}
                <Button asChild variant="outline" size="sm">
                  <Link to="/filler/splits/$proposalId" params={{ proposalId: reel.proposalId }}>
                    Review cuts
                  </Link>
                </Button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
};

export { IncomingPanel };
