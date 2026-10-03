import { Action, Field, Surface, Text, Toggle } from "@loomarr/design-system";
import { useState } from "react";

import type { ApprovalCardProps } from "./approval-card.type";

// One proposal waiting on an admin (Web's approval row): what was asked, by whom, how many titles
// would be downloaded, and two actions. Deny arms a reason field instead of firing, as on Web: the
// reason is optional, and it is what the requester reads. Editing a lineup stays on Web.
const ApprovalCard = ({
  busy = false,
  onApprove,
  onDeny,
  onToggleSelected,
  proposal,
  selected,
}: ApprovalCardProps) => {
  const [denying, setDenying] = useState(false);
  const [reason, setReason] = useState("");
  const { acquisitions, lineup } = proposal.proposal;
  const description = proposal.proposal.intent.description;
  const counts = [
    `${lineup.length} ${lineup.length === 1 ? "title" : "titles"}`,
    acquisitions.length > 0 ? `${acquisitions.length} to acquire` : "all in your library",
  ].join(" · ");

  return (
    <Surface gap="$control" level="raised" padding="$control" width="100%">
      {onToggleSelected ? (
        <Toggle
          checked={selected === true}
          density="touch"
          disabled={busy}
          kind="checkbox"
          label={description}
          onCheckedChange={onToggleSelected}
        />
      ) : (
        <Text density="touch" textRole="title">
          {description}
        </Text>
      )}
      <Text density="touch" textRole="body" tone="secondary">
        {counts}
      </Text>
      {proposal.createdByName ? (
        <Text
          density="touch"
          textRole="caption"
          tone="secondary"
        >{`Requested by ${proposal.createdByName}`}</Text>
      ) : null}
      {proposal.note ? (
        <Text density="touch" textRole="body">
          {proposal.note}
        </Text>
      ) : null}
      {denying ? (
        <Surface backgroundColor="$transparent" borderWidth={0} gap="$inline">
          <Field
            density="touch"
            disabled={busy}
            label="Why not? Optional: the requester sees this."
            onChangeText={setReason}
            placeholder="e.g. over the cap this week, ask again Monday"
            value={reason}
          />
          <Surface backgroundColor="$transparent" borderWidth={0} flexDirection="row" gap="$inline">
            <Action
              accessibilityRole="button"
              density="touch"
              disabled={busy}
              onPress={() => {
                setDenying(false);
                setReason("");
              }}
              style={{ flex: 1 }}
              tone="secondary"
            >
              Cancel
            </Action>
            <Action
              accessibilityRole="button"
              density="touch"
              disabled={busy}
              onPress={() => onDeny(reason.trim() || undefined)}
              style={{ flex: 1 }}
              tone="danger"
            >
              Deny request
            </Action>
          </Surface>
        </Surface>
      ) : (
        <>
          <Text density="touch" textRole="caption" tone="secondary">
            Approving builds the channel and starts the downloads.
          </Text>
          <Surface backgroundColor="$transparent" borderWidth={0} flexDirection="row" gap="$inline">
            <Action
              accessibilityRole="button"
              density="touch"
              disabled={busy}
              onPress={() => setDenying(true)}
              style={{ flex: 1 }}
              tone="secondary"
            >
              Deny
            </Action>
            <Action
              accessibilityRole="button"
              density="touch"
              disabled={busy}
              onPress={onApprove}
              style={{ flex: 1 }}
              tone="primary"
            >
              {busy ? "Approving…" : "Approve"}
            </Action>
          </Surface>
        </>
      )}
    </Surface>
  );
};

export { ApprovalCard };
