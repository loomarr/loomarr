import { arrayMove } from "@dnd-kit/sortable";
import { channelsApi, type LineupEntryDTO, toProblem } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import type { ChannelLineup } from "./use-channel-lineup.type";

// A cheap identity fingerprint — just the keys, in order — for telling an actual server
// update to `current` apart from a fresh array reference thrown off by an unrelated
// parent re-render (the page passes `ch.lineup ?? []`, a new `[]` literal on every
// render whenever the field is nullish, and `ch` itself is a new object on any query
// snapshot). Comparing THIS, not the array reference, is what lets local state resync
// to a genuine change without also resyncing — and clobbering an optimistic add/
// remove/reorder mid-flight — on every incidental re-render.
//
// It deliberately ignores metadata-only changes (a title rename/year edit that leaves the
// key SET and ORDER unchanged): the editor renders only name/year, which are cosmetic here
// and re-read by-key server-side on the next PATCH regardless, so at worst a display label
// is briefly stale until the next real edit — local and server truth never diverge in a
// way that reaches Tunarr (the backend owns what plays).
const fingerprint = (entries: LineupEntryDTO[]): string => entries.map((e) => e.key).join(" ");

// useChannelLineup — the manual lineup editor's one write path (§7 PATCH, phase 3 of
// editable channels). Add/remove/reorder are three different mutations of the SAME
// idea: the backend takes a whole-list replace (`PATCH /v1/channels/{id}`, body
// `{ lineup: LineupEntryDTO[] }`) and reconciles server-side, so every op here just
// computes the next full ordered list and commits it — never a per-item endpoint.
//
// SEAMLESS, same as the rest of this feature: no save/rebuild step. Each call applies
// OPTIMISTICALLY to local state, then commits via useUpdateChannel — an add, remove, or
// drag reads back immediately rather than waiting on the round trip. A `channel` SSE
// frame elsewhere on the page, or this mutation's own onSuccess invalidate, is what
// eventually lands a genuinely updated `current` prop; the render-time check below (the
// React-documented "adjust state during rendering" pattern — no effect, so no one-frame
// flash back to stale state) is what adopts it exactly then, and only then. A failed
// commit ROLLS BACK to the pre-optimistic snapshot rather than leaving the UI showing
// something the server refused (e.g. a 422 on a duplicate key that slipped past the
// client-side check) — the toast explains why, the list explains what's actually true.
const useChannelLineup = (channelId: string, current: LineupEntryDTO[]): ChannelLineup => {
  const queryClient = useQueryClient();
  const [entries, setEntries] = useState(current);
  // The fingerprint `entries` was last synced FROM — i.e. of the `current` this hook
  // has already adopted (via the initial useState above, or a prior render's adoption
  // below). Re-deriving it from `entries` itself (rather than tracking `current`
  // separately) means an optimistic entries change is never mistaken for "current
  // changed": it only ever moves when a real adopt happens.
  const [adoptedFingerprint, setAdoptedFingerprint] = useState(() => fingerprint(current));

  if (fingerprint(current) !== adoptedFingerprint) {
    setEntries(current);
    setAdoptedFingerprint(fingerprint(current));
  }

  const update = channelsApi.useUpdateChannel({
    mutation: {
      onSuccess: () => {
        void queryClient.invalidateQueries({ queryKey: channelsApi.getGetChannelQueryKey(channelId) });
        toast.success("Saved");
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't save that change"),
    },
  });

  // Applies `next` optimistically, then fires the PATCH. `rollback` is the snapshot
  // from just before THIS call (a per-call closure — not `current`, which the render-
  // time adopt above already keeps current anyway) — a per-mutate `onError` override
  // restores exactly that on rejection, running alongside (not instead of) the hook-
  // level onError above that toasts.
  const commit = (next: LineupEntryDTO[]) => {
    const rollback = entries;
    setEntries(next);
    update.mutate({ id: channelId, data: { lineup: next } }, { onError: () => setEntries(rollback) });
  };

  return {
    entries,
    isPending: update.isPending,
    add: (entry) => {
      if (entries.some((e) => e.key === entry.key)) return false;
      commit([...entries, entry]);
      return true;
    },
    remove: (key) => commit(entries.filter((e) => e.key !== key)),
    reorder: (fromIndex, toIndex) => commit(arrayMove(entries, fromIndex, toIndex)),
    updateEntry: (key, patch) => commit(entries.map((e) => (e.key === key ? { ...e, ...patch } : e))),
  };
};

export { useChannelLineup };
