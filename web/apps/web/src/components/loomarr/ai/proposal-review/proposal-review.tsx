import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import { formatPercent } from "@loomarr/core/format";
import { Check, Loader2, Pencil, ShieldAlert, X } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { ProposalReviewProps, ProposalStatus } from "./proposal-review.type";

// ProposalReview — the human-in-the-loop review that fronts the approval gate (§3,
// §8). It shows the grounded lineup (in-library, ready now), the acquisitions it
// wants (missing titles → the provisioner, but only on approve), ranked alternates,
// and the model's rationale + deterministic scores. Nothing here executes; approve/
// deny IS the gate (§7). Each row is editable-via-search (onEditItem) so a pick can
// be swapped for a grounded alternative without regenerating the whole proposal.
const STATUS: Record<
  ProposalStatus,
  { label: string; variant: "neutral" | "suggest" | "lock" | "onair" | "caution" }
> = {
  draft: { label: "Draft", variant: "neutral" },
  submitted: { label: "Awaiting approval", variant: "suggest" },
  approved: { label: "Approved", variant: "lock" },
  denied: { label: "Denied", variant: "onair" },
  "partially-edited": { label: "Edited", variant: "caution" },
};

// seasonWindowLabel renders a series' airing season window (§8) as a human chip:
// "Seasons 1–10", a single bound as "From season 11" / "Through season 10", or null
// when unbounded (0/0 = all seasons — no chip). Lets a reviewer SEE that "classic
// Simpsons" is scoped before approving, instead of the window being invisible.
const seasonWindowLabel = (min?: number, max?: number): string | null => {
  const lo = min ?? 0;
  const hi = max ?? 0;
  if (lo <= 0 && hi <= 0) return null;
  if (lo > 0 && hi > 0) return lo === hi ? `Season ${lo}` : `Seasons ${lo}–${hi}`;
  if (lo > 0) return `From season ${lo}`;
  return `Through season ${hi}`;
};

const ItemRow = ({
  item,
  kind,
  onEdit,
}: {
  item: ProposalItem;
  kind: "lineup" | "acquire";
  onEdit?: (item: ProposalItem) => void;
}) => (
  <li className="flex items-start gap-3 rounded-md border border-border bg-card px-3 py-2.5">
    <div className="min-w-0 flex-1">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium text-sm">{item.name}</span>
        {item.year ? <span className="font-mono text-static-400 text-xs">{item.year}</span> : null}
        <Badge variant={kind === "lineup" ? "lock" : "tune"}>
          {kind === "lineup" ? "In library" : "Will acquire"}
        </Badge>
        {seasonWindowLabel(item.seasonMin, item.seasonMax) && (
          <Badge variant="tune">{seasonWindowLabel(item.seasonMin, item.seasonMax)}</Badge>
        )}
        {typeof item.confidence === "number" && (
          <span className="font-mono text-static-400 text-xs">{`${formatPercent(item.confidence)} fit`}</span>
        )}
      </div>
      {item.rationale && (
        <details className="mt-1">
          <summary className="cursor-pointer list-none text-muted-foreground text-xs hover:text-foreground">
            {`Why ${item.name} fits`}
          </summary>
          <p className="mt-1 text-muted-foreground text-sm">{item.rationale}</p>
        </details>
      )}
    </div>
    {onEdit && (
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant="ghost"
              size="icon"
              className="size-7 shrink-0"
              aria-label={`Edit ${item.name}`}
              onClick={() => onEdit(item)}
            />
          }
        >
          <Pencil aria-hidden />
        </TooltipTrigger>
        <TooltipContent>Edit pick</TooltipContent>
      </Tooltip>
    )}
  </li>
);

const Section = ({ title, count, children }: { title: string; count: number; children: ReactNode }) =>
  count === 0 ? null : (
    <section className="flex flex-col gap-2">
      <h3 className="font-mono text-static-400 text-xs uppercase tracking-wide">{`${title} · ${count}`}</h3>
      <ul className="flex flex-col gap-1.5">{children}</ul>
    </section>
  );

const rangeLabel = (range?: { from?: number; to?: number }): string | null => {
  if (!range?.from && !range?.to) return null;
  if (range.from && range.to) return range.from === range.to ? `${range.from}` : `${range.from}–${range.to}`;
  return range.from ? `From ${range.from}` : `Through ${range.to}`;
};

const sentenceCase = (value: string): string =>
  value.replaceAll("_", " ").replace(/\b\w/g, (letter) => letter.toUpperCase());

const durationLabel = (value: string): string => {
  const match = value.match(/^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/);
  if (!match) return value;
  const hours = Number(match[1] ?? 0);
  const minutes = Number(match[2] ?? 0);
  const seconds = Number(match[3] ?? 0);
  if (minutes === 0 && seconds === 0 && hours > 0) {
    if (hours % 24 === 0) {
      const days = hours / 24;
      return `${days} ${days === 1 ? "day" : "days"}`;
    }
    return `${hours} ${hours === 1 ? "hour" : "hours"}`;
  }
  return value;
};

// policyFacts renders only policy the Suggester actually extracted. Omitting a field means
// "use the built-in default"; inventing a default label here would turn absence into a claim.
const policyFacts = (policy?: ChannelPolicy): string[] => {
  if (!policy) return [];
  const facts: string[] = [];
  const audience: string[] = [];
  if (policy.audience?.ceiling) audience.push(policy.audience.ceiling);
  if (policy.audience?.unrated) {
    audience.push(
      policy.audience.unrated === "exclude"
        ? "Unrated excluded"
        : policy.audience.unrated === "allow"
          ? "Unrated allowed"
          : `Unrated ${policy.audience.unrated}`,
    );
  }
  if (audience.length > 0) facts.push(`Audience · ${audience.join(" · ")}`);

  const scope: string[] = [];
  const era = rangeLabel(policy.scope?.era);
  if (era) scope.push(era);
  const seasons = rangeLabel(policy.scope?.seasons);
  if (seasons) scope.push(`Seasons ${seasons}`);
  if (policy.scope?.runtimeMax) scope.push(`Up to ${Math.round(policy.scope.runtimeMax / 60)} min`);
  if ((policy.scope?.genres?.include?.length ?? 0) > 0) {
    scope.push(`Genres ${policy.scope?.genres?.include?.join(", ")}`);
  }
  if ((policy.scope?.genres?.exclude?.length ?? 0) > 0) {
    scope.push(`Excludes ${policy.scope?.genres?.exclude?.join(", ")}`);
  }
  if ((policy.scope?.series?.length ?? 0) > 0) scope.push(`${policy.scope?.series?.length} series`);
  if ((policy.scope?.collections?.length ?? 0) > 0) {
    scope.push(`${policy.scope?.collections?.length} collections`);
  }
  if (scope.length > 0) facts.push(`Scope · ${scope.join(" · ")}`);

  if (policy.ordering) facts.push(`Ordering · ${sentenceCase(policy.ordering)}`);
  const separation: string[] = [];
  if (policy.separation?.movieNoRepeat) {
    separation.push(`Movies ${durationLabel(policy.separation.movieNoRepeat)}`);
  }
  if (policy.separation?.episodeNoRepeat) {
    separation.push(`Episodes ${durationLabel(policy.separation.episodeNoRepeat)}`);
  }
  if (policy.separation?.seriesMinGap) {
    separation.push(`Series ${durationLabel(policy.separation.seriesMinGap)}`);
  }
  if (policy.separation?.blockMax) separation.push(`Block limit ${policy.separation.blockMax}`);
  if (separation.length > 0) facts.push(`Separation · ${separation.join(" · ")}`);
  if (policy.seasonal?.mode) facts.push(`Seasonal · ${sentenceCase(policy.seasonal.mode)}`);
  if ((policy.rules?.length ?? 0) > 0) facts.push(`Scheduling rules · ${policy.rules?.length}`);
  return facts;
};

const refusalReason = (item: ProposalItem, reason: string, policy?: ChannelPolicy): string => {
  if (reason === "over_ceiling") {
    const rating = item.officialRating ? `Rated ${item.officialRating}` : "Rating unknown";
    const limit = policy?.audience?.ceiling
      ? `the ${policy.audience.ceiling} audience limit`
      : "this channel's audience limit";
    return `${rating}, above ${limit}.`;
  }
  if (reason === "unrated") return "No usable rating under this channel's audience limit.";
  if (reason === "out_of_scope") return "Outside this channel's scope.";
  if (reason === "out_of_season") return "Outside the active seasonal window.";
  return `Excluded by this channel's ${reason.replaceAll("_", " ")} policy.`;
};

const ProposalReview = ({
  proposal,
  status = "draft",
  busy = false,
  approving = false,
  onApprove,
  onDeny,
  onEditItem,
  className,
}: ProposalReviewProps) => {
  const s = STATUS[status];
  const actionable = status === "draft" || status === "submitted" || status === "partially-edited";
  const canDecide = actionable && (onApprove != null || onDeny != null);
  const decisionBusy = busy || approving;
  // Deny is a two-step: click arms a reason field, a second click sends it. Approve stays
  // one click — the asymmetry is deliberate, since approving needs no explanation and
  // denying is the case where a member is left guessing.
  const [denying, setDenying] = useState(false);
  const [reason, setReason] = useState("");
  const reasonId = useId();
  const cancelDeny = () => {
    setDenying(false);
    setReason("");
  };
  // The generated arrays are `ProposalItem[] | null` (a nil Go slice marshals as null).
  const lineup = proposal.lineup ?? [];
  const acquisitions = proposal.acquisitions ?? [];
  const alternates = proposal.alternates ?? [];
  const facts = policyFacts(proposal.policy);
  const refused = proposal.refused ?? [];
  return (
    <Card className={cn("flex flex-col gap-5 p-5", className)}>
      <header className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            {/* The LLM proposes a channel NAME (§8) and it becomes the channel's name
                on approve — but the heading said "Proposed lineup" regardless, so the
                one thing the reviewer is about to commit to was invisible until after
                they'd committed to it. */}
            <h2 className="font-semibold text-lg">{proposal.channelName || "Proposed lineup"}</h2>
            <Badge variant={s.variant}>{s.label}</Badge>
          </div>
          {proposal.channelName && (
            <p className="mt-0.5 text-static-400 text-xs">Proposed channel name, editable after approval.</p>
          )}
          {proposal.rationale && (
            <p className="mt-1 max-w-prose text-muted-foreground text-sm">{proposal.rationale}</p>
          )}
        </div>
        {/* All four scores, not two. `overall` is the weighted composite that RANKS
            proposals (0.5 theme + 0.35 ready-now + 0.15 era spread) — showing its
            inputs while hiding the result left a reviewer able to see the parts but
            not the conclusion. It leads, with the three components under it. */}
        {proposal.scores && (
          <dl className="shrink-0 text-right">
            <div className="flex items-baseline justify-end gap-1.5">
              <dt className="text-static-400 text-xs">Overall</dt>
              <dd className="font-mono font-semibold text-base text-foreground">
                {formatPercent(proposal.scores.overall)}
              </dd>
            </div>
            <div className="flex items-baseline justify-end gap-1.5">
              <dt className="text-static-400 text-xs">Theme fit</dt>
              <dd className="font-mono text-sm text-suggest-300">
                {formatPercent(proposal.scores.themeFit)}
              </dd>
            </div>
            <div className="flex items-baseline justify-end gap-1.5">
              <dt className="text-static-400 text-xs">Ready now</dt>
              <dd className="font-mono text-lock text-sm">
                {formatPercent(proposal.scores.availabilityRatio)}
              </dd>
            </div>
            <div className="flex items-baseline justify-end gap-1.5">
              <dt className="text-static-400 text-xs">Era spread</dt>
              <dd className="font-mono text-sm text-tune">{formatPercent(proposal.scores.eraBalance)}</dd>
            </div>
          </dl>
        )}
      </header>

      <div className="flex flex-wrap gap-2">
        <Badge variant="lock">{`${lineup.length} ready now`}</Badge>
        <Badge variant="tune">{`${acquisitions.length} to acquire`}</Badge>
      </div>

      {facts.length > 0 && (
        <section className="flex flex-col gap-1.5">
          <h3 className="font-mono text-static-400 text-xs uppercase tracking-wide">Extracted policy</h3>
          <ul className="flex flex-col gap-1 text-sm">
            {facts.map((fact) => (
              <li key={fact}>{fact}</li>
            ))}
          </ul>
        </section>
      )}

      {refused.length > 0 && (
        <section className="flex flex-col gap-1.5 rounded-md border border-signal/40 bg-signal/5 px-3 py-2">
          <h3 className="flex items-center gap-1.5 text-sm">
            <ShieldAlert className="size-4 shrink-0 text-signal" aria-hidden />
            <span className="font-medium">
              {`${refused.length} ${refused.length === 1 ? "title" : "titles"} won't be included`}
            </span>
          </h3>
          <ul className="flex flex-col gap-0.5">
            {refused.map(({ item, reason }) => (
              <li key={`${item.mediaType}-${item.tmdbId ?? item.tvdbId ?? item.name}`} className="text-sm">
                <span>{item.name}</span>{" "}
                <span className="text-muted-foreground">
                  — {refusalReason(item, reason, proposal.policy)}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}

      <Section title="Lineup" count={lineup.length}>
        {lineup.map((item) => (
          <ItemRow key={item.name} item={item} kind="lineup" onEdit={onEditItem} />
        ))}
      </Section>

      <Section title="Acquisitions" count={acquisitions.length}>
        {acquisitions.map((item) => (
          <ItemRow key={item.name} item={item} kind="acquire" onEdit={onEditItem} />
        ))}
      </Section>

      {alternates.length > 0 && (
        <section className="flex flex-col gap-1.5">
          <h3 className="font-mono text-static-400 text-xs uppercase tracking-wide">
            {`Alternates · ${alternates.length}`}
          </h3>
          <p className="text-muted-foreground text-sm">{alternates.map((a) => a.name).join(" · ")}</p>
        </section>
      )}

      {canDecide && (
        <footer className="flex flex-col gap-2 border-border border-t pt-4">
          {denying ? (
            /* Deny arms this instead of firing immediately: the requester sees whatever is
               typed here (ApprovalQueueItem already renders it), and a bare "denied" with no
               explanation is the thing that makes a member re-submit the same intent. */
            <div className="flex flex-col gap-2">
              <label className="text-muted-foreground text-sm" htmlFor={reasonId}>
                Why not? Optional: the requester sees this.
              </label>
              <Input
                id={reasonId}
                autoFocus
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                placeholder="e.g. we're over the acquisition cap this week, ask again Monday"
                disabled={decisionBusy}
              />
              <div className="flex justify-end gap-2">
                <Button variant="ghost" onClick={cancelDeny} disabled={decisionBusy}>
                  Cancel
                </Button>
                <Button
                  variant="outline"
                  onClick={() => onDeny?.(reason.trim() || undefined)}
                  disabled={decisionBusy}
                >
                  <X aria-hidden />
                  Deny
                </Button>
              </div>
            </div>
          ) : (
            <div className="flex justify-end gap-2">
              {onDeny && (
                <Button variant="outline" onClick={() => setDenying(true)} disabled={decisionBusy}>
                  <X aria-hidden />
                  Deny
                </Button>
              )}
              {onApprove && (
                <Button onClick={onApprove} disabled={decisionBusy} aria-busy={approving || undefined}>
                  {approving ? <Loader2 className="animate-spin" aria-hidden /> : <Check aria-hidden />}
                  {approving ? "Creating channel…" : "Approve & acquire"}
                </Button>
              )}
            </div>
          )}
        </footer>
      )}
    </Card>
  );
};

export { ProposalReview };
