import { Check, Loader2, X } from "lucide-react";
import { Badge, Button, Card } from "@/components/ui";
import { cn } from "@/lib";
import type { ApprovalQueueItemProps } from "./approval-queue-item.type";

// ApprovalQueueItem — the admin approval row (§3, §7 approval gate). One glance: what
// was asked, by whom, how many acquisitions it implies; two actions: approve or deny.
// `approving` disables both and spins; `denied` shows the reason and retires the
// actions. Members never see this — the gate is admin-only (§11), enforced server-side.
const ApprovalQueueItem = ({
  title,
  requestedBy,
  summary,
  acquisitions,
  status = "pending",
  denyReason,
  onApprove,
  onDeny,
  className,
}: ApprovalQueueItemProps) => {
  const busy = status === "approving";
  const denied = status === "denied";
  return (
    <Card className={cn("flex items-start gap-4 p-4", className)}>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <p className="font-medium">{title}</p>
          {typeof acquisitions === "number" && acquisitions > 0 && (
            <Badge variant="tune">{`${acquisitions} to acquire`}</Badge>
          )}
          {denied && <Badge variant="onair">Denied</Badge>}
        </div>
        {summary && <p className="mt-1 text-muted-foreground text-sm">{summary}</p>}
        {requestedBy && (
          <p className="mt-1 font-mono text-static-400 text-xs">{`Requested by ${requestedBy}`}</p>
        )}
        {denied && denyReason && <p className="mt-1 text-onair-300 text-sm">{denyReason}</p>}
      </div>

      {!denied && (
        <div className="flex shrink-0 gap-2">
          <Button variant="outline" size="sm" onClick={onDeny} disabled={busy}>
            <X aria-hidden />
            Deny
          </Button>
          <Button size="sm" onClick={onApprove} disabled={busy}>
            {busy ? <Loader2 className="animate-spin" aria-hidden /> : <Check aria-hidden />}
            Approve
          </Button>
        </div>
      )}
    </Card>
  );
};

export { ApprovalQueueItem };
