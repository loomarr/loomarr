import type { ConstraintMatches } from "@loomarr/api/models/constraintMatches";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import { Check, Pencil, X } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { episodeSelectionLabel } from "../episode-selection-label";
import type { ProposalReviewProps, ProposalStatus } from "./proposal-review.type";

// ProposalReview — the human-in-the-loop review that fronts the approval gate (§3,
// §8). It shows the grounded lineup (in-library), the acquisitions it
// wants (missing titles → the provisioner, but only on approve), ranked alternates,
// and the model's rationale + catalog evidence. Nothing here executes; approve/
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

const constraintSummary = (matches?: ConstraintMatches): string => {
  if (!matches) return "";
  const labels = [
    matches.request && "request",
    matches.tone && "tone",
    matches.era && "era",
    matches.mustInclude && "required terms",
    matches.mustExclude && "excluded terms",
    matches.refine && "refinement",
  ].filter((label): label is string => Boolean(label));
  return labels.length > 0 ? ` · matched ${labels.join(", ")}` : "";
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
        {episodeSelectionLabel(item) && <Badge variant="suggest">{episodeSelectionLabel(item)}</Badge>}
      </div>
      {item.rationale && <p className="mt-1 text-muted-foreground text-sm">{item.rationale}</p>}
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

const ProposalReview = ({
  proposal,
  outlook,
  status = "draft",
  busy = false,
  onApprove,
  onDeny,
  onEditItem,
  onEditRequest,
  className,
}: ProposalReviewProps) => {
  const s = STATUS[status];
  const actionable = status === "draft" || status === "submitted" || status === "partially-edited";
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
      </header>

      {outlook}

      <section aria-label="Request interpretation" className="flex flex-col gap-2 text-sm">
        {status === "partially-edited" ? (
          <p>These picks have changed. The original assessment no longer describes this lineup.</p>
        ) : proposal.scores?.version !== 1 ? (
          <p>
            This saved proposal has no current evidence assessment. Review the picks against your request.
          </p>
        ) : proposal.scores.theme.basis === "named_membership" ? (
          <p>The picks have source-backed membership in the named lineup you requested.</p>
        ) : proposal.scores.theme.basis === "none" ? (
          <p>No theme qualifiers to assess. Review the picks and any date requirements below.</p>
        ) : proposal.scores.theme.status !== "supported" ? (
          <p role="status" className="text-caution">
            This may be a narrow reading of your request. The catalog evidence supports only part of it, or is
            too sparse to assess it. Review the interpretation or edit your request before approving.
          </p>
        ) : (
          <p>
            Catalog metadata supports the requested theme terms. Review the picks to judge the overall match.
          </p>
        )}
        {actionable && onEditRequest && (
          <Button variant="outline" size="sm" className="w-fit" onClick={onEditRequest} disabled={busy}>
            Edit request
          </Button>
        )}
        {status !== "partially-edited" && proposal.scores?.version === 1 && (
          <details>
            <summary className="cursor-pointer text-muted-foreground">
              Evidence behind this interpretation
            </summary>
            <div className="mt-2 flex flex-col gap-2 text-muted-foreground">
              {proposal.scores.theme.basis === "qualifiers" && (
                <>
                  <p>Term coverage is a catalog check, not a probability of understanding your request.</p>
                  <ul className="list-inside list-disc">
                    {(proposal.scores.theme.qualifiers ?? []).map((qualifier) => (
                      <li key={qualifier.term}>
                        “{qualifier.term}”: supported by {qualifier.supportedItems} of{" "}
                        {proposal.scores.theme.assessedItems} assessed titles
                      </li>
                    ))}
                  </ul>
                  {proposal.scores.theme.unknownItems > 0 && (
                    <p>{proposal.scores.theme.unknownItems} titles have too little metadata to assess.</p>
                  )}
                </>
              )}
              <p>
                {proposal.scores.era.status === "not_requested"
                  ? "No date requirement was requested."
                  : proposal.scores.era.status === "unassessed"
                    ? "Date adherence is not assessed: catalog years or resolved episode dates are missing."
                    : `${proposal.scores.era.matchingItems} of ${proposal.scores.era.assessedItems} titles with known applicable dates meet the requested date requirements.`}
              </p>
              <p>
                {lineup.length} titles are in the library; {acquisitions.length} need acquisition. Library
                presence alone does not establish playback readiness.
              </p>
            </div>
          </details>
        )}
      </section>

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

      {proposal.trace?.candidates?.length ? (
        <section aria-label="Decision trace" className="flex flex-col gap-1.5 border-border border-t pt-3">
          <h3 className="font-mono text-static-400 text-xs uppercase tracking-wide">Why this / why not</h3>
          <ul className="flex flex-col gap-1 text-muted-foreground text-sm">
            {proposal.trace.candidates.map((candidate) => (
              <li key={`${candidate.key}-${candidate.disposition}`}>
                <span className="font-medium text-foreground">
                  {candidate.name || candidate.key || "Candidate"}
                </span>{" "}
                <span>
                  {candidate.disposition === "selected" ? "selected" : candidate.reason.replaceAll("_", " ")}
                  {constraintSummary(candidate.constraints)}
                </span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {actionable && (
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
                disabled={busy}
              />
              <div className="flex justify-end gap-2">
                <Button variant="ghost" onClick={cancelDeny} disabled={busy}>
                  Cancel
                </Button>
                <Button
                  variant="outline"
                  onClick={() => onDeny?.(reason.trim() || undefined)}
                  disabled={busy}
                >
                  <X aria-hidden />
                  Deny
                </Button>
              </div>
            </div>
          ) : (
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => setDenying(true)} disabled={busy}>
                <X aria-hidden />
                Deny
              </Button>
              <Button onClick={onApprove} disabled={busy}>
                <Check aria-hidden />
                Approve & acquire
              </Button>
            </div>
          )}
        </footer>
      )}
    </Card>
  );
};

export { ProposalReview };
