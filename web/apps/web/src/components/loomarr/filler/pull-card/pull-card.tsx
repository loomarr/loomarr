import { pluralize } from "@loomarr/core/format";
import { Action, Badge, Card, Field, Stack, StatusLabel, Text } from "@loomarr/design-system";
import { useStore } from "@tanstack/react-form";
import { useId, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useAppForm } from "@/lib/app-form";
import { cn } from "@/lib/utils";
import type { PullCardProps } from "./pull-card.type";

// PullCard — a proposed filler acquisition awaiting a human (V66).
//
// ⚠ **This card IS the approval gate's face.** §10 has said "the machine proposes, a human
// commits" since the starter pack shipped; until V35 there was no object to commit. Nothing
// downloads while this card sits here — approving is the only path that enqueues, and it goes
// through the ordinary ingest job rather than a downloader of its own.
//
// ⚠ **Dropping a row does not remove it from the record.** The plan keeps every row it was
// proposed with and marks the dropped ones, because "we approved this" is only meaningful next
// to what was proposed. That is a server property; this component's job is not to hide it.

const PullCard = ({
  pull,
  onApprove,
  onDismiss,
  onEditedChange,
  deciding,
  error,
  flat = false,
}: PullCardProps) => {
  // Which rows the operator has struck out, held locally until they commit. ⚠ Not sent as they
  // click: a pull is decided in one act, and a per-click PATCH would make "half-approved" a
  // state the gate has to reason about.
  const [dropped, setDropped] = useState<ReadonlySet<string>>(new Set());
  // The annotation is read when the pull is approved; nothing validates or submits it on its own.
  const noteForm = useAppForm({ defaultValues: { note: "" } });
  const note = useStore(noteForm.store, (state) => state.values.note);
  const titleId = useId();
  const edited = dropped.size > 0 || note.trim() !== "";

  // ⚠ `?? []` because huma types every Go slice as nullable, so the generated DTO says
  // `PullPlanRowDTO[] | null` even though the handler always sends `[]`.
  const plan = pull.plan ?? [];
  const skippedSources = (pull.sources ?? []).filter((source) => source.disposition !== "enumerated");
  const candidateID = (row: (typeof plan)[number]) => row.candidateId || row.sourceId;
  const kept = plan.filter((row) => !dropped.has(candidateID(row)));

  // Reported from the handlers, not an effect, so the queue hears about an edit in the same
  // event that made it.
  const toggleDropped = (id: string) => {
    const next = new Set(dropped);
    if (!next.delete(id)) next.add(id);
    setDropped(next);
    onEditedChange?.(next.size > 0 || note.trim() !== "");
  };

  return (
    // A named group so a screen reader announces which pull it has entered — several cards share
    // identical "Not now" / "Approve pull" buttons, and the title is the only thing telling them apart.
    <Card
      aria-labelledby={titleId}
      gap={12}
      role="group"
      {...(flat ? { backgroundColor: "transparent", borderWidth: 0 } : { padding: 16 })}
    >
      <Stack align="start" direction="row" gap="control" wrap>
        <Stack flexBasis="0%" flexGrow={1} flexShrink={1} gap="1">
          <Stack align="center" direction="row" gap="inline" wrap>
            <StatusLabel tone="ok">Filler pull</StatusLabel>
            <Text as="span" id={titleId} strong textRole="cardLabel" tone="primary">
              {pull.title}
            </Text>
            {edited && <StatusLabel tone="attention">Edited</StatusLabel>}
          </Stack>
          {/* ⚠ proposedByName is resolved server-side (#1430), the same way a proposal's
              requester is — proposedBy itself is a raw user id (or a scheduled job's name) and
              must never reach the screen. */}
          {pull.proposedByName && (
            <Text as="span" textRole="mono">
              proposed by {pull.proposedByName}
            </Text>
          )}
        </Stack>

        <Stack direction="row" flexShrink={0} gap="inline">
          <Action disabled={deciding} onPress={onDismiss} variant="outline">
            Not now
          </Action>
          <Action
            // Nothing left to fetch is refused by the server rather than recorded as an
            // approval that fetched nothing; disabling here says so before the round trip.
            disabled={deciding || kept.length === 0}
            onPress={() => onApprove({ dropCandidateIds: [...dropped], note })}
          >
            {deciding ? "Starting…" : "Approve pull"}
          </Action>
        </Stack>
      </Stack>

      {/* ⚠ On the row, not in a page-level banner (#1430): a decision failure belongs beside the
          card whose decision failed, not somewhere a reviewer scanning other cards has to go
          looking for it. */}
      {error != null && <ErrorState error={error} />}

      {/* Why this pull exists. "Approve this" without a reason is a button, not a decision. */}
      {pull.reason && (
        <Text as="p" className="bg-muted/40 rounded-md p-3" textRole="compact" tone="secondary">
          {pull.reason}
        </Text>
      )}

      <Stack gap="inline" render="ul">
        {plan.map((row) => {
          const id = candidateID(row);
          const isDropped = dropped.has(id);
          return (
            <Stack
              align="center"
              className={cn(isDropped && "opacity-50")}
              direction="row"
              gap="control"
              key={id}
              render="li"
              wrap
            >
              <Badge shape="square" tone="neutral">
                {row.tag}
              </Badge>
              <div className="min-w-0 flex-1">
                <Text
                  as="p"
                  className={cn("truncate", isDropped && "line-through")}
                  textRole="compact"
                  tone="primary"
                >
                  {row.name}
                </Text>
                {row.why && (
                  <Text as="span" textRole="mono">
                    {row.why}
                  </Text>
                )}
              </div>
              {/* ⚠ An estimate, rendered as one. What a source yields depends on what is still
                  there, what deduplicates, and what the splitter makes of a compilation — a
                  number presented as exact becomes "Loomarr said 40 and downloaded 12". */}
              {row.estimateClips > 0 && (
                <Text as="span" className="tabular-nums" textRole="mono">
                  ~{row.estimateClips} clips
                </Text>
              )}
              <Action
                accessibilityLabel={isDropped ? `Put back ${row.name}` : `Leave out ${row.name}`}
                disabled={deciding}
                onPress={() => toggleDropped(id)}
                variant="ghost"
              >
                {isDropped ? "Put back" : "Leave out"}
              </Action>
            </Stack>
          );
        })}
      </Stack>

      {(pull.rejected?.length ?? 0) > 0 && (
        <details className="border-border rounded-md border px-3 py-2">
          <Text as="summary" className="cursor-pointer" textRole="compact" tone="secondary">
            {pluralize(pull.rejected?.length ?? 0, "other candidate")} considered and excluded
          </Text>
          <Stack className="mt-2" gap="1" render="ul">
            {(pull.rejected ?? []).map((candidate) => (
              <Stack direction="row" gap="inline" key={candidate.candidateId} render="li" wrap>
                <Text as="span" textRole="compact" tone="primary">
                  {candidate.name}
                </Text>
                <Text as="span" textRole="mono">
                  {candidate.disposition.replaceAll("_", " ")}: {candidate.detail}
                </Text>
              </Stack>
            ))}
          </Stack>
        </details>
      )}

      {skippedSources.length > 0 && (
        <details className="border-border rounded-md border px-3 py-2">
          <Text as="summary" className="cursor-pointer" textRole="compact" tone="secondary">
            {pluralize(skippedSources.length, "registered source")} not searched
          </Text>
          <Stack className="mt-2" gap="1" render="ul">
            {skippedSources.map((source) => (
              <Stack direction="row" gap="inline" key={source.sourceId} render="li" wrap>
                <Text as="span" textRole="compact" tone="primary">
                  {source.label || source.sourceId}
                </Text>
                <Text as="span" textRole="mono">
                  {source.disposition.replaceAll("_", " ")}: {source.detail}
                </Text>
              </Stack>
            ))}
          </Stack>
        </details>
      )}

      <noteForm.Field name="note">
        {(field) => (
          <Field
            accessibilityLabel="Notes for this pull"
            disabled={deciding}
            onChangeText={(value) => {
              field.handleChange(value);
              onEditedChange?.(dropped.size > 0 || value.trim() !== "");
            }}
            placeholder="Optional annotation for your records"
            value={field.state.value}
          />
        )}
      </noteForm.Field>

      <Text as="span" textRole="mono">
        Optional annotation for your records. This does not change what is downloaded.
      </Text>

      <Text as="span" textRole="mono">
        {kept.length === 0
          ? "Every candidate is left out, so there's nothing to fetch. Dismiss it instead."
          : `${pluralize(kept.length, "candidate")} will be fetched. Nothing downloads until you approve.`}
      </Text>
    </Card>
  );
};

export { PullCard };
