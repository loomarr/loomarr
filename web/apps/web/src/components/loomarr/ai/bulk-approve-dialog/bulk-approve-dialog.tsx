import { pluralize } from "@loomarr/core/format";
import { approvalSizeLine } from "@loomarr/core/requests";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogTitle } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import type {
  BulkApproveConsequences,
  BulkApproveDialogProps,
  BulkApproveOutcome,
  ChannelRequestConsequence,
  FillerDownloadConsequence,
} from "./bulk-approve-dialog.type";

// BulkApproveDialog — what a group's bulk approve will DO, shown before it happens, then what it
// did (#1817, mock #1876). Two steps in one modal: the consequence summary with Cancel / Approve
// N, and the per-item result with a Retry on each failure.
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

// ⚠ An estimate, rendered as one (PullCard's rule): "Loomarr said 40 and downloaded 12" is
// what an unmarked figure becomes. 0 means unmeasured, so it is not shown as "~0 clips".
const fillerEstimateLine = (item: FillerDownloadConsequence) => {
  const sources = pluralize(item.sources, "source");
  return item.estimateClips > 0
    ? `~${item.estimateClips} clips from ${sources} (estimate — may yield fewer once fetched)`
    : `From ${sources} (no clip estimate yet)`;
};

// Filler pulls carry no size until a pull producer returns (beta.11) — an empty item list is
// exactly "nothing known", which the shared rule words as unavailable.
const FILLER_SIZE_LINE = approvalSizeLine([], "downloads");

const ConsequenceItem = ({ children }: { children: React.ReactNode }) => (
  <li className="flex flex-col gap-1 rounded-md border border-border px-3 py-2.5">{children}</li>
);

const Consequences = ({ consequences }: { consequences: BulkApproveConsequences }) =>
  consequences.group === "channels" ? (
    <ul className="flex flex-col gap-2">
      {consequences.items.map((item) => (
        <ConsequenceItem key={item.id}>
          <span className="font-medium text-sm">{item.title}</span>
          <span className="text-muted-foreground text-xs">{metaLine(item)}</span>
          <span className="text-muted-foreground text-xs">{approvalSizeLine(item.sizes, "titles")}</span>
          <span className="text-tune text-xs">{channelLine(item)}</span>
        </ConsequenceItem>
      ))}
    </ul>
  ) : (
    <ul className="flex flex-col gap-2">
      {consequences.items.map((item) => (
        <ConsequenceItem key={item.id}>
          <span className="font-medium text-sm">{item.title}</span>
          <span className="text-muted-foreground text-xs">{fillerEstimateLine(item)}</span>
          <span className="text-muted-foreground text-xs">{FILLER_SIZE_LINE}</span>
          {/* Legacy pulls name a gap but no channel, and nothing records channels for new
              ones yet — so "unavailable", never a guessed channel and never "General catalog
              variety", which is only true when a producer recorded that no gap was filled. */}
          <span className="text-muted-foreground text-xs">Channel: unavailable</span>
        </ConsequenceItem>
      ))}
    </ul>
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

// Across the whole selection, with the same honesty rule as one item: a sum over the known
// sizes is labelled with how many it covers.
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

const BulkApproveDialog = ({
  open,
  consequences,
  results,
  pending = false,
  error,
  onCancel,
  onConfirm,
  onRetry,
  onDone,
  portalContainer,
}: BulkApproveDialogProps) => {
  const copy = GROUP_COPY[consequences.group];
  const count = consequences.items.length;
  const titleOf = (id: string) => consequences.items.find((i) => i.id === id)?.title ?? id;
  const failed = (results ?? []).filter((r) => !r.ok);

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        // Escape / the close button mid-call would orphan a request whose result nobody sees.
        if (next || pending) return;
        if (results) onDone();
        else onCancel();
      }}
    >
      <DialogContent
        portalContainer={portalContainer}
        className="max-h-[85vh] w-[calc(100%-2rem)] max-w-120 gap-3.5 overflow-y-auto p-5"
      >
        {results ? (
          <>
            <DialogTitle className="pr-8">{`Approved ${results.filter((r) => r.ok).length} of ${results.length}`}</DialogTitle>
            <ul className="flex flex-col gap-1.5" aria-live="polite">
              {results.map((r) => (
                <li
                  key={r.id}
                  className={cn(
                    "flex items-start gap-2 rounded-md border px-2.5 py-2 text-sm",
                    r.ok ? "border-border" : "border-onair/40 bg-onair/5",
                  )}
                >
                  <span
                    aria-hidden
                    className={cn("shrink-0 font-bold", r.ok ? "text-lock" : "text-onair-300")}
                  >
                    {r.ok ? "✓" : "✕"}
                  </span>
                  <div className="min-w-0 flex-1">
                    <div>
                      <span className="sr-only">{r.ok ? "Approved: " : "Not approved: "}</span>
                      {titleOf(r.id)}
                    </div>
                    <div className="text-muted-foreground text-xs">{outcomeLine(consequences, r)}</div>
                  </div>
                  {!r.ok && (
                    <Button
                      variant="outline"
                      size="sm"
                      className="ml-auto shrink-0"
                      disabled={pending}
                      aria-label={`Retry ${titleOf(r.id)}`}
                      onClick={() => onRetry([r.id])}
                    >
                      Retry
                    </Button>
                  )}
                </li>
              ))}
            </ul>
            {error != null && <ErrorState error={error} />}
            {failed.length > 0 && (
              <Button variant="outline" disabled={pending} onClick={() => onRetry(failed.map((r) => r.id))}>
                {`Retry ${failed.length} failed`}
              </Button>
            )}
            <DialogFooter className="mt-1">
              <Button disabled={pending} onClick={onDone}>
                Done
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogTitle className="pr-8">{`Approve ${pluralize(count, copy.noun)}?`}</DialogTitle>
            <DialogDescription>
              Scope: this approves only the <strong className="text-foreground">{copy.group}</strong> group.
              Items selected in the other group are untouched.
            </DialogDescription>
            <Consequences consequences={consequences} />
            <p className="text-sm">{summaryLine(consequences)}</p>
            <p className="text-sm">{selectionSizeLine(consequences)}</p>
            {error != null && <ErrorState error={error} />}
            <DialogFooter className="mt-1">
              <Button variant="outline" disabled={pending} onClick={onCancel}>
                Cancel
              </Button>
              <Button disabled={pending} onClick={onConfirm}>
                {pending ? "Approving…" : `Approve ${count}`}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
};

export { BulkApproveDialog };
