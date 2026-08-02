import type { IncomingAskDTO } from "@loomarr/api";
import { formatDuration, pluralize } from "@loomarr/core";
import { Link } from "@tanstack/react-router";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { Badge, Button, Caption } from "@/components/ui";
import { cn } from "@/lib";
import type { IncomingPanelProps } from "./incoming-panel.type";

// IncomingPanel — what has been downloaded but is not yet filed (V35).
//
// Two halves of ONE read: clips whose tags need a human, and compilations mid-split.
//
// ⚠ **There is no confidence bar, and that is deliberate.** The mock draws one per row; the
// tagger records neither a score nor a rationale, so a bar here would be a number no code
// produced. Each row shows the REASON it is waiting instead, which the server derives from real
// state. When an auto-file threshold exists (`filler.autofile.*`) it will have a real score to
// render; until then this says what it knows.

// A clip whose era was guessed has a proposed answer to confirm; one with no tags at all has
// nothing to confirm. Keeping the two actions distinct is why `askReason` on the server keeps
// them distinct — one button on two questions is how an operator confirms a guess they never saw.
const AskRow = ({
  ask,
  busy,
  onConfirmEra,
  onEditTags,
  onDismiss,
}: {
  ask: IncomingAskDTO;
  busy: boolean;
} & Pick<IncomingPanelProps, "onConfirmEra" | "onEditTags" | "onDismiss">) => {
  const guessed = (ask.suggestedEra ?? 0) > 0;

  return (
    <li className="flex flex-wrap items-start gap-4 rounded-lg border border-border p-4">
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <div className="flex flex-wrap items-baseline gap-x-2">
          <span className="truncate font-medium text-sm">{ask.name}</span>
          <Caption className="tabular-nums">{formatDuration(ask.durationMs)}</Caption>
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
          {ask.category && <Badge variant="neutral">{ask.category}</Badge>}
        </div>

        <p className="text-muted-foreground text-xs">{ask.reason}</p>
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
  onConfirmEra,
  onEditTags,
  onDismiss,
  busyPath,
  className,
}: IncomingPanelProps) => {
  const nothingWaiting = asks.length === 0 && reels.length === 0;

  return (
    <div className={cn("flex flex-col gap-6", className)}>
      {nothingWaiting && (
        <EmptyState
          title="Nothing needs you"
          description="Everything downloaded so far has been filed. New clips show up here when Loomarr can't work out what they are."
        />
      )}

      {asks.length > 0 && (
        <section className="flex flex-col gap-3">
          <h2 className="font-medium text-sm">
            {pluralize(asks.length, "clip")} {asks.length === 1 ? "needs" : "need"} a decision
          </h2>
          <ul className="flex flex-col gap-2">
            {asks.map((ask) => (
              <AskRow
                key={ask.path}
                ask={ask}
                busy={busyPath === ask.path}
                {...(onConfirmEra ? { onConfirmEra } : {})}
                {...(onEditTags ? { onEditTags } : {})}
                {...(onDismiss ? { onDismiss } : {})}
              />
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
                  <span className="truncate font-medium text-sm">{reel.clipPath}</span>
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
