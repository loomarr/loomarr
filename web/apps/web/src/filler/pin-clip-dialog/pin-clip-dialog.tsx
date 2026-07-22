import { channelsApi, toProblem } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { Check } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { ErrorState } from "@/components/loomarr";
import { Button, Card } from "@/components/ui";
import type { PinClipDialogProps } from "./pin-clip-dialog.type";

// PinClipDialog — the catalog → channel bridge (P3 cohesion): pin one clip into a channel's
// "always include" list straight from the Filler catalog, without opening that channel.
// This is the SAME write the channel page's Apply does — append the clip id to
// `policy.filler.pinned` and PATCH — just without the draft sandbox (you're doing one thing,
// not iterating). Two correctness points carried from the channel page:
//   1. PATCH replaces `policy` WHOLE, so we must read the target channel's CURRENT policy
//      and merge the pin onto it (never send `{filler}` alone — it would wipe scope/etc).
//   2. Pinning is idempotent: a clip already pinned on that channel is a no-op, surfaced as
//      "already pinned" rather than a duplicate id.
// Inline Card, same open/close idiom as ClipTagDialog (not a fixed overlay).
const PinClipDialog = ({ clip, onClose }: PinClipDialogProps) => {
  const queryClient = useQueryClient();
  const channels = channelsApi.useListChannels();
  // The channel currently being pinned to (its row shows a spinner) — one PATCH at a time.
  const [pinning, setPinning] = useState<string>();

  const update = channelsApi.useUpdateChannel({
    mutation: {
      onSuccess: (_res, vars) => {
        void queryClient.invalidateQueries({ queryKey: channelsApi.getGetChannelQueryKey(vars.id) });
        void queryClient.invalidateQueries({ queryKey: channelsApi.getPreviewChannelPodsQueryKey(vars.id) });
        toast.success("Pinned to channel");
        onClose();
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't pin the clip"),
      onSettled: () => setPinning(undefined),
    },
  });

  if (!clip) return null;

  const rows = channels.data?.status === 200 ? (channels.data.data.channels ?? []) : [];

  // Pin `clip` into `channelId`: fetch that channel's live policy, append the id to
  // filler.pinned (dedup), and PATCH the WHOLE merged policy. Reading the channel here
  // (rather than trusting the list row, which carries policy but may be stale) keeps the
  // merge honest.
  const pinTo = async (channelId: string) => {
    setPinning(channelId);
    const res = await channelsApi.getChannel(channelId);
    if (res.status !== 200) {
      toast.error("Couldn't read that channel");
      setPinning(undefined);
      return;
    }
    const policy = res.data.policy ?? {};
    const pinned = policy.filler?.pinned ?? [];
    if (pinned.includes(clip.tunarrProgramId)) {
      toast.info(`Already pinned to ${res.data.name}`);
      setPinning(undefined);
      onClose();
      return;
    }
    update.mutate({
      id: channelId,
      data: {
        policy: { ...policy, filler: { ...policy.filler, pinned: [...pinned, clip.tunarrProgramId] } },
      },
    });
  };

  return (
    <Card>
      <section aria-label={`Pin ${clip.name} to a channel`} className="flex flex-col gap-4 p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="truncate font-semibold text-lg">Pin “{clip.name}” to a channel</h2>
            <p className="mt-1 text-muted-foreground text-sm">
              It'll always be available in that channel's breaks, ahead of the theme. Fine-tune the rest on
              the channel's Filler section.
            </p>
          </div>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Close
          </Button>
        </div>

        {channels.error != null && <ErrorState error={channels.error} onRetry={() => channels.refetch()} />}

        {rows.length === 0 ? (
          <p className="text-muted-foreground text-sm">No channels yet — create one first.</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {rows.map((ch) => {
              const already = (ch.policy?.filler?.pinned ?? []).includes(clip.tunarrProgramId);
              return (
                <li
                  key={ch.id}
                  className="flex items-center gap-3 rounded-md border border-border bg-card px-3 py-2"
                >
                  <div className="min-w-0 flex-1">
                    <span className="truncate font-medium text-sm">{ch.name}</span>
                    <span className="ml-2 font-mono text-static-400 text-xs">Ch {ch.number}</span>
                  </div>
                  {already ? (
                    <span className="flex items-center gap-1 text-muted-foreground text-xs">
                      <Check className="size-3.5" aria-hidden />
                      Pinned
                    </span>
                  ) : (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={pinning !== undefined}
                      onClick={() => void pinTo(ch.id)}
                    >
                      {pinning === ch.id ? "Pinning…" : "Pin"}
                    </Button>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </Card>
  );
};

export { PinClipDialog };
