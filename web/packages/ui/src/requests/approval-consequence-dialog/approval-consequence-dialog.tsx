import { pluralize } from "@loomarr/core/format";
import { approvalSizeLine } from "@loomarr/core/requests";
import { Action, Surface, Text } from "@loomarr/design-system";

import { ModalOverlay } from "../../overlay";
import type {
  ApprovalConsequenceDialogProps,
  BulkApproveConsequences,
  BulkApproveOutcome,
  ChannelRequestConsequence,
  FillerDownloadConsequence,
} from "./approval-consequence-dialog.type";

// What a group's bulk approve will DO, shown before it happens, then what it did (#1817, mock
// #1876). Two steps in one modal: the consequence summary with Cancel / Approve N, and the per-item
// result with a Retry on each failure the server marked retryable. A permanent failure (already
// decided, not found) shows its reason and nothing to press: a retry could only repeat it.
//
// ⚠ Every figure here is one the server stated; where it stated nothing the dialog says
// "unavailable" rather than inventing a number. A size line that is missing, or "0 B", reads as
// "this costs nothing" — the one wrong conclusion an approval summary must not invite.

const GROUP_COPY = {
  channels: { noun: "channel request", group: "Channel requests" },
  filler: { noun: "filler download", group: "Filler downloads" },
} as const;

const channelLine = ({ channel }: ChannelRequestConsequence) =>
  channel.kind === "update"
    ? `Updates the channel "${channel.name}"`
    : `Becomes a new channel named "${channel.name}"`;

const metaLine = (item: ChannelRequestConsequence) =>
  [
    pluralize(item.titleCount, "title"),
    `${item.toDownload} to download`,
    ...(item.ceiling ? [`up to ${item.ceiling}`] : []),
  ].join(" · ");

// ⚠ An estimate, rendered as one (PullCard's rule): "Loomarr said 40 and downloaded 12" is what an
// unmarked figure becomes. 0 means unmeasured, so it is not shown as "~0 clips".
const fillerEstimateLine = (item: FillerDownloadConsequence) => {
  const sources = pluralize(item.sources, "source");
  return item.estimateClips > 0
    ? `~${item.estimateClips} clips from ${sources} (estimate — may yield fewer once fetched)`
    : `From ${sources} (no clip estimate yet)`;
};

// Filler pulls carry no size until a pull producer returns (beta.11) — an empty item list is
// exactly "nothing known", which the shared rule words as unavailable.
const FILLER_SIZE_LINE = approvalSizeLine([], "downloads");

const Detail = ({ children, tone = "secondary" }: { children: string; tone?: "info" | "secondary" }) => (
  <Text textRole="caption" tone={tone}>
    {children}
  </Text>
);

const ConsequenceItem = ({ children, title }: { children: React.ReactNode; title: string }) => (
  <Surface
    backgroundColor="transparent"
    borderRadius="control"
    gap={4}
    paddingHorizontal={12}
    paddingVertical={10}
    role="listitem"
  >
    <Text textRole="label">{title}</Text>
    {children}
  </Surface>
);

const Consequences = ({ consequences }: { consequences: BulkApproveConsequences }) => (
  <Surface backgroundColor="transparent" borderWidth={0} gap={8} role="list">
    {consequences.group === "channels"
      ? consequences.items.map((item) => (
          <ConsequenceItem key={item.id} title={item.title}>
            <Detail>{metaLine(item)}</Detail>
            <Detail>{approvalSizeLine(item.sizes, "titles")}</Detail>
            <Detail tone="info">{channelLine(item)}</Detail>
          </ConsequenceItem>
        ))
      : consequences.items.map((item) => (
          <ConsequenceItem key={item.id} title={item.title}>
            <Detail>{fillerEstimateLine(item)}</Detail>
            <Detail>{FILLER_SIZE_LINE}</Detail>
            {/* Legacy pulls name a gap but no channel, and nothing records channels for new ones
                yet — so "unavailable", never a guessed channel and never "General catalog variety",
                which is only true when a producer recorded that no gap was filled. */}
            <Detail>Channel: unavailable</Detail>
          </ConsequenceItem>
        ))}
  </Surface>
);

const summaryLine = (consequences: BulkApproveConsequences) => {
  if (consequences.group === "channels") {
    const toDownload = consequences.items.reduce((n, i) => n + i.toDownload, 0);
    return `${pluralize(toDownload, "title")} will start downloading.`;
  }
  const clips = consequences.items.reduce((n, i) => n + i.estimateClips, 0);
  return clips > 0
    ? `~${clips} clips will start downloading, where still available.`
    : "Clips will start downloading, where still available.";
};

// Across the whole selection, with the same honesty rule as one item: a sum over the known sizes
// is labelled with how many it covers.
const selectionSizeLine = (consequences: BulkApproveConsequences) =>
  consequences.group === "channels"
    ? approvalSizeLine(
        consequences.items.flatMap((i) => i.sizes),
        "titles",
      )
    : FILLER_SIZE_LINE;

const outcomeLine = (consequences: BulkApproveConsequences, outcome: BulkApproveOutcome) => {
  if (!outcome.ok) return outcome.error || "Not approved";
  if (consequences.group === "channels") {
    const item = consequences.items.find((i) => i.id === outcome.id);
    const verb = item?.channel.kind === "update" ? "updated" : "created";
    const channel = item ? ` · channel "${item.channel.name}" ${verb}` : "";
    return `${pluralize(outcome.enqueued ?? 0, "title")} enqueued${channel}`;
  }
  // `enqueued` is the approved pull's exact candidate count (#1884) — no estimate language.
  return outcome.enqueued === undefined ? "Approved" : `${pluralize(outcome.enqueued, "clip")} queued`;
};

const ApprovalConsequenceDialog = ({
  consequences,
  error,
  onCancel,
  onConfirm,
  onDone,
  onRetry,
  pending = false,
  results,
  visible,
}: ApprovalConsequenceDialogProps) => {
  const copy = GROUP_COPY[consequences.group];
  const count = consequences.items.length;
  const titleOf = (id: string) => consequences.items.find((i) => i.id === id)?.title ?? id;
  const retryable = (results ?? []).filter((r) => !r.ok && r.retryable);
  // Roles sit on Surface: the design-system Text does not carry one to the web.
  const errorLine = error ? (
    <Surface backgroundColor="transparent" borderWidth={0} role="alert">
      <Text textRole="compact" tone="danger">
        {error}
      </Text>
    </Surface>
  ) : null;

  if (results) {
    return (
      <ModalOverlay
        actions={[
          { disabled: pending, label: "Done", onPress: onDone, preferredFocus: true, tone: "primary" },
        ]}
        // Escape mid-call would orphan a retry whose answer nobody then sees.
        dismissible={!pending}
        onDismiss={onDone}
        title={`Approved ${results.filter((r) => r.ok).length} of ${results.length}`}
        visible={visible}
      >
        <Surface aria-live="polite" backgroundColor="transparent" borderWidth={0} gap={6} role="list">
          {results.map((r) => (
            <Surface
              alignItems="flex-start"
              backgroundColor={r.ok ? "transparent" : "stateDangerSurface"}
              borderColor={r.ok ? "borderDecorative" : "stateDanger"}
              borderRadius="control"
              flexDirection="row"
              gap={8}
              paddingHorizontal={10}
              paddingVertical={8}
              key={r.id}
              role="listitem"
            >
              {/* The mark is the row's status, so it carries it in words for a screen reader. */}
              <Surface
                aria-label={r.ok ? "Approved" : "Not approved"}
                backgroundColor="transparent"
                borderWidth={0}
                role="img"
              >
                <Text textRole="label" tone={r.ok ? "success" : "danger"}>
                  {r.ok ? "✓" : "✕"}
                </Text>
              </Surface>
              <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={0}>
                <Text textRole="compact">{titleOf(r.id)}</Text>
                <Text textRole="caption" tone="secondary">
                  {outcomeLine(consequences, r)}
                </Text>
              </Surface>
              {r.ok || !r.retryable ? null : (
                <Action
                  accessibilityLabel={`Retry ${titleOf(r.id)}`}
                  accessibilityRole="button"
                  disabled={pending}
                  onPress={() => onRetry([r.id])}
                  tone="secondary"
                >
                  Retry
                </Action>
              )}
            </Surface>
          ))}
        </Surface>
        {errorLine}
        {retryable.length > 0 ? (
          <Action
            accessibilityRole="button"
            disabled={pending}
            onPress={() => onRetry(retryable.map((r) => r.id))}
            tone="secondary"
          >
            {`Retry ${retryable.length} failed`}
          </Action>
        ) : null}
      </ModalOverlay>
    );
  }

  return (
    <ModalOverlay
      actions={[
        { disabled: pending, label: "Cancel", onPress: onCancel, tone: "secondary" },
        {
          disabled: pending,
          label: pending ? "Approving…" : `Approve ${count}`,
          onPress: onConfirm,
          preferredFocus: true,
          tone: "primary",
        },
      ]}
      dismissible={!pending}
      onDismiss={onCancel}
      title={`Approve ${pluralize(count, copy.noun)}?`}
      visible={visible}
    >
      <Text textRole="compact" tone="secondary">
        {"Scope: this approves only the "}
        <Text textRole="label" tone="primary">
          {copy.group}
        </Text>
        {" group. Items selected in the other group are untouched."}
      </Text>
      <Consequences consequences={consequences} />
      <Text textRole="compact">{summaryLine(consequences)}</Text>
      <Text textRole="compact">{selectionSizeLine(consequences)}</Text>
      {errorLine}
    </ModalOverlay>
  );
};

export { ApprovalConsequenceDialog };
