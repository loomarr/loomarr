import { formatRelative } from "@loomarr/core";
import { Badge } from "@/components/ui";
import { cn } from "@/lib";
import type { ApprovalHistoryRowProps } from "./approval-history-row.type";

// ApprovalHistoryRow — one DECIDED proposal in Queue's History tab (V27).
//
// This is the audit trail for the gate: approving is the only path from a proposal to a `wanted`
// title, so "what did we let through, when, and did anyone change it" is the question this tab
// exists to answer. Every field it shows was already persisted and, until V26/V27, invisible —
// `approvedBy` was stored and rendered nowhere, `modSummary`/`note` never left the server, and
// `approvedAt` did not exist at all (V27's migration `00016` added it, deliberately not reusing
// `updated_at`, which three subsystems write).
//
// Compact by design: a history is scanned, not read. The intent, the outcome, and when — with
// the provenance only on the rows that have it.
const ApprovalHistoryRow = ({ proposal, className }: ApprovalHistoryRowProps) => {
  const denied = proposal.status === "denied";
  const changed = !denied && Boolean(proposal.modSummary);
  const title = proposal.proposal?.intent?.description ?? "Suggested lineup";

  return (
    <li
      className={cn(
        "flex flex-col gap-1 rounded-md border border-border bg-card px-3 py-2.5 text-sm",
        className,
      )}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="min-w-0 flex-1 truncate font-medium">{title}</span>
        <Badge variant={denied ? "onair" : changed ? "caution" : "lock"}>
          {denied ? "Denied" : changed ? "Approved with changes" : "Approved"}
        </Badge>
        {/* WHEN. The ordering key the tab is sorted by, and the reason `approved_at` had to be
            its own column: a re-curation touching an approved proposal would have moved a
            timestamp borrowed from `updated_at`, silently reordering the audit trail. Denials
            carry no approval time, so the slot stays empty rather than showing a wrong one. */}
        {proposal.approvedAt && (
          <span className="shrink-0 font-mono text-2xs text-static-400">
            {formatRelative(proposal.approvedAt)}
          </span>
        )}
      </div>

      {/* WHO, and WHAT they changed. `modSummary` is generated server-side, so it records what
          actually happened rather than what the approver said happened. */}
      {(proposal.approvedBy || changed) && (
        <p className="text-muted-foreground text-xs">
          {proposal.approvedBy && (
            <span className="font-mono text-2xs uppercase tracking-wide">
              {`Approved by ${proposal.approvedBy}`}
            </span>
          )}
          {changed && <span className="text-caution">{` — ${proposal.modSummary}`}</span>}
        </p>
      )}

      {denied && proposal.denyReason && <p className="text-onair-300 text-xs">{proposal.denyReason}</p>}
    </li>
  );
};

export { ApprovalHistoryRow };
