import type { ConstraintMatches } from "@loomarr/api/models/constraintMatches";
import { provisionKey } from "@loomarr/core/provision";
import { Check, X } from "lucide-react";
import { useId, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { ProposalEdit } from "../proposal-edit";
import type { ProposalReviewProps, ProposalStatus } from "./proposal-review.type";

const STATUS: Partial<
  Record<ProposalStatus, { label: string; variant: "neutral" | "suggest" | "lock" | "onair" }>
> = {
  submitted: { label: "Sent for approval", variant: "suggest" },
  approved: { label: "Created", variant: "lock" },
  denied: { label: "Not approved", variant: "onair" },
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

const ProposalReview = ({
  proposal,
  outlook,
  edit,
  status = "draft",
  selfService = false,
  busy = false,
  onApprove,
  onDeny,
  onEdit,
  onEditRequest,
  className,
}: ProposalReviewProps) => {
  const actionable = status === "draft" || status === "submitted" || status === "partially-edited";
  const [denying, setDenying] = useState(false);
  const [reason, setReason] = useState("");
  const reasonId = useId();
  const lineup = proposal.lineup ?? [];
  const acquisitions = proposal.acquisitions ?? [];
  const alternates = proposal.alternates ?? [];
  const dropped = new Set(edit?.drop ?? []);
  const selectedReady = lineup.filter((item) => !dropped.has(provisionKey(item))).length;
  const selectedMissing =
    acquisitions.filter((item) => !dropped.has(provisionKey(item))).length + (edit?.add?.length ?? 0);
  const selectedCount = selectedReady + selectedMissing;
  const statusDisplay = selfService ? (status === "approved" ? STATUS.approved : undefined) : STATUS[status];
  const scoresCurrent = status !== "partially-edited" && proposal.scores?.version === 1;
  const partialTheme = scoresCurrent && proposal.scores?.theme.status !== "supported";

  return (
    <Card className={cn("mx-auto flex w-full max-w-4xl flex-col gap-5 p-5 sm:p-6", className)}>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="font-semibold text-xl">Review your channel</h2>
          <p className="mt-1 font-medium">{proposal.channelName || "Your new channel"}</p>
          {proposal.rationale && (
            <p className="mt-1 max-w-2xl text-muted-foreground text-sm">{proposal.rationale}</p>
          )}
        </div>
        {statusDisplay && <Badge variant={statusDisplay.variant}>{statusDisplay.label}</Badge>}
      </header>

      <div className="grid grid-cols-3 gap-2 rounded-lg bg-muted/45 p-3 text-sm">
        <div>
          <span className="block font-semibold text-lg">{selectedCount}</span>
          <span className="text-muted-foreground text-xs sm:text-sm">selected</span>
        </div>
        <div>
          <span className="block font-semibold text-lg">{selectedReady}</span>
          <span className="text-muted-foreground text-xs sm:text-sm">ready now</span>
        </div>
        <div>
          <span className="block font-semibold text-lg">{selectedMissing}</span>
          <span className="text-muted-foreground text-xs sm:text-sm">to add</span>
        </div>
      </div>

      {partialTheme && (
        <div role="status" className="rounded-md border border-caution/40 bg-caution/5 p-3 text-sm">
          <p className="font-medium">Double-check the fit</p>
          <p className="mt-1 text-muted-foreground">
            Loomarr could not confidently verify every part of your description. Review the titles below,
            remove any mismatches, or change the description.
          </p>
          {actionable && onEditRequest && (
            <Button variant="outline" size="sm" className="mt-2" onClick={onEditRequest} disabled={busy}>
              Change description
            </Button>
          )}
        </div>
      )}

      {outlook}

      <ProposalEdit
        lineup={lineup}
        acquisitions={acquisitions}
        alternates={alternates}
        onChange={onEdit}
        disabled={busy}
      />

      {actionable && onEditRequest && !partialTheme && (
        <Button variant="outline" size="sm" className="w-fit" onClick={onEditRequest} disabled={busy}>
          Change description
        </Button>
      )}

      {actionable && onApprove && (
        <footer className="flex flex-col gap-3 border-border border-t pt-4">
          <div>
            <p className="font-medium text-sm">
              {selectedCount === 0
                ? "Choose at least one title"
                : `Create “${proposal.channelName || "Your new channel"}” with ${selectedCount} ${selectedCount === 1 ? "title" : "titles"}.`}
            </p>
            {selectedCount > 0 && (
              <p className="mt-0.5 text-muted-foreground text-sm">
                {selectedMissing > 0
                  ? `Loomarr will create the channel now and start adding ${selectedMissing} missing ${selectedMissing === 1 ? "title" : "titles"}.`
                  : "Everything selected is already in your library."}
              </p>
            )}
          </div>

          {denying && !selfService ? (
            <div className="flex flex-col gap-2">
              <label className="text-muted-foreground text-sm" htmlFor={reasonId}>
                Why not? Optional: the requester sees this.
              </label>
              <Input
                id={reasonId}
                autoFocus
                value={reason}
                onChange={(event) => setReason(event.target.value)}
                placeholder="For example, ask for fewer new titles"
                disabled={busy}
              />
              <div className="flex flex-wrap justify-end gap-2">
                <Button variant="ghost" onClick={() => setDenying(false)} disabled={busy}>
                  Cancel
                </Button>
                <Button
                  variant="outline"
                  onClick={() => onDeny?.(reason.trim() || undefined)}
                  disabled={busy}
                >
                  <X aria-hidden /> Not approved
                </Button>
              </div>
            </div>
          ) : (
            <div className="flex flex-wrap justify-end gap-2">
              {onDeny && (
                <Button
                  variant="ghost"
                  onClick={() => (selfService ? onDeny() : setDenying(true))}
                  disabled={busy}
                >
                  {selfService ? "Discard" : "Not approved"}
                </Button>
              )}
              <Button onClick={onApprove} disabled={busy || selectedCount === 0}>
                <Check aria-hidden /> Create channel
              </Button>
            </div>
          )}
        </footer>
      )}

      <details className="rounded-md border border-border px-3 py-2.5 text-sm">
        <summary className="cursor-pointer font-medium">How Loomarr chose these titles</summary>
        <div className="mt-3 flex flex-col gap-3 text-muted-foreground">
          {status === "partially-edited" ? (
            <p>You changed the title list, so the original assessment no longer describes every pick.</p>
          ) : proposal.scores?.version !== 1 ? (
            <p>
              This saved channel draft has no current catalog assessment. Review each title before creating
              it.
            </p>
          ) : (
            <>
              <p>
                {proposal.scores.theme.basis === "named_membership"
                  ? "Catalog sources place these titles in the named lineup you requested."
                  : proposal.scores.theme.basis === "none"
                    ? "No specific theme terms needed checking."
                    : "Loomarr checked your theme words against catalog metadata. This is supporting evidence, not a guarantee."}
              </p>
              {proposal.scores.theme.basis === "qualifiers" && (
                <ul className="list-inside list-disc">
                  {(proposal.scores.theme.qualifiers ?? []).map((qualifier) => (
                    <li key={qualifier.term}>
                      “{qualifier.term}” matched {qualifier.supportedItems} of{" "}
                      {proposal.scores.theme.assessedItems} checked titles
                    </li>
                  ))}
                </ul>
              )}
              <p>
                {proposal.scores.era.status === "not_requested"
                  ? "You did not ask for a particular date range."
                  : proposal.scores.era.status === "unassessed"
                    ? "Loomarr could not check the date range because some catalog dates were missing."
                    : `${proposal.scores.era.matchingItems} of ${proposal.scores.era.assessedItems} titles with known dates matched your requested range.`}
              </p>
            </>
          )}

          {proposal.trace?.candidates?.length ? (
            <details>
              <summary className="cursor-pointer">See the catalog decisions</summary>
              <ul className="mt-2 flex flex-col gap-1">
                {proposal.trace.candidates.map((candidate) => (
                  <li key={`${candidate.key}-${candidate.disposition}`}>
                    <span className="font-medium text-foreground">
                      {candidate.name || candidate.key || "Title"}
                    </span>{" "}
                    {candidate.disposition === "selected"
                      ? "included"
                      : candidate.reason.replaceAll("_", " ")}
                    {constraintSummary(candidate.constraints)}
                  </li>
                ))}
              </ul>
            </details>
          ) : null}
        </div>
      </details>
    </Card>
  );
};

export { ProposalReview };
