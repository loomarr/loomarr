import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import { ApiError, toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";
import { toast } from "sonner";
import type { ChannelEdit, ChannelSave, ChannelSaveOptions } from "./use-channel-save.type";

// A 409 "Channel changed" means another writer (a reconcile, a sweep, a second admin) saved the
// channel first. Each one is rebased and retried; a run of them this long means the channel is
// changing under the editor faster than it can save, and the operator hears about it instead.
const MAX_CONFLICT_RETRIES = 3;

// The stale-revision problem's title (internal/api/channels.go staleChannelConflict, pinned by
// channels_test.go). A 409 with any other title, such as a renumber onto a taken number, is a
// real failure: refetching cannot fix it.
const STALE_TITLE = "Channel changed";
const isStale = (err: unknown): boolean =>
  err instanceof ApiError && err.status === 409 && err.problem.title === STALE_TITLE;

const same = (a: unknown, b: unknown): boolean => JSON.stringify(a) === JSON.stringify(b);

// For callers that fire and forget: the writer has already shown the failure as a toast.
const ignoreFailure = () => undefined;

// rebasePolicy re-applies the operator's policy edits onto a policy another writer changed. An
// edit is a top-level policy field whose value differs from the saved policy the edits were made
// against (`base`); only those fields take the operator's value, and every other field keeps the
// fresh one. A field the operator removed (an opt-out such as `autoCurate`) is removed again:
// PATCH replaces the policy wholesale, so absence is the clear.
const rebasePolicy = (fresh: ChannelPolicy, base: ChannelPolicy, desired: ChannelPolicy): ChannelPolicy => {
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

// The fields of `desired` that the PATCH which just landed did not already carry.
const unsent = (desired: ChannelEdit, sent: ChannelEdit): ChannelEdit =>
  Object.fromEntries(
    Object.entries(desired).filter(([key, value]) => !same(value, sent[key as keyof ChannelEdit])),
  ) as ChannelEdit;

type Waiter = { resolve: () => void; reject: (err: unknown) => void };

type Job = {
  // The saved policy `desired.policy` was built on: the diff between the two is the operator's
  // policy edit. Every other field in `desired` is a whole value the operator set.
  basePolicy: ChannelPolicy;
  // Every edit queued since the last save landed, merged; a later edit to a field replaces it.
  desired: ChannelEdit;
  // One per `save` call this job answers.
  waiters: Waiter[];
  success: string;
};

type Writer = {
  get: (id: string) => Job | undefined;
  subscribe: (listener: () => void) => () => void;
  save: (id: string, edit: ChannelEdit, options?: ChannelSaveOptions) => Promise<void>;
};

// createWriter is the one PATCH path for every channel a QueryClient knows (#1895, #1899). The
// PATCH carries the channel revision it was based on, and the server 409s a stale one, so two
// editors each sending their own PATCH from the page's read lost whichever landed second: a
// policy edit then a quick rename, or two lineup removes.
//
// So saves are SERIAL and COALESCED, per channel, whichever editor made them. One PATCH is in
// flight at a time. Edits made meanwhile merge into the pending job (the editors render it, so
// each edit stacks on the last) and go out together as the next PATCH, at the revision the
// previous response returned. A 409 "Channel changed" from some other writer refetches the
// channel and rebases the edits onto it. Any other failure is an error toast and the pending
// edits drop, so the editors fall back to what is saved and nothing is lost silently.
const createWriter = (queryClient: QueryClient): Writer => {
  const jobs = new Map<string, Job>();
  const draining = new Set<string>();
  const listeners = new Set<() => void>();

  const set = (id: string, job: Job | undefined) => {
    if (job) jobs.set(id, job);
    else jobs.delete(id);
    for (const listener of listeners) listener();
  };
  const cached = (id: string): ChannelDTO | undefined =>
    unwrap(queryClient.getQueryData<channelsApi.getChannelResponse>(channelsApi.getGetChannelQueryKey(id)));
  const read = async (id: string): Promise<ChannelDTO | undefined> => {
    const queryKey = channelsApi.getGetChannelQueryKey(id);
    // A read already in flight may predate the write that caused a 409: start a fresh one.
    await queryClient.cancelQueries({ queryKey });
    const res: channelsApi.getChannelResponse = await queryClient.fetchQuery({
      queryKey,
      queryFn: () => channelsApi.getChannel(id),
      staleTime: 0,
    });
    return unwrap(res);
  };

  // Everything a channel edit can change outside the channel resource itself: the guide and
  // now/next, the Overview's upcoming strip, the saved break and the saved cycle preview.
  const refresh = (id: string) => {
    for (const queryKey of [
      channelsApi.getGetChannelQueryKey(id),
      channelsApi.getChannelUpcomingQueryKey(id),
      channelsApi.getPreviewChannelPodsQueryKey(id),
      channelsApi.getPreviewChannelCycleQueryKey(id),
      channelsApi.getChannelsNowNextQueryKey(),
      channelsApi.getListChannelsQueryKey(),
      // The prefix: every cached guide window.
      channelsApi.getChannelGuideQueryKey(),
    ]) {
      void queryClient.invalidateQueries({ queryKey });
    }
  };

  // Refetches the channel and rebases the pending edits onto it. False when it can't.
  const rebaseOntoServer = async (id: string): Promise<boolean> => {
    const fresh = await read(id);
    const job = jobs.get(id);
    if (!fresh || !job) return false;
    const policy = job.desired.policy && rebasePolicy(fresh.policy, job.basePolicy, job.desired.policy);
    set(id, { ...job, basePolicy: fresh.policy, desired: policy ? { ...job.desired, policy } : job.desired });
    return true;
  };

  const drain = async (id: string) => {
    if (draining.has(id)) return;
    draining.add(id);
    const queryKey = channelsApi.getGetChannelQueryKey(id);
    let conflicts = 0;
    try {
      for (let job = jobs.get(id); job; job = jobs.get(id)) {
        const sent = job;
        try {
          // A page that never read this channel (the guide's row menu) reads it now.
          const base = cached(id) ?? (await read(id));
          if (!base) throw new Error("Couldn't read the latest channel");
          const res = await channelsApi.updateChannel(id, { ...sent.desired, revision: base.revision });
          // An older read still in flight must not land over the saved channel and its revision.
          await queryClient.cancelQueries({ queryKey });
          queryClient.setQueryData(queryKey, res);
          conflicts = 0;
          for (const waiter of sent.waiters) waiter.resolve();
          // Edits made during this round trip stay pending, now stacked on what was saved.
          const now = jobs.get(id) ?? sent;
          const desired = unsent(now.desired, sent.desired);
          const waiters = now.waiters.filter((waiter) => !sent.waiters.includes(waiter));
          if (Object.keys(desired).length > 0) {
            set(id, { ...now, basePolicy: unwrap(res)?.policy ?? base.policy, desired, waiters });
            continue;
          }
          // Whatever arrived meanwhile repeated what was just saved.
          for (const waiter of waiters) waiter.resolve();
          set(id, undefined);
          refresh(id);
          toast.success(now.success);
        } catch (err) {
          if (isStale(err) && conflicts < MAX_CONFLICT_RETRIES) {
            conflicts += 1;
            if (await rebaseOntoServer(id).catch(() => false)) continue;
          }
          const failed = jobs.get(id) ?? sent;
          set(id, undefined);
          refresh(id);
          toast.error(toProblem(err).title ?? "Couldn't save that change");
          for (const waiter of failed.waiters) waiter.reject(err);
          return;
        }
      }
    } finally {
      draining.delete(id);
    }
  };

  return {
    get: (id) => jobs.get(id),
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    save: (id, edit, options) =>
      new Promise<void>((resolve, reject) => {
        const job = jobs.get(id);
        set(id, {
          basePolicy: job?.basePolicy ?? cached(id)?.policy ?? {},
          desired: { ...job?.desired, ...edit },
          waiters: [...(job?.waiters ?? []), { resolve, reject }],
          success: options?.success ?? "Saved",
        });
        void drain(id);
      }),
  };
};

// One writer per QueryClient, so every component editing a channel shares its queue: the layout,
// the lineup editor, the rules and filler drafts and the guide's row menu. A test's fresh client
// gets a fresh writer.
const writers = new WeakMap<QueryClient, Writer>();
const writerFor = (queryClient: QueryClient): Writer => {
  let writer = writers.get(queryClient);
  if (!writer) {
    writer = createWriter(queryClient);
    writers.set(queryClient, writer);
  }
  return writer;
};

// useChannelSave is a channel's one write path. Every channel editor saves through it; none
// sends its own PATCH or holds a revision.
//
// The queue is per channel id. Surfing to another channel reuses the layout (and this hook), and
// an edit there must neither show nor ride on the previous channel's unsaved edits.
const useChannelSave = (channelId: string): ChannelSave => {
  const writer = writerFor(useQueryClient());
  const get = () => writer.get(channelId);
  const job = useSyncExternalStore(writer.subscribe, get, get);
  return {
    pending: job?.desired,
    save: (edit, options) => writer.save(channelId, edit, options),
    isSaving: job !== undefined,
  };
};

// withPending is the channel the page renders: the saved one under the pending policy and
// lineup, the two editors whose next edit is built from the current value.
const withPending = (ch: ChannelDTO, pending: ChannelEdit | undefined): ChannelDTO =>
  pending ? { ...ch, policy: pending.policy ?? ch.policy, lineup: pending.lineup ?? ch.lineup } : ch;

export { ignoreFailure, useChannelSave, withPending };
