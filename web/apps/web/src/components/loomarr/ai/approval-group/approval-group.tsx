import { useId } from "react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { cn } from "@/lib/utils";
import type { ApprovalGroupProps } from "./approval-group.type";

// ApprovalGroup — one kind of pending decision with its own bulk scope (Decision R1, mock #1876).
//
// ⚠ Channel requests and filler downloads are separate groups, each with its own select-all and
// its own Approve, and nothing spans both. That is the fix for "two domains in one queue": a
// mixed bulk approve could not say in one sentence what it was about to do, so it cannot exist.
//
// The group's Approve opens the consequence summary; it never approves on its own.

const ApprovalGroup = ({
  title,
  requestedBy,
  rows,
  selected,
  onSelectedChange,
  onApprove,
  disabled = false,
  className,
}: ApprovalGroupProps) => {
  const titleId = useId();
  const selectAllId = useId();
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
    <Card role="group" aria-labelledby={titleId} className={cn("overflow-hidden", className)}>
      <div className="flex flex-wrap items-center gap-2.5 border-border border-b bg-static-950/50 px-3.5 py-2.5">
        <label htmlFor={selectAllId} className="flex items-center gap-2">
          {/* A "select all" over a single row is noise; that row's own checkbox is the control. */}
          {bulkable.length > 1 && (
            <Checkbox
              id={selectAllId}
              className="size-4.5"
              checked={allSelected}
              disabled={disabled}
              aria-label={`Select all ${title}`}
              onChange={(e) =>
                onSelectedChange(e.target.checked ? new Set(bulkable.map((r) => r.id)) : new Set())
              }
            />
          )}
          <span id={titleId} className="font-semibold text-sm">
            {title}
          </span>
        </label>
        {requestedBy && <span className="text-muted-foreground text-xs">{requestedBy}</span>}
        <span className="flex-auto" />
        <span className="text-muted-foreground text-xs">
          {selectedCount > 0 ? `${selectedCount} selected` : "None selected"}
        </span>
        <Button size="sm" disabled={disabled || selectedCount === 0} onClick={onApprove}>
          {selectedCount > 0 ? `Approve ${selectedCount}` : "Approve"}
        </Button>
      </div>
      <ul className="flex flex-col divide-y divide-border">
        {rows.map((row) => (
          <li key={row.id} className="flex items-start gap-2.5 px-3.5 py-3">
            {/* An excluded row has no checkbox at all rather than a disabled one: its own Approve
                is the right action, and a greyed box invites a click that would do the wrong thing. */}
            {!row.excluded && (
              <Checkbox
                className="mt-0.5 size-4.5"
                checked={selected.has(row.id)}
                disabled={disabled}
                aria-label={`Select ${row.label} for bulk approve`}
                onChange={(e) => toggle(row.id, e.target.checked)}
              />
            )}
            <div className="flex min-w-0 flex-1 flex-col gap-1.5">
              {row.content}
              {row.excluded && (
                <p className="text-caution text-xs">
                  Excluded from bulk — approve individually so the edit is kept.
                </p>
              )}
            </div>
          </li>
        ))}
      </ul>
    </Card>
  );
};

export { ApprovalGroup };
