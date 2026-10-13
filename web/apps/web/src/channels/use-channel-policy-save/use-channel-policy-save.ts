import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import { ApiError, toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { toast } from "sonner";
import type { ChannelPolicySave } from "./use-channel-policy-save.type";

// A 409 means another writer (a reconcile, a sweep, a second admin) saved the channel first. Each
// one is rebased and retried; a run of them this long means the channel is changing under the
// editor faster than it can save, and the operator hears about it instead.
const MAX_CONFLICT_RETRIES = 3;

const same = (a: unknown, b: unknown): boolean => JSON.stringify(a) === JSON.stringify(b);

// rebase re-applies the operator's edits onto a policy another writer changed. An edit is a
// top-level policy field whose value differs from the saved policy the edits were made against
// (`base`); only those fields take the operator's value, and every other field keeps the fresh
// one. A field the operator removed (an opt-out such as `autoCurate`) is removed again: PATCH
// replaces the policy wholesale, so absence is the clear.
const rebase = (fresh: ChannelPolicy, base: ChannelPolicy, desired: ChannelPolicy): ChannelPolicy => {
  const next: Record<string, unknown> = { ...fresh };
  const was = base as Record<string, unknown>;
  const want = desired as Record<string, unknown>;
  for (const key of new Set([...Object.keys(was), ...Object.keys(want)])) {
    if (same(was[key], want[key])) continue;
    if (want[key] === undefined) delete next[key];
    else next[key] = want[key];
  }
  return next as ChannelPolicy;
};

type Pending = {
  // The saved policy `desired` was built on: the diff between the two is the operator's edit.
  base: ChannelPolicy;
  // The whole policy the operator last asked for.
  desired: ChannelPolicy;
};

// useChannelPolicySave is the channel page's one policy write path (#1895). The policy PATCH is
// wholesale and carries the channel revision it was based on; the server 409s a stale revision.
// Firing one PATCH per edit therefore lost every edit made inside a round trip twice over: the
// second PATCH carried the pre-save revision (409), and it was built from the pre-save policy, so
// even an accepted one would have erased the first edit.
//
// So saves are SERIAL and COALESCED. One PATCH is in flight at a time. Edits made meanwhile
// replace `pending.desired` (the editors render it, so each edit stacks on the last) and go out
// together as the next PATCH, at the revision the previous response returned. A 409 from some
// other writer refetches the channel and rebases the edits onto it. Any other failure is an
// error toast, and the editors fall back to the saved policy, so nothing is dropped silently.
//
// The queue is per channel id. Surfing to another channel reuses the layout (and this hook), and
// an edit there must neither show nor ride on the previous channel's unsaved policy.
const useChannelPolicySave = (channelId: string, onSaved: () => void): ChannelPolicySave => {
  const queryClient = useQueryClient();
  // The drain loop runs across awaits, so it reads the queue from a ref; the state copy only
  // re-renders the editors.
  const queues = useRef(new Map<string, Pending>());
  const draining = useRef(new Set<string>());
  const [, setVersion] = useState(0);

  const setPending = (id: string, next: Pending | undefined) => {
    if (next) queues.current.set(id, next);
    else queues.current.delete(id);
    setVersion((v) => v + 1);
  };
  const saved = (id: string): ChannelDTO | undefined =>
    unwrap(queryClient.getQueryData<channelsApi.getChannelResponse>(channelsApi.getGetChannelQueryKey(id)));

  // Refetches the channel and rebases the pending edits onto it. False when it can't.
  const rebaseOntoServer = async (id: string): Promise<boolean> => {
    const queryKey = channelsApi.getGetChannelQueryKey(id);
    // A read already in flight may predate the write that caused the 409: start a fresh one.
    await queryClient.cancelQueries({ queryKey });
    const res: channelsApi.getChannelResponse = await queryClient.fetchQuery({
      queryKey,
      queryFn: () => channelsApi.getChannel(id),
      staleTime: 0,
    });
    const fresh = unwrap(res);
    const job = queues.current.get(id);
    if (!fresh || !job) return false;
    setPending(id, { base: fresh.policy, desired: rebase(fresh.policy, job.base, job.desired) });
    return true;
  };

  const drain = async (id: string) => {
    if (draining.current.has(id)) return;
    draining.current.add(id);
    const queryKey = channelsApi.getGetChannelQueryKey(id);
    let conflicts = 0;
    try {
      for (let job = queues.current.get(id); job; job = queues.current.get(id)) {
        const sending = job.desired;
        const revision = saved(id)?.revision;
        try {
          if (revision === undefined) throw new Error("The channel hasn't loaded yet.");
          const res = await channelsApi.updateChannel(id, { revision, policy: sending });
          // An older read still in flight must not land over the saved channel and its revision.
          await queryClient.cancelQueries({ queryKey });
          queryClient.setQueryData(queryKey, res);
          conflicts = 0;
          const next = queues.current.get(id);
          // Edits made during this round trip stay pending, now stacked on what was saved.
          setPending(
            id,
            next && next.desired !== sending
              ? { base: unwrap(res)?.policy ?? sending, desired: next.desired }
              : undefined,
          );
          if (!queues.current.has(id)) {
            onSaved();
            toast.success("Saved");
          }
        } catch (err) {
          const conflict = err instanceof ApiError && err.status === 409 && conflicts < MAX_CONFLICT_RETRIES;
          if (conflict) {
            conflicts += 1;
            if (await rebaseOntoServer(id).catch(() => false)) continue;
          }
          setPending(id, undefined);
          onSaved();
          toast.error(toProblem(err).title ?? "Couldn't save that change");
          return;
        }
      }
    } finally {
      draining.current.delete(id);
    }
  };

  const pending = queues.current.get(channelId);
  return {
    pending: pending?.desired,
    save: (next) => {
      const base = queues.current.get(channelId)?.base ?? saved(channelId)?.policy ?? next;
      setPending(channelId, { base, desired: next });
      void drain(channelId);
    },
    isSaving: pending !== undefined,
  };
};

export { useChannelPolicySave };
