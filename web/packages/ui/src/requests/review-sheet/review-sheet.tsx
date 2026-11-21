import { provisionKey } from "@loomarr/core/provision";
import type { ApprovalEditDTO, ProposalDTO } from "@loomarr/core/requests";
import { Action, BottomSheet, Surface, Text, Toggle } from "@loomarr/design-system";
import { Platform } from "react-native";

import { useUiForm } from "../../form-kit";
import type { ReviewGroup, ReviewRow } from "../review";
import type { ReviewSheetProps } from "./review-sheet.type";

const groupLabel: Record<ReviewGroup, string> = { filler: "Filler downloads", requests: "Channel requests" };

const Buttons = ({ children }: { children: React.ReactNode }) => (
  <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" gap="inline">
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
  const form = useUiForm({
    defaultValues: { dropped: [] as string[], note: "" },
    onSubmit: ({ value }) => {
      const trimmed = value.note.trim();
      onApprove({
        ...(value.dropped.length > 0 ? { drop: [...value.dropped] } : {}),
        ...(trimmed ? { note: trimmed } : {}),
      });
    },
  });
  const titles = editableTitles(proposal);
  return (
    <>
      <Text density="touch" numberOfLines={2} textRole="title">
        {`Edit "${proposal.proposal.intent.description}"`}
      </Text>
      <Text density="touch" textRole="body" tone="secondary">
        Untick a title to leave it out. Approving applies your changes.
      </Text>
      <form.Field name="dropped">
        {(field) =>
          titles.map(({ key, label }) => (
            <Toggle
              checked={!field.state.value.includes(key)}
              density="touch"
              disabled={busy}
              key={key}
              kind="checkbox"
              label={label}
              onCheckedChange={(keep) =>
                field.handleChange((previous) =>
                  keep ? previous.filter((k) => k !== key) : [...previous, key],
                )
              }
            />
          ))
        }
      </form.Field>
      <form.AppField name="note">
        {(field) => (
          <field.TextField density="touch" disabled={busy} label="Optional note to the requester" />
        )}
      </form.AppField>
      <Buttons>
        <SheetAction label="Cancel" onPress={onClose} tone="secondary" />
        <SheetAction
          disabled={busy}
          label="Approve with changes"
          onPress={() => void form.handleSubmit()}
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
  const form = useUiForm({
    defaultValues: { reason: "" },
    onSubmit: ({ value }) => onDeny(value.reason.trim() || undefined),
  });
  const verb = filler ? "Dismiss" : "Deny";
  return (
    <>
      <Text density="touch" numberOfLines={2} textRole="title">{`${verb} "${title}"?`}</Text>
      {filler ? null : (
        <form.AppField name="reason">
          {(field) => (
            <field.TextField density="touch" disabled={busy} label="Optional note to the requester" />
          )}
        </form.AppField>
      )}
      <Buttons>
        <SheetAction label="Cancel" onPress={onClose} tone="secondary" />
        <SheetAction disabled={busy} label={verb} onPress={() => void form.handleSubmit()} tone="primary" />
      </Buttons>
    </>
  );
};

const ResultRows = ({ rows }: { rows: readonly ReviewRow[] }) => (
  <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
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
