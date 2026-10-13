import { provisionKey } from "@loomarr/core/provision";
import type { ApprovalEditDTO, ProposalDTO } from "@loomarr/core/requests";
import { Action, BottomSheet, Field, Surface, Text, Toggle } from "@loomarr/design-system";
import { useState } from "react";
import { Platform } from "react-native";

import type { ReviewGroup, ReviewRow } from "../review";
import type { ReviewSheetProps } from "./review-sheet.type";

const groupLabel: Record<ReviewGroup, string> = { filler: "Filler downloads", requests: "Channel requests" };

const Buttons = ({ children }: { children: React.ReactNode }) => (
  <Surface backgroundColor="$transparent" borderWidth={0} flexDirection="row" gap="$inline">
    {children}
  </Surface>
);

// Every sheet's row reads the same: the secondary action (Cancel, Done) is sized to its label and the
// primary one takes the rest, so a long label like "Approve with changes" stays on one line at 390 pt.
const SheetAction = ({
  disabled,
  label,
  onPress,
  tone,
}: {
  disabled?: boolean;
  label: string;
  onPress: () => void;
  tone: "primary" | "secondary";
}) => (
  <Action
    accessibilityRole="button"
    density="touch"
    disabled={disabled ?? false}
    onPress={onPress}
    style={tone === "secondary" ? { flexGrow: 0, flexShrink: 0 } : { flex: 1 }}
    tone={tone}
  >
    {label}
  </Action>
);

// Every title the approver could remove: the lineup and what would be downloaded. A title with no
// provisioning key can't be named in `drop`, so it isn't offered.
const editableTitles = (proposal: ProposalDTO) => {
  const seen = new Set<string>();
  return [...proposal.proposal.lineup, ...proposal.proposal.acquisitions].flatMap((item) => {
    const key = provisionKey(item);
    if (!key || seen.has(key)) return [];
    seen.add(key);
    return [{ key, label: item.year ? `${item.name} (${item.year})` : item.name }];
  });
};

// A draft lives only while its sheet is open. Nothing is saved until Approve: the edit is a parameter
// to the one approval, so cancelling leaves the proposal exactly as it was asked.
const EditSheet = ({
  busy,
  onApprove,
  onClose,
  proposal,
}: {
  busy: boolean;
  onApprove: (edit: ApprovalEditDTO) => void;
  onClose: () => void;
  proposal: ProposalDTO;
}) => {
  const [dropped, setDropped] = useState<readonly string[]>([]);
  const [note, setNote] = useState("");
  const titles = editableTitles(proposal);
  const trimmed = note.trim();
  return (
    <>
      <Text density="touch" numberOfLines={2} textRole="title">
        {`Edit "${proposal.proposal.intent.description}"`}
      </Text>
      <Text density="touch" textRole="body" tone="secondary">
        Untick a title to leave it out. Approving applies your changes.
      </Text>
      {titles.map(({ key, label }) => (
        <Toggle
          checked={!dropped.includes(key)}
          density="touch"
          disabled={busy}
          key={key}
          kind="checkbox"
          label={label}
          onCheckedChange={(keep) =>
            setDropped((previous) => (keep ? previous.filter((k) => k !== key) : [...previous, key]))
          }
        />
      ))}
      <Field
        density="touch"
        disabled={busy}
        label="Optional note to the requester"
        onChangeText={setNote}
        value={note}
      />
      <Buttons>
        <SheetAction label="Cancel" onPress={onClose} tone="secondary" />
        <SheetAction
          disabled={busy}
          label="Approve with changes"
          onPress={() =>
            onApprove({
              ...(dropped.length > 0 ? { drop: [...dropped] } : {}),
              ...(trimmed ? { note: trimmed } : {}),
            })
          }
          tone="primary"
        />
      </Buttons>
    </>
  );
};

const DenySheet = ({
  busy,
  filler,
  onClose,
  onDeny,
  title,
}: {
  busy: boolean;
  filler: boolean;
  onClose: () => void;
  onDeny: (reason?: string) => void;
  title: string;
}) => {
  const [reason, setReason] = useState("");
  const verb = filler ? "Dismiss" : "Deny";
  return (
    <>
      <Text density="touch" numberOfLines={2} textRole="title">{`${verb} "${title}"?`}</Text>
      {filler ? null : (
        <Field
          density="touch"
          disabled={busy}
          label="Optional note to the requester"
          onChangeText={setReason}
          value={reason}
        />
      )}
      <Buttons>
        <SheetAction label="Cancel" onPress={onClose} tone="secondary" />
        <SheetAction
          disabled={busy}
          label={verb}
          onPress={() => onDeny(reason.trim() || undefined)}
          tone="primary"
        />
      </Buttons>
    </>
  );
};

const ResultRows = ({ rows }: { rows: readonly ReviewRow[] }) => (
  <Surface backgroundColor="$transparent" borderWidth={0} gap="$inline">
    {rows.map((row) => (
      <Text density="touch" key={row.id} textRole="body" tone={row.ok ? "success" : "danger"}>
        {`${row.ok ? "✓" : "✕"} ${row.title}${row.error ? ` — ${row.error}` : ""}`}
      </Text>
    ))}
  </Surface>
);

// The one docked sheet of Needs you (#1840, on #1776's BottomSheet): confirm a bulk approve, say why a
// request is denied, edit before approving, or read what a bulk approve did id by id. The host puts it
// above its tab bar. iPhone's sheet has a grabber and drags to close; Android's is the docked strip.
const ReviewSheet = ({
  busy,
  onApproveEdited,
  onApproveSelected,
  onClose,
  onDeny,
  proposals,
  pulls,
  review,
}: ReviewSheetProps) => {
  const { sheet } = review;
  if (!sheet) return null;
  let content: React.ReactNode;
  let label: string;
  if (sheet.kind === "bulk") {
    const ids = (sheet.group === "requests" ? proposals : pulls).map((row) => row.id);
    const count = review.selected[sheet.group].filter((id) => ids.includes(id)).length;
    label = "Bulk approve";
    content = (
      <>
        <Text density="touch" textRole="title">{`${count} selected in ${groupLabel[sheet.group]}`}</Text>
        <Text density="touch" textRole="body" tone="secondary">
          Scope: this action approves only this group. Select items in another group separately.
        </Text>
        <Buttons>
          <SheetAction label="Cancel" onPress={onClose} tone="secondary" />
          <SheetAction
            disabled={busy || count === 0}
            label={`Approve ${count}`}
            onPress={() => onApproveSelected(sheet.group)}
            tone="primary"
          />
        </Buttons>
      </>
    );
  } else if (sheet.kind === "result") {
    const approved = sheet.rows.filter((row) => row.ok).length;
    const refused = sheet.rows.length - approved;
    label = "Approval result";
    content = (
      <>
        <Text density="touch" textRole="title">
          {`Approved ${approved} of ${sheet.rows.length}${refused > 0 ? ` · ${refused} couldn't be approved` : ""}`}
        </Text>
        <ResultRows rows={sheet.rows} />
        <Buttons>
          <SheetAction label="Done" onPress={onClose} tone="primary" />
        </Buttons>
      </>
    );
  } else if (sheet.kind === "edit") {
    const proposal = proposals.find((candidate) => candidate.id === sheet.id);
    if (!proposal) return null;
    label = "Edit request";
    content = (
      <EditSheet
        busy={busy}
        key={proposal.id}
        onApprove={(edit) => onApproveEdited(proposal, edit)}
        onClose={onClose}
        proposal={proposal}
      />
    );
  } else {
    const filler = sheet.group === "filler";
    const title = filler
      ? pulls.find((pull) => pull.id === sheet.id)?.title
      : proposals.find((proposal) => proposal.id === sheet.id)?.proposal.intent.description;
    if (title === undefined) return null;
    label = filler ? "Dismiss download" : "Deny request";
    content = (
      <DenySheet
        busy={busy}
        filler={filler}
        key={sheet.id}
        onClose={onClose}
        onDeny={(reason) => onDeny(sheet.group, sheet.id, reason)}
        title={title}
      />
    );
  }
  return (
    <BottomSheet accessibilityLabel={label} handle={Platform.OS !== "android"} onDismiss={onClose}>
      {content}
    </BottomSheet>
  );
};

export { ReviewSheet };
