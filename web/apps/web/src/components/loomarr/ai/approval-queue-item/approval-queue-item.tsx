import type { ProposalItem } from "@loomarr/api";
import { Check, ChevronDown, Loader2, ShieldAlert, X } from "lucide-react";
import { useId, useState } from "react";
import { Badge, Button, Card, Input } from "@/components/ui";
import { cn } from "@/lib";
import { ProposalEdit } from "../proposal-edit";
import type { ApprovalQueueItemProps } from "./approval-queue-item.type";

// seasonWindowLabel renders a series' airing season window (§8) as a human chip so a
// reviewer can see "classic X → Seasons 1–10" before approving. null = unbounded.
const seasonWindowLabel = (min?: number, max?: number): string | null => {
  const lo = min ?? 0;
  const hi = max ?? 0;
  if (lo <= 0 && hi <= 0) return null;
  if (lo > 0 && hi > 0) return lo === hi ? `Season ${lo}` : `Seasons ${lo}–${hi}`;
  if (lo > 0) return `From season ${lo}`;
  return `Through season ${hi}`;
};

const PickRow = ({ item, kind }: { item: ProposalItem; kind: "lineup" | "acquire" }) => {
  const window = seasonWindowLabel(item.seasonMin, item.seasonMax);
  return (
    <li className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-card px-2.5 py-1.5 text-sm">
      <span className="font-medium">{item.name}</span>
      {item.year ? <span className="font-mono text-static-400 text-xs">{item.year}</span> : null}
      <Badge variant={kind === "lineup" ? "lock" : "tune"}>
        {kind === "lineup" ? "In library" : "Will acquire"}
      </Badge>
      {window && <Badge variant="tune">{window}</Badge>}
    </li>
  );
};

// ApprovalQueueItem — the admin approval row (§3, §7 approval gate). One glance: what
// was asked, by whom, how many acquisitions it implies; two actions: approve or deny.
// A "Show picks" toggle reveals the grounded lineup (titles, in-library vs. acquire,
// season windows) so an admin can REVIEW what they're approving before it goes live —
// otherwise the gate is blind. `approving` disables both and spins; `denied` shows the
// reason. Members never see this — the gate is admin-only (§11), enforced server-side.
const ApprovalQueueItem = ({
  title,
  requestedBy,
  summary,
  acquisitions,
  status = "pending",
  denyReason,
  lineup,
  acquisitionItems,
  refused,
  onEdit,
  onApprove,
  onDeny,
  className,
}: ApprovalQueueItemProps) => {
  const busy = status === "approving";
  const denied = status === "denied";
  const [open, setOpen] = useState(false);
  const [denying, setDenying] = useState(false);
  const [reason, setReason] = useState("");
  const reasonId = useId();
  const hasPicks = (lineup?.length ?? 0) + (acquisitionItems?.length ?? 0) > 0;

  return (
    <Card className={cn("flex flex-col gap-3 p-4", className)}>
      <div className="flex items-start gap-4">
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

        {!denied && !denying && (
          <div className="flex shrink-0 gap-2">
            <Button variant="outline" size="sm" onClick={() => setDenying(true)} disabled={busy}>
              <X aria-hidden />
              Deny
            </Button>
            <Button size="sm" onClick={onApprove} disabled={busy}>
              {busy ? <Loader2 className="animate-spin" aria-hidden /> : <Check aria-hidden />}
              Approve
            </Button>
          </div>
        )}
      </div>

      {/* Deny arms this row rather than firing straight away. The reason is optional —
          requiring one would turn every decline into a chore — but offering it is what
          stops a member from re-submitting the same intent, having learned nothing. */}
      {denying && (
        <div className="flex flex-col gap-2 border-border border-t pt-2">
          <label className="text-muted-foreground text-sm" htmlFor={reasonId}>
            Why not? Optional: the requester sees this.
          </label>
          <Input
            id={reasonId}
            autoFocus
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="e.g. over the acquisition cap this week, ask again Monday"
            disabled={busy}
          />
          <div className="flex justify-end gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setDenying(false);
                setReason("");
              }}
              disabled={busy}
            >
              Cancel
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => onDeny?.(reason.trim() || undefined)}
              disabled={busy}
            >
              <X aria-hidden />
              Deny
            </Button>
          </div>
        </div>
      )}

      {/* ⚠ OUTSIDE the picks disclosure, and above it. What a reviewer is approving changed:
          these titles were proposed and will not play. Before this, the card offered them
          silently and the §4 gate dropped them after approval — so an admin authorised seven
          titles, got five, and nothing anywhere said which two or why (#259). A fact that
          changes the meaning of the button is not detail behind a toggle. */}
      {refused && refused.length > 0 && (
        <div className="flex flex-col gap-1.5 rounded-md border border-signal/40 bg-signal/5 px-3 py-2">
          <p className="flex items-center gap-1.5 text-sm">
            <ShieldAlert className="size-4 shrink-0 text-signal" aria-hidden />
            <span className="font-medium">
              {refused.length} {refused.length === 1 ? "title" : "titles"} won't be included
            </span>
          </p>
          <ul className="flex flex-col gap-0.5">
            {refused.map((r) => (
              <li key={`refused-${r.item.name}-${r.item.tmdbId ?? ""}`} className="text-sm">
                <span>{r.item.name}</span>{" "}
                <span className="text-muted-foreground">
                  {/* The reason is spelled out rather than shown as a code: the reviewer's
                      next action (raise the ceiling, or accept the shorter lineup) depends on
                      knowing it was the audience rule and not a missing file. */}
                  — rated {r.item.officialRating || "unknown"}, above this channel's audience limit
                </span>
              </li>
            ))}
          </ul>
          <p className="text-muted-foreground text-xs">
            Approving adds everything else. To include these, raise the channel's audience limit after it's
            created.
          </p>
        </div>
      )}

      {hasPicks && (
        <div className="border-border border-t pt-2">
          <button
            type="button"
            className="flex items-center gap-1 text-muted-foreground text-sm hover:text-foreground"
            onClick={() => setOpen((v) => !v)}
            aria-expanded={open}
          >
            <ChevronDown className={cn("size-4 transition-transform", open && "rotate-180")} aria-hidden />
            {open ? "Hide picks" : onEdit ? "Review & edit picks" : "Show picks"}
          </button>
          {open &&
            // With an edit handler the disclosure IS the edit surface (V25b) — the same list,
            // with drop/add/note. Without one it stays read-only, so every other caller and the
            // member-facing views are unchanged.
            (onEdit ? (
              <ProposalEdit
                className="mt-2"
                lineup={lineup ?? []}
                acquisitions={acquisitionItems ?? []}
                disabled={status === "approving"}
                onChange={onEdit}
              />
            ) : (
              <ul className="mt-2 flex flex-col gap-1.5">
                {lineup?.map((it) => (
                  <PickRow key={`lib-${it.name}`} item={it} kind="lineup" />
                ))}
                {acquisitionItems?.map((it) => (
                  <PickRow key={`acq-${it.name}`} item={it} kind="acquire" />
                ))}
              </ul>
            ))}
        </div>
      )}
    </Card>
  );
};

export { ApprovalQueueItem };
