import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { FillerSelection } from "@loomarr/api/models/fillerSelection";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import type { ChannelFillerDraft, FillerDraft } from "./use-channel-filler-draft.type";

// How long we let the criteria settle before re-assembling. The controls are commit-on-
// blur (era) and instant (selects/checkboxes/chips), so a few hundred ms coalesces a
// burst of toggles into one preview POST rather than one-per-click — cheap, and it keeps
// the timeline from flickering as you tick several categories in a row.
const PREVIEW_DEBOUNCE_MS = 300;

// canonicalize — a stable string identity for a selection, used for BOTH the dirty check
// and the debounce key. It folds the "empty means any" equivalences so clearing a list to
// [] reads identical to never having set it (undefined), and sorts the id/tag arrays so a
// reorder that doesn't change membership isn't mistaken for an edit. Order of the six keys
// is fixed here, not by object-key order, so the output is deterministic.
const canonicalize = (s: FillerDraft): string => {
  const list = (v?: string[] | null): string[] => [...(v ?? [])].sort();
  const era = s.era && (s.era.from != null || s.era.to != null) ? s.era : undefined;
  return JSON.stringify({
    era: era ? { from: era.from ?? null, to: era.to ?? null } : null,
    audience: s.audience || null,
    categories: list(s.categories),
    kinds: list(s.kinds),
    pinned: list(s.pinned),
    excluded: list(s.excluded),
  });
};

// useChannelFillerDraft — the live filler sandbox (§10, §12) for one channel. It holds a
// DRAFT filler selection, break frequency and break length in local state (seeded from the saved policy)
// and, on every settled change, re-assembles the actual break through POST …/pods/preview
// — the SAME assembler + seed reconcile uses — so what you see is exactly what will air.
// Nothing is saved until `apply`, which PATCHes both choices onto the policy; the
// existing onSuccess → invalidate → SSE loop reconciles from there (seamless for the
// effect, draft/apply for the authoring — the one deliberate exception, §10).
//
// This is a MUTATION rendered like a query: a draft has no URL identity to cache on (it
// isn't saved), so preview rides a POST whose response is the preview. Contrast
// usePreviewChannelPods, which reads the SAVED selection via GET.
//
// It takes the WHOLE `policy`, not just `policy.filler`: PATCH /channels replaces the
// policy object whole (only reconcile-owned `applied` is preserved server-side), so apply
// must merge onto the policy or it would wipe scope/audience/separation/ordering.
const useChannelFillerDraft = (
  channelId: string,
  policy: ChannelPolicy | undefined,
  revision: number,
): ChannelFillerDraft => {
  const queryClient = useQueryClient();
  const saved: FillerSelection | undefined = policy?.filler;
  const savedBreaksPerHour = policy?.breaksPerHour;
  const savedBreakDuration = policy?.breakDuration;
  // The saved selection this draft was seeded from, as a canonical string — its identity,
  // not its reference (the page hands a fresh `ch.policy.filler` object every render). We
  // resync the draft to `saved` only when THIS changes: a genuine server update (an apply
  // landing, a re-approval reseeding the era) adopts, an incidental re-render does not —
  // the same render-time "adjust state" pattern useChannelLineup uses, no effect/flash.
  const savedSelectionKey = canonicalize(saved ?? {});
  const savedKey = `${savedSelectionKey}|breaks:${savedBreaksPerHour === undefined ? "default" : savedBreaksPerHour}|duration:${savedBreakDuration ?? "default"}`;
  const [draft, setDraft] = useState<FillerDraft>(saved ?? {});
  const [breaksPerHour, setBreaksPerHour] = useState<number | undefined>(savedBreaksPerHour);
  const [breakDuration, setBreakDuration] = useState<string | undefined>(savedBreakDuration);
  const [adoptedKey, setAdoptedKey] = useState(savedKey);
  if (savedKey !== adoptedKey) {
    setDraft(saved ?? {});
    setBreaksPerHour(savedBreaksPerHour);
    setBreakDuration(savedBreakDuration);
    setAdoptedKey(savedKey);
  }

  const preview = channelsApi.usePreviewDraftChannelPods();
  const draftKey = canonicalize(draft);

  // Debounced re-assemble: fire a preview whenever the canonical draft settles. Keying the
  // effect on `draftKey` (not the object) means editing a field to the same value, or a
  // parent re-render, doesn't re-POST. The timer is cleared on the next change so only the
  // settled draft is sent. preview.mutate is stable across renders (react-query); depending
  // on the canonical draft + channel is the whole intent — re-preview exactly when the
  // selection changes.
  // biome-ignore lint/correctness/useExhaustiveDependencies: draftKey IS the dependency
  useEffect(() => {
    const t = setTimeout(() => {
      preview.mutate({
        id: channelId,
        data: { filler: JSON.parse(draftKey) as FillerSelection, breakDuration },
      });
    }, PREVIEW_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [channelId, draftKey, breakDuration]);

  const update = channelsApi.useUpdateChannel({
    mutation: {
      onSuccess: () => {
        void queryClient.invalidateQueries({ queryKey: channelsApi.getGetChannelQueryKey(channelId) });
        // The SAVED preview (GET …/pods, read elsewhere) is NOT under the channel key, so
        // invalidate it explicitly — otherwise the saved break shown after apply would lag
        // the change we just made.
        void queryClient.invalidateQueries({
          queryKey: channelsApi.getPreviewChannelPodsQueryKey(channelId),
        });
        toast.success("Filler updated");
      },
      onError: (e) => {
        void queryClient.invalidateQueries({ queryKey: channelsApi.getGetChannelQueryKey(channelId) });
        toast.error(toProblem(e).title ?? "Couldn't update filler");
      },
    },
  });

  const body = unwrap(preview.data);

  return {
    draft,
    setDraft,
    breaksPerHour,
    setBreaksPerHour,
    breakDuration,
    setBreakDuration,
    preview: body,
    isPreviewing: preview.isPending,
    // The transport mutator THROWS an ApiError on non-2xx for mutations (unlike a query,
    // which would carry the non-200 in data.status) — so a failed preview lands in
    // `preview.error`, not `preview.data`. Surfaced so a broken draft reads as an error,
    // not an empty pool.
    previewError: preview.error,
    isDirty:
      draftKey !== savedSelectionKey ||
      breaksPerHour !== savedBreaksPerHour ||
      breakDuration !== savedBreakDuration,
    // Merge onto the saved policy — PATCH replaces `policy` whole, so we must carry the
    // rest of it (scope/audience/separation/ordering) alongside the new filler.
    apply: () =>
      update.mutate({
        id: channelId,
        data: { revision, policy: { ...policy, filler: draft, breaksPerHour, breakDuration } },
      }),
    isApplying: update.isPending,
    discard: () => {
      setDraft(saved ?? {});
      setBreaksPerHour(savedBreaksPerHour);
      setBreakDuration(savedBreakDuration);
    },
  };
};

export { canonicalize, PREVIEW_DEBOUNCE_MS, useChannelFillerDraft };
