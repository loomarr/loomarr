import { useState } from "react";
import { useDeleteConfirm } from "@/channels/use-delete-confirm";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ChannelDangerZoneProps } from "./channel-danger-zone.type";

// ChannelDangerZone — the destructive-actions section (frontend-design §6: an isolated
// danger zone, onair styling). Pause/resume is reversible and gets a plain button. Detach and
// purge have different consequences, so they are separate actions rather than one permanent-delete
// claim with a checkbox: detach keeps both records; purge removes Loomarr's record and any retained
// Tunarr projection (§7). Both stay behind a two-step confirm the operator can't fat-finger past.
//
// The confirm step is local UI state only: nothing here mutates until the operator actually
// clicks the second action button, so closing/canceling never partially applies.
const ChannelDangerZone = ({
  channelName,
  status,
  onPause,
  onResume,
  onDelete,
  busy,
  className,
}: ChannelDangerZoneProps) => {
  const { confirming, arm, cancel: cancelConfirm } = useDeleteConfirm();
  const [removal, setRemoval] = useState<"detach" | "purge" | null>(null);

  const paused = status === "paused";

  const cancel = () => {
    cancelConfirm();
    setRemoval(null);
  };
  const chooseRemoval = (next: "detach" | "purge") => {
    setRemoval(next);
    arm();
  };

  return (
    <div
      className={cn(
        "flex flex-col gap-4 rounded-lg border border-onair-tint-15 bg-onair-tint-15 p-4",
        className,
      )}
    >
      <div>
        <h3 className="font-medium text-onair-300 text-sm">Danger zone</h3>
        <p className="mt-1 text-muted-foreground text-xs">
          These actions affect whether {channelName} broadcasts at all.
        </p>
      </div>

      {/* Pause/resume — fully reversible, so it is a single click with no confirm step. */}
      <div className="flex items-center justify-between gap-3 border-onair-tint-15 border-t pt-4">
        <p className="text-sm">
          {paused ? "Paused: off air but kept." : "Take this channel off air without deleting it."}
        </p>
        {paused ? (
          <Button variant="outline" size="sm" disabled={busy} onClick={onResume}>
            Resume
          </Button>
        ) : (
          <Button variant="outline" size="sm" disabled={busy} onClick={onPause}>
            Pause
          </Button>
        )}
      </div>

      {/* Detach and purge are distinct server operations (§7), never a checkbox under one "delete"
          label. The confirmation step replaces both choices so it has one unambiguous action. */}
      <div className="flex flex-col gap-3 border-onair-tint-15 border-t pt-4">
        {!confirming ? (
          <div className="flex flex-col gap-4">
            <div className="flex items-center justify-between gap-3">
              <div className="flex flex-col gap-1">
                <p className="text-sm">Stop managing this channel in Loomarr.</p>
                <p className="text-muted-foreground text-xs">
                  Loomarr keeps its record and leaves any Tunarr channel in place.
                </p>
              </div>
              <Button variant="outline" size="sm" disabled={busy} onClick={() => chooseRemoval("detach")}>
                Stop managing
              </Button>
            </div>

            <div className="flex items-center justify-between gap-3">
              <div className="flex flex-col gap-1">
                <p className="text-sm">
                  Permanently delete Loomarr's record and any retained Tunarr channel.
                </p>
                <p className="text-onair-300 text-xs">This can't be undone.</p>
              </div>
              <Button variant="destructive" size="sm" disabled={busy} onClick={() => chooseRemoval("purge")}>
                Delete from Loomarr and Tunarr
              </Button>
            </div>
          </div>
        ) : (
          <div className="flex flex-col gap-3 rounded-md border border-onair-tint-15 bg-background/40 p-3">
            {removal === "detach" ? (
              <p className="text-sm">
                Stop managing {channelName}? Loomarr will keep its record and leave any Tunarr channel in
                place.
              </p>
            ) : (
              <>
                <p className="text-sm">Delete {channelName} from Loomarr and Tunarr for good?</p>
                <p className="text-onair-300 text-xs">This can't be undone.</p>
              </>
            )}

            <div className="flex items-center gap-2">
              <Button
                variant={removal === "purge" ? "destructive" : "outline"}
                size="sm"
                disabled={busy}
                onClick={() => onDelete({ purge: removal === "purge" })}
              >
                {removal === "purge" ? "Delete from Loomarr and Tunarr" : "Stop managing"}
              </Button>
              <Button variant="ghost" size="sm" disabled={busy} onClick={cancel}>
                Cancel
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

export { ChannelDangerZone };
