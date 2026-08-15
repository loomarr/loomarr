import { useState } from "react";
import { useDeleteConfirm } from "@/channels/use-delete-confirm";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import type { ChannelDangerZoneProps } from "./channel-danger-zone.type";

// ChannelDangerZone — the destructive-actions section (frontend-design §6: an isolated
// danger zone, onair styling). Pause/resume is reversible and gets a plain button; delete is
// not, so it stays behind a two-step confirm the operator can't fat-finger past — one click
// arms it, a second executes. (Previously this required typing the exact channel name; that
// was tedious for a household app, so it's now a plain confirm step.)
//
// The confirm step is local UI state only: nothing here mutates until the operator actually
// clicks "Delete permanently", so closing/canceling never partially applies.
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
  const [purge, setPurge] = useState(false);

  const paused = status === "paused";

  const cancel = () => {
    cancelConfirm();
    setPurge(false);
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

      {/* Delete — irreversible, so it is gated behind a two-step confirm (arm, then execute). */}
      <div className="flex flex-col gap-3 border-onair-tint-15 border-t pt-4">
        <div className="flex items-center justify-between gap-3">
          <p className="text-sm">Permanently delete this channel. This can't be undone.</p>
          {!confirming && (
            <Button variant="destructive" size="sm" disabled={busy} onClick={arm}>
              Delete channel
            </Button>
          )}
        </div>

        {confirming && (
          <div className="flex flex-col gap-3 rounded-md border border-onair-tint-15 bg-background/40 p-3">
            <p className="text-sm">
              Delete <span className="font-medium">{channelName}</span> for good? This can't be undone.
            </p>

            <div className="flex items-center gap-2">
              <Checkbox
                id="delete-confirm-purge"
                checked={purge}
                disabled={busy}
                onChange={(e) => setPurge(e.target.checked)}
              />
              <Label htmlFor="delete-confirm-purge" className="text-muted-foreground text-xs">
                Also remove it from Tunarr
              </Label>
            </div>

            <div className="flex items-center gap-2">
              <Button variant="destructive" size="sm" disabled={busy} onClick={() => onDelete({ purge })}>
                Delete permanently
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
