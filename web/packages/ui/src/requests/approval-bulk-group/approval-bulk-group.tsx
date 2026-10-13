import { Action, Surface, Text, Toggle } from "@loomarr/design-system";

import type { ApprovalBulkGroupProps } from "./approval-bulk-group.type";

// One kind of pending decision with its own bulk scope, as the approved web mock draws it (Decision
// R1, #1876): a head with its own select-all, a count and "Approve N", then the rows, each with
// its own checkbox. The native #1840 ApprovalGroup is the touch shape of the same decision.
//
// ⚠ Channel requests and filler downloads are separate groups and nothing spans both: a mixed bulk
// approve could not say in one sentence what it was about to do. The group's Approve opens the
// consequence summary; it never approves on its own.
const ApprovalBulkGroup = ({
  disabled = false,
  onApprove,
  onSelectedChange,
  requestedBy,
  rows,
  selected,
  title,
}: ApprovalBulkGroupProps) => {
  const bulkable = rows.filter((r) => !r.excluded);
  // Only ids still in the group count — a row that left the queue must not inflate "N selected".
  const selectedCount = bulkable.filter((r) => selected.has(r.id)).length;
  const allSelected = bulkable.length > 0 && selectedCount === bulkable.length;

  const toggle = (id: string, on: boolean) => {
    const next = new Set(selected);
    if (on) next.add(id);
    else next.delete(id);
    onSelectedChange(next);
  };

  return (
    <Surface aria-label={title} level="raised" overflow="hidden" role="group">
      <Surface
        alignItems="center"
        borderRadius={0}
        // ⚠ Tamagui applies props in order: borderWidth must come before the one side it keeps.
        borderWidth={0}
        borderBottomWidth={1}
        flexDirection="row"
        flexWrap="wrap"
        gap={10}
        level="canvas"
        paddingHorizontal={14}
        paddingVertical={10}
      >
        <Surface
          alignItems="center"
          backgroundColor="$transparent"
          borderWidth={0}
          flexDirection="row"
          gap={8}
        >
          {/* A "select all" over a single row is noise; that row's own checkbox is the control. */}
          {bulkable.length > 1 ? (
            <Toggle
              checked={allSelected}
              disabled={disabled}
              label={`Select all ${title}`}
              labelHidden
              onCheckedChange={(on) => onSelectedChange(on ? new Set(bulkable.map((r) => r.id)) : new Set())}
            />
          ) : null}
          <Text textRole="label">{title}</Text>
        </Surface>
        {requestedBy ? (
          <Text textRole="metadata" tone="secondary">
            {requestedBy}
          </Text>
        ) : null}
        <Surface backgroundColor="$transparent" borderWidth={0} flexGrow={1} />
        <Text textRole="metadata" tone="secondary">
          {selectedCount > 0 ? `${selectedCount} selected` : "None selected"}
        </Text>
        {/* The visible text is the mock's terse "Approve" / "Approve N", but each row also has an
          "Approve": the name must say this one acts on the group's selection. It starts with the
          visible text so a speech user can still say what they see. */}
        <Action
          accessibilityLabel={
            selectedCount > 0
              ? `Approve ${selectedCount} selected in ${title.toLowerCase()}`
              : `Approve selected ${title.toLowerCase()}`
          }
          accessibilityRole="button"
          disabled={disabled || selectedCount === 0}
          onPress={onApprove}
          tone="primary"
        >
          {selectedCount > 0 ? `Approve ${selectedCount}` : "Approve"}
        </Action>
      </Surface>
      <Surface backgroundColor="$transparent" borderRadius={0} borderWidth={0} role="list">
        {rows.map((row, index) => (
          <Surface
            alignItems="flex-start"
            backgroundColor="$transparent"
            borderRadius={0}
            borderWidth={0}
            borderTopWidth={index === 0 ? 0 : 1}
            flexDirection="row"
            gap={10}
            key={row.id}
            paddingHorizontal={14}
            paddingVertical={12}
            role="listitem"
          >
            {/* An excluded row has no checkbox at all rather than a disabled one: its own Approve is
              the right action, and a greyed box invites a click that would do the wrong thing. */}
            {row.excluded ? null : (
              <Toggle
                checked={selected.has(row.id)}
                disabled={disabled}
                label={`Select ${row.label} for bulk approve`}
                labelHidden
                onCheckedChange={(on) => toggle(row.id, on)}
              />
            )}
            <Surface backgroundColor="$transparent" borderWidth={0} flex={1} gap={6} minWidth={0}>
              {row.content}
              {row.excluded ? (
                <Text textRole="caption" tone="warning">
                  Excluded from bulk — approve individually so the edit is kept.
                </Text>
              ) : null}
            </Surface>
          </Surface>
        ))}
      </Surface>
    </Surface>
  );
};

export { ApprovalBulkGroup };
