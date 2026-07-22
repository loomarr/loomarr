import { useState } from "react";
import { Button, Checkbox, Input, Label } from "@/components/ui";
import { cn } from "@/lib";
import type { ChannelDangerZoneProps } from "./channel-danger-zone.type";

// ChannelDangerZone — the destructive-actions section (frontend-design §6: an isolated
// danger zone with typed confirmation, onair styling). Pause/resume is reversible and
// gets a plain button; delete is not, so it stays behind a confirm step the operator
// cannot fat-finger past — typing the exact channel name is what enables the final button,
// not a generic "yes/are you sure" dialog.
//
// The confirm step is local UI state only: nothing here mutates until the operator
// actually clicks "Delete permanently", so closing/canceling never partially applies.
const ChannelDangerZone = ({
  channelName,
  status,
  onPause,
  onResume,
  onDelete,
  busy,
  className,
}: ChannelDangerZoneProps) => {
  const [confirming, setConfirming] = useState(false);
  const [typed, setTyped] = useState("");
  const [purge, setPurge] = useState(false);

  const paused = status === "paused";
  const canDelete = typed === channelName;

  const cancel = () => {
    setConfirming(false);
    setTyped("");
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
          {paused ? "Paused — off air but kept." : "Take this channel off air without deleting it."}
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

      {/* Delete — irreversible, so it is gated behind typing the exact channel name. */}
      <div className="flex flex-col gap-3 border-onair-tint-15 border-t pt-4">
        <div className="flex items-center justify-between gap-3">
          <p className="text-sm">Permanently delete this channel. This can't be undone.</p>
          {!confirming && (
            <Button variant="destructive" size="sm" disabled={busy} onClick={() => setConfirming(true)}>
              Delete channel
            </Button>
          )}
        </div>

        {confirming && (
          <div className="flex flex-col gap-3 rounded-md border border-onair-tint-15 bg-background/40 p-3">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="delete-confirm-name" className="text-xs">
                {`Type "${channelName}" to confirm`}
              </Label>
              <Input
                id="delete-confirm-name"
                value={typed}
                disabled={busy}
                autoComplete="off"
                onChange={(e) => setTyped(e.target.value)}
              />
            </div>

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
              <Button
                variant="destructive"
                size="sm"
                disabled={!canDelete || busy}
                onClick={() => onDelete({ purge })}
              >
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
