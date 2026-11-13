import { Action, Stack, Text } from "@loomarr/design-system";
import { useState } from "react";
import { useDeleteConfirm } from "@/channels/use-delete-confirm";
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
    <Stack className={cn("border-onair-tint-15 bg-onair-tint-15 rounded-lg border p-4", className)} gap="4">
      <div>
        <Text as="h3" textRole="heading" tone="danger">
          Danger zone
        </Text>
        <Text as="p" className="mt-1" textRole="hint" tone="secondary">
          These actions affect whether {channelName} broadcasts at all.
        </Text>
      </div>

      {/* Pause/resume — fully reversible, so it is a single click with no confirm step. */}
      <Stack
        align="center"
        className="border-onair-tint-15 border-t pt-4"
        direction="row"
        gap="control"
        justify="between"
      >
        <Text as="p" textRole="compact" tone="primary">
          {paused ? "Paused: off air but kept." : "Take this channel off air without deleting it."}
        </Text>
        {paused ? (
          <Action disabled={busy} onPress={onResume} variant="outline">
            Resume
          </Action>
        ) : (
          <Action disabled={busy} onPress={onPause} variant="outline">
            Pause
          </Action>
        )}
      </Stack>

      {/* Detach and purge are distinct server operations (§7), never a checkbox under one "delete"
          label. The confirmation step replaces both choices so it has one unambiguous action.
          The words are the web mock's. ⚠ Its detach line ("It keeps playing as it is now") holds
          only for a Tunarr-streamed channel: a detached channel Loomarr plays itself goes off
          air (api/channelplayurl.go). Until the maintainer settles it, this row keeps the line
          that is true for both. */}
      <Stack className="border-onair-tint-15 border-t pt-4" gap="control">
        {!confirming ? (
          <Stack gap="4">
            <Stack align="center" direction="row" gap="control" justify="between">
              <Stack flexShrink={1} gap="1">
                <Text as="p" textRole="compact" tone="primary">
                  Stop Loomarr from updating this channel.
                </Text>
                <Text as="p" textRole="hint" tone="secondary">
                  Loomarr keeps its record and leaves any Tunarr channel in place.
                </Text>
              </Stack>
              <div className="shrink-0">
                <Action disabled={busy} onPress={() => chooseRemoval("detach")} variant="outline">
                  Stop updating
                </Action>
              </div>
            </Stack>

            <Stack align="center" direction="row" gap="control" justify="between">
              <Stack flexShrink={1} gap="1">
                <Text as="p" textRole="compact" tone="primary">
                  Delete this channel from Loomarr, and from Tunarr if it's there.
                </Text>
                <Text as="p" textRole="hint" tone="danger">
                  This can't be undone.
                </Text>
              </Stack>
              <div className="shrink-0">
                <Action
                  disabled={busy}
                  onPress={() => chooseRemoval("purge")}
                  tone="danger"
                  variant="outline"
                >
                  Delete channel
                </Action>
              </div>
            </Stack>
          </Stack>
        ) : (
          <Stack className="border-onair-tint-15 bg-background/40 rounded-md border p-3" gap="control">
            {removal === "detach" ? (
              <Text as="p" textRole="compact" tone="primary">
                Stop updating {channelName}? Loomarr will keep its record and leave any Tunarr channel in
                place.
              </Text>
            ) : (
              <>
                <Text as="p" textRole="compact" tone="primary">
                  Delete {channelName} from Loomarr, and from Tunarr if it's there?
                </Text>
                <Text as="p" textRole="hint" tone="danger">
                  This can't be undone.
                </Text>
              </>
            )}

            <Stack align="center" direction="row" gap="inline">
              <Action
                disabled={busy}
                onPress={() => onDelete({ purge: removal === "purge" })}
                tone={removal === "purge" ? "danger" : undefined}
                variant="outline"
              >
                {removal === "purge" ? "Delete channel" : "Stop updating"}
              </Action>
              <Action disabled={busy} onPress={cancel} variant="ghost">
                Cancel
              </Action>
            </Stack>
          </Stack>
        )}
      </Stack>
    </Stack>
  );
};

export { ChannelDangerZone };
