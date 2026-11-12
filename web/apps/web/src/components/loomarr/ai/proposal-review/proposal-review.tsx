import { provisionKey } from "@loomarr/core/provision";
import { Action, Badge, type BadgeTone, Field, Stack, Text } from "@loomarr/design-system";
import { useEffect, useId, useRef, useState } from "react";
import { useAppForm } from "@/lib/app-form";
import { cn } from "@/lib/utils";
import {
  MAX_SUGGESTION_OPTIONS,
  selectedLineupContext,
  suggestionExpansionIntent,
  suggestionIdentity,
} from "@/suggest/suggestion-expansion";
import { friendlyCandidateName, friendlyProposalRationale } from "@/suggest/suggestion-language";
import { ProposalEdit } from "../proposal-edit";
import { ProposalOutlook, ProposalOutlookDetails, ProposalOutlookDiagnostics } from "../proposal-outlook";
import type { ProposalReviewProps, ProposalStatus } from "./proposal-review.type";

const STATUS: Partial<Record<ProposalStatus, { label: string; tone: BadgeTone }>> = {
  submitted: { label: "Sent for approval", tone: "suggest" },
  approved: { label: "Created", tone: "lock" },
  denied: { label: "Not approved", tone: "onair" },
  superseded: { label: "Replaced", tone: "neutral" },
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
    rationale_rejects: "Considered, but its own reason ruled it out",
    tone_mismatch: "Doesn't match the mood you asked for",
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

const ProposalReview = ({
  proposal,
  optionalSuggestions,
  expansionAdded,
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
  const [reason, setReason] = useState("");
  const reasonId = useId();
  const lineup = proposal.lineup ?? [];
  const acquisitions = proposal.acquisitions ?? [];
  const alternates = proposal.alternates ?? [];
  const suggestionIdentities = new Set(
    [...(optionalSuggestions ?? []), ...alternates].map(suggestionIdentity),
  );
  const dropped = new Set(edit?.drop ?? []);
  const selectedReady =
    lineup.filter((item) => !dropped.has(provisionKey(item))).length +
    (edit?.add?.filter((item) => item.inLibrary).length ?? 0);
  const selectedMissing =
    acquisitions.filter((item) => !dropped.has(provisionKey(item))).length +
    (edit?.add?.filter((item) => !item.inLibrary).length ?? 0);
  const selectedCount = selectedReady + selectedMissing;
  const currentLineup = selectedLineupContext(proposal, edit);
  const {
    refineText: _previousRefinement,
    currentLineup: _previousLineup,
    ...requestIntent
  } = proposal.intent;
  const briefForm = useAppForm({
    defaultValues: { brief: proposal.intent.description },
    onSubmit: ({ value }) => {
      const description = value.brief.trim();
      if (!description || !onRevise) return;
      onRevise({ ...requestIntent, description, currentLineup });
      setEditingBrief(false);
    },
  });
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
      <Stack align="start" direction="row" gap="control" justify="between" render="header" wrap>
        <div className="min-w-0">
          {showWorkflowHeading ? (
            <>
              <Text as="h2" strong textRole="headline">
                Review your channel
              </Text>
              <Text as="p" className="mt-1" textRole="cardLabel">
                {proposal.channelName || "Your new channel"}
              </Text>
            </>
          ) : (
            <Text as="h2" strong textRole="headline">
              {proposal.channelName || "Your new channel"}
            </Text>
          )}
        </div>
        {statusDisplay && (
          <Badge shape="square" tone={statusDisplay.tone}>
            {statusDisplay.label}
          </Badge>
        )}
      </Stack>

      {editingBrief ? (
        <form
          className="border-suggest/40 bg-suggest/5 flex flex-col gap-3 rounded-lg border p-4"
          onSubmit={(event) => {
            event.preventDefault();
            void briefForm.handleSubmit();
          }}
        >
          <briefForm.Field name="brief">
            {(field) => (
              <Field
                accessibilityLabel="Channel brief"
                description="Loomarr will refresh its suggestions. Your title choices stay with the draft."
                disabled={controlsDisabled}
                label="Update your brief"
                multiline
                numberOfLines={3}
                onChangeText={field.handleChange}
                value={field.state.value}
              />
            )}
          </briefForm.Field>
          {requestDetails.length > 0 && (
            <details>
              <summary className="w-fit cursor-pointer">
                <Text as="span" textRole="compact" tone="secondary">
                  Other request details ({requestDetails.length})
                </Text>
              </summary>
              <dl className="bg-background/60 mt-2 grid gap-x-4 gap-y-1.5 rounded-md p-3 sm:grid-cols-[auto_1fr]">
                {requestDetails.map(([label, value]) => (
                  <div key={label} className="contents">
                    <Text as="dt" textRole="cardLabel" tone="secondary">
                      {label}
                    </Text>
                    <Text as="dd" textRole="compact">
                      {value}
                    </Text>
                  </div>
                ))}
              </dl>
            </details>
          )}
          <Stack direction="row" gap="inline" justify="end" wrap>
            <Action onPress={() => setEditingBrief(false)} variant="ghost">
              Cancel
            </Action>
            <briefForm.Subscribe selector={(state) => state.values.brief.trim() === ""}>
              {(empty) => (
                <Action disabled={controlsDisabled || empty} submit tone="suggest">
                  Update suggestions
                </Action>
              )}
            </briefForm.Subscribe>
          </Stack>
        </form>
      ) : (
        <Stack
          align="start"
          className="border-border border-y py-3"
          direction="row"
          gap="control"
          justify="between"
          wrap
        >
          <Text as="p" className="min-w-0 flex-1" textRole="compact" tone="secondary">
            “{proposal.intent.description}”
          </Text>
          {actionable && onRevise && (
            <div className="-my-1 shrink-0">
              <Action
                disabled={controlsDisabled}
                onPress={() => {
                  briefForm.reset({ brief: proposal.intent.description });
                  setEditingBrief(true);
                }}
                variant="ghost"
              >
                Edit brief
              </Action>
            </div>
          )}
        </Stack>
      )}

      <Stack gap="1">
        <Text as="p" textRole="compact">
          {availabilitySummary(selectedCount, selectedReady, selectedMissing)}
        </Text>
        <ProposalOutlook assessment={assessment} pending={assessmentPending} showDetails={false} />
      </Stack>

      {(revising || findingMore) && (
        <div role="status">
          <Text as="p" textRole="compact" tone="secondary">
            {findingMore
              ? "Finding more suggestions… Your selected titles won’t change."
              : "Updating suggestions… Your current lineup will stay here until the new one is ready."}
          </Text>
        </div>
      )}

      {revisionError && !revising && !findingMore && (
        <div role="alert" className="border-onair/40 bg-onair/5 rounded-md border px-3 py-2">
          <Text as="p" textRole="compact">
            We couldn't update the suggestions. Your current lineup is unchanged.
          </Text>
        </div>
      )}

      {expansionAdded === 0 && !revising && !findingMore && !revisionError && (
        <div role="status">
          <Text as="p" textRole="compact" tone="secondary">
            Loomarr didn’t find any new strong matches this time. Your existing options are still here.
          </Text>
        </div>
      )}

      {partialTheme && (
        <div role="status">
          <Text as="p" textRole="compact" tone="warning">
            <Text as="span" textRole="cardLabel" tone="warning">
              Some titles may be a loose match.
            </Text>{" "}
            Remove anything that does not fit, or edit the brief.
          </Text>
        </div>
      )}

      <ProposalEdit
        lineup={lineup}
        acquisitions={acquisitions}
        alternates={alternates}
        optionalSuggestions={optionalSuggestions}
        onFindMore={
          actionable && onRevise && suggestionIdentities.size < MAX_SUGGESTION_OPTIONS
            ? () => {
                setFindingMore(true);
                onRevise(
                  suggestionExpansionIntent(proposal, edit, [...(optionalSuggestions ?? []), ...alternates]),
                );
              }
            : undefined
        }
        findingMore={findingMore}
        value={edit}
        onChange={onEdit}
        disabled={controlsDisabled}
      />

      {actionable && onApprove && (
        <Stack
          className="border-border bg-card/95 sticky bottom-0 z-10 -mx-4 border-t px-4 py-4 backdrop-blur sm:static sm:mx-0 sm:bg-transparent sm:px-0 sm:pb-0 sm:backdrop-blur-none"
          gap="control"
          render="footer"
        >
          <div>
            <Text as="p" textRole="cardLabel">
              {selectedCount === 0
                ? "Choose at least one title"
                : selectedMissing > 0
                  ? `Loomarr will add ${selectedMissing} missing ${selectedMissing === 1 ? "title" : "titles"}.`
                  : "Everything selected is already in your library."}
            </Text>
          </div>

          {denying && !selfService ? (
            <Stack gap="inline">
              <Field
                autoFocus
                disabled={controlsDisabled}
                id={reasonId}
                label="Why not? Optional: the requester sees this."
                onChangeText={setReason}
                placeholder="For example, ask for fewer new titles"
                value={reason}
              />
              <Stack direction="row" gap="inline" justify="end" wrap>
                <Action disabled={controlsDisabled} onPress={() => setDenying(false)} variant="ghost">
                  Cancel
                </Action>
                <Action
                  disabled={controlsDisabled}
                  icon="close"
                  onPress={() => onDeny?.(reason.trim() || undefined)}
                  variant="outline"
                >
                  Not approved
                </Action>
              </Stack>
            </Stack>
          ) : (
            <Stack direction="row" gap="inline" justify="end" wrap>
              {onDeny && (
                <Action
                  disabled={controlsDisabled}
                  onPress={() => (selfService ? onDeny() : setDenying(true))}
                  variant="ghost"
                >
                  {selfService ? "Discard" : "Not approved"}
                </Action>
              )}
              <Action disabled={controlsDisabled || selectedCount === 0} icon="success" onPress={onApprove}>
                Create channel
              </Action>
            </Stack>
          )}
        </Stack>
      )}

      <details className="border-border border-t pt-3 text-sm">
        <summary className="cursor-pointer">
          <Text as="span" textRole="cardLabel" tone="secondary">
            How these suggestions were chosen
          </Text>
        </summary>
        <Stack className="text-muted-foreground mt-4" gap="5">
          <Stack aria-label="Why these titles" gap="inline" render="section">
            <Text as="h3" textRole="cardLabel">
              Why these titles
            </Text>
            {status === "partially-edited" ? (
              <p>You changed the title list. Check any titles you added against your brief.</p>
            ) : proposal.manual ? (
              <p>You picked every title in this channel yourself — there's nothing to check.</p>
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
          </Stack>

          {assessmentPending ? (
            <p role="status">Updating what's ready to play…</p>
          ) : assessment ? (
            <ProposalOutlookDetails assessment={assessment} showDiagnostics={false} />
          ) : null}

          {(proposal.rationale || assessment || proposal.trace?.candidates?.length) && (
            <details className="border-border border-t pt-3">
              <summary className="w-fit cursor-pointer">Troubleshooting details</summary>
              <Stack className="mt-3" gap="4">
                <Text as="p" textRole="hint" tone="secondary">
                  This shows how Loomarr handled the original suggestions, before any changes you made.
                </Text>
                {proposal.rationale && (
                  <section>
                    <Text as="h3" className="mb-1" textRole="cardLabel">
                      Suggestion summary
                    </Text>
                    <p>{friendlyProposalRationale(proposal.rationale)}</p>
                  </section>
                )}
                {assessment && !assessmentPending && <ProposalOutlookDiagnostics assessment={assessment} />}
                {proposal.trace?.candidates?.length ? (
                  <section aria-label="Titles considered">
                    <Text as="h3" className="mb-2" textRole="cardLabel">
                      Titles considered
                    </Text>
                    <Stack className="divide-border divide-y" render="ul">
                      {proposal.trace.candidates.map((candidate) => (
                        <li
                          key={`${candidate.key}-${candidate.disposition}-${candidate.reason}`}
                          className="py-2"
                        >
                          <Text as="p" textRole="cardLabel">
                            {friendlyCandidateName(candidate.name)}
                          </Text>
                          <p>{catalogDecision(candidate.disposition, candidate.reason)}</p>
                        </li>
                      ))}
                    </Stack>
                    <details className="mt-3">
                      <summary className="w-fit cursor-pointer">
                        <Text as="span" textRole="hint">
                          Raw search data
                        </Text>
                      </summary>
                      <pre className="bg-muted mt-2 max-h-72 overflow-auto rounded-md p-3 text-xs">
                        {JSON.stringify(proposal.trace, null, 2)}
                      </pre>
                    </details>
                  </section>
                ) : null}
              </Stack>
            </details>
          )}
        </Stack>
      </details>
    </section>
  );
};

export { ProposalReview };
