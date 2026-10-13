import { Action, Surface, Text, Toggle } from "@loomarr/design-system";

import type { ApprovalGroupProps } from "./approval-group.type";

// One kind of decision on Needs you, as the approved mock draws it (#1840): a header with its own
// Select, and rows that each carry Deny, Edit where it applies, and Approve. Select swaps a row's
// actions for a checkbox. A group never selects another group's rows, so there is no select-all
// that spans both: Web's mixed bulk approve is the defect this shape removes.
const ApprovalGroup = ({
  deciding,
  denyLabel = "Deny",
  onApprove,
  onDeny,
  onEdit,
  onToggleSelected,
  onToggleSelecting,
  rows,
  selected,
  selecting,
  title,
}: ApprovalGroupProps) => (
  <Surface backgroundColor="$transparent" borderWidth={0} gap="$control" role="group">
    <Surface
      alignItems="center"
      backgroundColor="$transparent"
      borderWidth={0}
      flexDirection="row"
      justifyContent="space-between"
    >
      <Text accessibilityRole="header" density="touch" textRole="headline">
        {title}
      </Text>
      {/* One row has nothing to select all of; the mock offers Select from two rows up. */}
      {rows.length > 1 ? (
        <Action
          accessibilityRole="button"
          density="touch"
          disabled={deciding.length > 0}
          onPress={onToggleSelecting}
          tone="secondary"
        >
          {selecting ? "Cancel" : "Select"}
        </Action>
      ) : null}
    </Surface>
    {rows.map((row) => {
      const busy = deciding.includes(row.id);
      return (
        <Surface gap="$control" key={row.id} level="raised" padding="$control" width="100%">
          {selecting ? (
            <Toggle
              checked={selected.includes(row.id)}
              density="touch"
              disabled={busy}
              kind="checkbox"
              label={row.title}
              onCheckedChange={(on) => onToggleSelected(row.id, on)}
            />
          ) : (
            <Text density="touch" numberOfLines={2} textRole="title">
              {row.title}
            </Text>
          )}
          <Text density="touch" textRole="body" tone="secondary">
            {row.meta}
          </Text>
          {row.requestedBy ? (
            <Text
              density="touch"
              textRole="caption"
              tone="secondary"
            >{`Requested by ${row.requestedBy}`}</Text>
          ) : null}
          {row.note ? (
            <Text density="touch" textRole="body">
              {row.note}
            </Text>
          ) : null}
          {selecting ? null : (
            <Surface backgroundColor="$transparent" borderWidth={0} flexDirection="row" gap="$inline">
              <Action
                accessibilityRole="button"
                density="touch"
                disabled={busy}
                onPress={() => onDeny(row.id)}
                style={{ flex: 1 }}
                tone="secondary"
              >
                {denyLabel}
              </Action>
              {onEdit ? (
                <Action
                  accessibilityRole="button"
                  density="touch"
                  disabled={busy}
                  onPress={() => onEdit(row.id)}
                  style={{ flex: 1 }}
                  tone="secondary"
                >
                  Edit
                </Action>
              ) : null}
              <Action
                accessibilityRole="button"
                density="touch"
                disabled={busy}
                onPress={() => onApprove(row.id)}
                style={{ flex: 1 }}
                tone="primary"
              >
                {busy ? "Approving…" : "Approve"}
              </Action>
            </Surface>
          )}
        </Surface>
      );
    })}
  </Surface>
);

export { ApprovalGroup };
