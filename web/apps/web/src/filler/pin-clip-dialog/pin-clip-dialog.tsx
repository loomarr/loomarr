import { channelsApi, fillerApi, toProblem, unwrap } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { ChannelOverridePicker } from "@/components/loomarr";
import { Button, Card } from "@/components/ui";
import type { PinClipDialogProps } from "./pin-clip-dialog.type";

// PinClipDialog — which channels use this clip (§10, V35 item 1.7).
//
// ⚠ **This replaced a pin-only dialog, and the model changed underneath it.** The old surface
// offered one action ("Pin"), which could express only two of the domain's three states —
// pinned, or nothing. Blocking a clip on one channel was unreachable from the catalog, and
// UNPINNING was too: the button read "Pinned" and did nothing.
//
// The three states are pinned (forced in ahead of the ladder), excluded (never used, and it
// WINS over pinned), and neither — the default, where the clip competes on the ladder. The
// picker owns how they are presented; this owns reading the fit and writing the policy.
//
// Two correctness points carried from the old dialog, both still load-bearing:
//   1. PATCH replaces `policy` WHOLE, so the target channel's CURRENT policy must be read and
//      the change merged onto it — sending `{filler}` alone wipes scope, audience, ordering…
//   2. The write is per channel, one at a time, so one failure cannot leave a half-applied set.
const PinClipDialog = ({ clip, onClose }: PinClipDialogProps) => {
  const queryClient = useQueryClient();
  const [busy, setBusy] = useState<string>();
  const [error, setError] = useState<string | null>(null);

  // The per-channel note. ⚠ Enabled only with a clip: the dialog renders nothing without one,
  // but the hook must still be called unconditionally (hooks cannot sit below an early return).
  const fit = fillerApi.useClipChannelFit(
    { clip: clip?.hash ?? "" },
    { query: { enabled: Boolean(clip?.hash) } },
  );
  const channels = unwrap(fit.data, (b) => b.channels) ?? [];

  const update = channelsApi.useUpdateChannel({
    mutation: {
      onSettled: () => setBusy(undefined),
      onSuccess: (_res, vars) => {
        void queryClient.invalidateQueries({ queryKey: channelsApi.getGetChannelQueryKey(vars.id) });
        void queryClient.invalidateQueries({ queryKey: channelsApi.getPreviewChannelPodsQueryKey(vars.id) });
        // The note itself changes — a pin removes the rung, an exclusion adds a reason — so the
        // fit is as stale as the channel is.
        void queryClient.invalidateQueries({ queryKey: fillerApi.getClipChannelFitQueryKey() });
      },
      onError: (e) => setError(toProblem(e).detail ?? toProblem(e).title ?? "Couldn't save that change"),
    },
  });

  if (!clip) return null;

  // Write one channel's override.
  //
  // ⚠ Reads the channel LIVE rather than trusting a list row: PATCH replaces the whole policy,
  // so merging onto a stale copy silently reverts whatever else changed since it was fetched.
  const write = async (channelId: string, next: { pinned: boolean; excluded: boolean }) => {
    setBusy(channelId);
    setError(null);
    const res = await channelsApi.getChannel(channelId);
    if (res.status !== 200) {
      setError("Couldn't read that channel");
      setBusy(undefined);
      return;
    }
    const policy = res.data.policy ?? {};
    const f = policy.filler ?? {};
    // Both lists are rebuilt from the requested state rather than toggled, so the two flags
    // cannot drift apart — the source of truth is what the caller asked for, not what was there.
    // ⚠ `string[] | null | undefined`, not just `string[]`: the generated list types are
    // nullable because the server omits an empty list, so an absent override arrives as `null`
    // rather than `[]` and a signature without it silently narrows.
    const without = (ids: string[] | null | undefined) => (ids ?? []).filter((id) => id !== clip.hash);
    const pinned = next.pinned ? [...without(f.pinned), clip.hash] : without(f.pinned);
    const excluded = next.excluded ? [...without(f.excluded), clip.hash] : without(f.excluded);

    update.mutate({
      id: channelId,
      data: { policy: { ...policy, filler: { ...f, pinned, excluded } } },
    });
  };

  return (
    <Card>
      <section aria-label={`Channels for ${clip.name}`} className="flex flex-col gap-4 p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="truncate font-semibold text-lg">Which channels play “{clip.name}”</h2>
          </div>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Close
          </Button>
        </div>

        <ChannelOverridePicker
          clipName={clip.name}
          channels={channels}
          onSet={(id, next) => void write(id, next)}
          // "Back to automatic" clears BOTH lists — the only way to reach the third state.
          onReset={(id) => void write(id, { pinned: false, excluded: false })}
          busyChannelId={update.isPending ? busy : null}
          loading={fit.isLoading}
          error={error ?? fit.error?.detail ?? null}
        />
      </section>
    </Card>
  );
};

export { PinClipDialog };
