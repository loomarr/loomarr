import { provisionKey } from "@loomarr/core/provision";
import { Check, X } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { friendlyCandidateName, friendlyProposalRationale } from "@/suggest/suggestion-language";
import { ProposalEdit } from "../proposal-edit";
import { ProposalOutlook, ProposalOutlookDetails, ProposalOutlookDiagnostics } from "../proposal-outlook";
import type { ProposalReviewProps, ProposalStatus } from "./proposal-review.type";

const STATUS: Partial<
  Record<ProposalStatus, { label: string; variant: "neutral" | "suggest" | "lock" | "onair" }>
> = {
  submitted: { label: "Sent for approval", variant: "suggest" },
  approved: { label: "Created", variant: "lock" },
  denied: { label: "Not approved", variant: "onair" },
  superseded: { label: "Replaced", variant: "neutral" },
};

const catalogDecision = (disposition: string, reason: string) => {
  if (disposition === "selected") return "Included in the original suggestions";
  if (disposition === "alternate") return "Kept as a backup";
  const reasons: Record<string, string> = {
    not_selected: "Considered, but not chosen",
    not_surfaced: "Loomarr couldn't confirm this title",
    no_relevance_evidence: "Loomarr couldn't confirm that it fits your request",
    malformed_id: "Could not identify this title reliably",
    over_ceiling: "Outside the audience limit",
    never: "Excluded by your preferences",
    validation_dropped: "Did not pass the final title checks",
  };
  return reasons[reason] ?? "Not included in the suggestions";
};

const availabilitySummary = (selected: number, ready: number, missing: number) => {
  const titles = `${selected} ${selected === 1 ? "title" : "titles"}`;
  if (selected === 0) return "No titles selected";
  if (missing === 0) return `${titles} · all in your library`;
  if (ready === 0) return `${titles} · ${missing} will be added`;
  return `${titles} · ${ready} in your library · ${missing} will be added`;
};

const selectedLineupContext = (
  lineup: ProposalReviewProps["proposal"]["lineup"],
  acquisitions: ProposalReviewProps["proposal"]["acquisitions"],
  edit: ProposalReviewProps["edit"],
) => {
  const dropped = new Set(edit?.drop ?? []);
  const selected = [
    ...lineup.filter((item) => !dropped.has(provisionKey(item))),
    ...acquisitions.filter((item) => !dropped.has(provisionKey(item))),
    ...(edit?.add ?? []),
  ];
  const seen = new Set<string>();

  return selected.flatMap((item) => {
    const key = provisionKey(item);
    const identity = key || `${item.mediaType}:${item.name.toLocaleLowerCase()}:${item.year ?? ""}`;
    if (seen.has(identity)) return [];
    seen.add(identity);
    return [
      {
        name: item.name,
        ...(item.year ? { year: item.year } : {}),
        ...(key ? { key } : {}),
      },
    ];
  });
};

const ProposalReview = ({
  proposal,
  optionalSuggestionKeys,
  assessment,
  assessmentPending = false,
  edit,
  status = "draft",
  selfService = false,
  busy = false,
  revising = false,
  revisionError,
  showWorkflowHeading = true,
  onApprove,
  onDeny,
  onEdit,
  onRevise,
  className,
}: ProposalReviewProps) => {
  const actionable = status === "draft" || status === "submitted" || status === "partially-edited";
  const [denying, setDenying] = useState(false);
  const [editingBrief, setEditingBrief] = useState(false);
  const [findingMore, setFindingMore] = useState(false);
  const findingMoreStarted = useRef(false);
  const [brief, setBrief] = useState(proposal.intent.description);
  const [reason, setReason] = useState("");
  const reasonId = useId();
  const lineup = proposal.lineup ?? [];
  const acquisitions = proposal.acquisitions ?? [];
  const alternates = proposal.alternates ?? [];
  const dropped = new Set(edit?.drop ?? []);
  const selectedReady =
    lineup.filter((item) => !dropped.has(provisionKey(item))).length +
    (edit?.add?.filter((item) => item.inLibrary).length ?? 0);
  const selectedMissing =
    acquisitions.filter((item) => !dropped.has(provisionKey(item))).length +
    (edit?.add?.filter((item) => !item.inLibrary).length ?? 0);
  const selectedCount = selectedReady + selectedMissing;
  const currentLineup = selectedLineupContext(lineup, acquisitions, edit);
  const {
    refineText: _previousRefinement,
    currentLineup: _previousLineup,
    ...requestIntent
  } = proposal.intent;
  const statusDisplay = selfService ? (status === "approved" ? STATUS.approved : undefined) : STATUS[status];
  const scoresCurrent = status !== "partially-edited" && proposal.scores?.version === 1;
  const partialTheme = scoresCurrent && proposal.scores?.theme.status !== "supported";
  const controlsDisabled = busy || revising || findingMore;
  const requestDetails = (
    [
      proposal.intent.era ? (["Era", proposal.intent.era] as const) : null,
      proposal.intent.tone ? (["Tone", proposal.intent.tone] as const) : null,
      proposal.intent.runtimeTargetMin
        ? (["Target length", `${proposal.intent.runtimeTargetMin} minutes`] as const)
        : null,
      proposal.intent.maxAcquisitions
        ? (["New title limit", String(proposal.intent.maxAcquisitions)] as const)
        : null,
      proposal.intent.mustInclude?.length
        ? (["Must include", proposal.intent.mustInclude.join(", ")] as const)
        : null,
      proposal.intent.mustExclude?.length
        ? (["Must exclude", proposal.intent.mustExclude.join(", ")] as const)
        : null,
    ] as Array<readonly [string, string] | null>
  ).filter((detail): detail is readonly [string, string] => detail !== null);

  useEffect(() => {
    if (!findingMore) return;
    if (revising) findingMoreStarted.current = true;
    if (!revising && findingMoreStarted.current) {
      findingMoreStarted.current = false;
      setFindingMore(false);
    }
  }, [findingMore, revising]);

  return (
    <section className={cn("mx-auto flex w-full max-w-3xl flex-col gap-5 py-2", className)}>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          {showWorkflowHeading ? (
            <>
              <h2 className="font-semibold text-xl">Review your channel</h2>
              <p className="mt-1 font-medium text-base">{proposal.channelName || "Your new channel"}</p>
            </>
          ) : (
            <h2 className="font-semibold text-xl">{proposal.channelName || "Your new channel"}</h2>
          )}
        </div>
        {statusDisplay && <Badge variant={statusDisplay.variant}>{statusDisplay.label}</Badge>}
      </header>

      {editingBrief ? (
        <form
          className="flex flex-col gap-3 rounded-lg border border-suggest/40 bg-suggest/5 p-4"
          onSubmit={(event) => {
            event.preventDefault();
            const description = brief.trim();
            if (!description || !onRevise) return;
            onRevise({ ...requestIntent, description, currentLineup });
            setEditingBrief(false);
          }}
        >
          <div>
            <label htmlFor="proposal-brief" className="font-medium text-sm">
              Update your brief
            </label>
            <p className="mt-0.5 text-muted-foreground text-xs">
              Loomarr will refresh its suggestions. Your title choices stay with the draft.
            </p>
          </div>
          <textarea
            id="proposal-brief"
            aria-label="Channel brief"
            rows={3}
            value={brief}
            disabled={controlsDisabled}
            onChange={(event) => setBrief(event.target.value)}
            className="w-full resize-none rounded-lg border border-input bg-background px-3 py-2 text-sm leading-relaxed shadow-sm focus-visible:border-suggest focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-suggest"
          />
          {requestDetails.length > 0 && (
            <details className="text-sm">
              <summary className="w-fit cursor-pointer text-muted-foreground">
                Other request details ({requestDetails.length})
              </summary>
              <dl className="mt-2 grid gap-x-4 gap-y-1.5 rounded-md bg-background/60 p-3 sm:grid-cols-[auto_1fr]">
                {requestDetails.map(([label, value]) => (
                  <div key={label} className="contents">
                    <dt className="font-medium text-muted-foreground">{label}</dt>
                    <dd>{value}</dd>
                  </div>
                ))}
              </dl>
            </details>
          )}
          <div className="flex flex-wrap justify-end gap-2">
            <Button type="button" variant="ghost" size="sm" onClick={() => setEditingBrief(false)}>
              Cancel
            </Button>
            <Button
              type="submit"
              variant="suggest"
              size="sm"
              disabled={controlsDisabled || brief.trim() === ""}
            >
              Update suggestions
            </Button>
          </div>
        </form>
      ) : (
        <div className="flex flex-wrap items-start justify-between gap-3 border-border border-y py-3">
          <p className="min-w-0 flex-1 text-muted-foreground text-sm">“{proposal.intent.description}”</p>
          {actionable && onRevise && (
            <Button
              variant="ghost"
              size="sm"
              className="-my-1 shrink-0"
              onClick={() => {
                setBrief(proposal.intent.description);
                setEditingBrief(true);
              }}
              disabled={controlsDisabled}
            >
              Edit brief
            </Button>
          )}
        </div>
      )}

      <div className="flex flex-col gap-1 text-sm">
        <p>{availabilitySummary(selectedCount, selectedReady, selectedMissing)}</p>
        <ProposalOutlook assessment={assessment} pending={assessmentPending} showDetails={false} />
      </div>

      {(revising || findingMore) && (
        <p role="status" className="text-muted-foreground text-sm">
          {findingMore
            ? "Finding more suggestions… Your selected titles won’t change."
            : "Updating suggestions… Your current lineup will stay here until the new one is ready."}
        </p>
      )}

      {revisionError && !revising && !findingMore && (
        <p role="alert" className="rounded-md border border-onair/40 bg-onair/5 px-3 py-2 text-sm">
          We couldn't update the suggestions. Your current lineup is unchanged.
        </p>
      )}

      {partialTheme && (
        <p role="status" className="text-caution text-sm">
          <span className="font-medium">Some titles may be a loose match.</span> Remove anything that does not
          fit, or edit the brief.
        </p>
      )}

      <ProposalEdit
        lineup={lineup}
        acquisitions={acquisitions}
        alternates={alternates}
        optionalSuggestionKeys={optionalSuggestionKeys}
        onFindMore={
          actionable && onRevise
            ? () => {
                setFindingMore(true);
                onRevise({
                  ...requestIntent,
                  currentLineup,
                  refineText:
                    "Find 6–8 additional titles that match this brief. Do not repeat or replace the selected lineup.",
                });
              }
            : undefined
        }
        findingMore={findingMore}
        value={edit}
        onChange={onEdit}
        disabled={controlsDisabled}
      />

      {actionable && onApprove && (
        <footer className="sticky bottom-0 z-10 -mx-4 flex flex-col gap-3 border-border border-t bg-card/95 px-4 py-4 backdrop-blur sm:static sm:mx-0 sm:bg-transparent sm:px-0 sm:pb-0 sm:backdrop-blur-none">
          <div>
            <p className="font-medium text-sm">
              {selectedCount === 0
                ? "Choose at least one title"
                : selectedMissing > 0
                  ? `Loomarr will add ${selectedMissing} missing ${selectedMissing === 1 ? "title" : "titles"}.`
                  : "Everything selected is already in your library."}
            </p>
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
                disabled={controlsDisabled}
              />
              <div className="flex flex-wrap justify-end gap-2">
                <Button variant="ghost" onClick={() => setDenying(false)} disabled={controlsDisabled}>
                  Cancel
                </Button>
                <Button
                  variant="outline"
                  onClick={() => onDeny?.(reason.trim() || undefined)}
                  disabled={controlsDisabled}
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
                  disabled={controlsDisabled}
                >
                  {selfService ? "Discard" : "Not approved"}
                </Button>
              )}
              <Button onClick={onApprove} disabled={controlsDisabled || selectedCount === 0}>
                <Check aria-hidden /> Create channel
              </Button>
            </div>
          )}
        </footer>
      )}

      <details className="border-border border-t pt-3 text-sm">
        <summary className="cursor-pointer font-medium text-muted-foreground">
          How these suggestions were chosen
        </summary>
        <div className="mt-4 flex flex-col gap-5 text-muted-foreground">
          <section aria-label="Why these titles" className="flex flex-col gap-2">
            <h3 className="font-medium text-foreground">Why these titles</h3>
            {status === "partially-edited" ? (
              <p>You changed the title list. Check any titles you added against your brief.</p>
            ) : proposal.scores?.version !== 1 ? (
              <p>Loomarr couldn't check this saved draft. Review each title before creating the channel.</p>
            ) : (
              <>
                <p>
                  {proposal.scores.theme.basis === "named_membership"
                    ? "These suggestions are listed as part of the lineup you asked for."
                    : proposal.scores.theme.basis === "none"
                      ? "Review the titles to decide whether this is the channel you want."
                      : "We compared the titles with your description. Remove anything that doesn't fit."}
                </p>
                {proposal.scores.theme.basis === "qualifiers" && (
                  <ul className="list-inside list-disc">
                    {(proposal.scores.theme.qualifiers ?? []).map((qualifier) => (
                      <li key={qualifier.term}>
                        “{qualifier.term}” fits {qualifier.supportedItems} of{" "}
                        {proposal.scores.theme.assessedItems} titles Loomarr could check
                      </li>
                    ))}
                  </ul>
                )}
                {proposal.scores.era.status !== "not_requested" && (
                  <p>
                    {proposal.scores.era.status === "unassessed"
                      ? "Loomarr could not check the date range because some release or air dates were missing."
                      : `${proposal.scores.era.matchingItems} of ${proposal.scores.era.assessedItems} titles with known dates matched your requested range.`}
                  </p>
                )}
              </>
            )}
          </section>

          {assessmentPending ? (
            <p role="status">Updating what's ready to play…</p>
          ) : assessment ? (
            <ProposalOutlookDetails assessment={assessment} showDiagnostics={false} />
          ) : null}

          {(proposal.rationale || assessment || proposal.trace?.candidates?.length) && (
            <details className="border-border border-t pt-3">
              <summary className="w-fit cursor-pointer">Troubleshooting details</summary>
              <div className="mt-3 flex flex-col gap-4">
                <p className="text-xs">
                  This shows how Loomarr handled the original suggestions, before any changes you made.
                </p>
                {proposal.rationale && (
                  <section>
                    <h3 className="mb-1 font-medium text-foreground">Suggestion summary</h3>
                    <p>{friendlyProposalRationale(proposal.rationale)}</p>
                  </section>
                )}
                {assessment && !assessmentPending && <ProposalOutlookDiagnostics assessment={assessment} />}
                {proposal.trace?.candidates?.length ? (
                  <section aria-label="Titles considered">
                    <h3 className="mb-2 font-medium text-foreground">Titles considered</h3>
                    <ul className="flex flex-col divide-y divide-border">
                      {proposal.trace.candidates.map((candidate) => (
                        <li
                          key={`${candidate.key}-${candidate.disposition}-${candidate.reason}`}
                          className="py-2"
                        >
                          <p className="font-medium text-foreground">
                            {friendlyCandidateName(candidate.name)}
                          </p>
                          <p>{catalogDecision(candidate.disposition, candidate.reason)}</p>
                        </li>
                      ))}
                    </ul>
                    <details className="mt-3">
                      <summary className="w-fit cursor-pointer text-xs">Raw search data</summary>
                      <pre className="mt-2 max-h-72 overflow-auto rounded-md bg-muted p-3 text-xs">
                        {JSON.stringify(proposal.trace, null, 2)}
                      </pre>
                    </details>
                  </section>
                ) : null}
              </div>
            </details>
          )}
        </div>
      </details>
    </section>
  );
};

export { ProposalReview };
