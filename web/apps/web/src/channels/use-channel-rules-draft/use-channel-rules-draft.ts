import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import type { ChannelRulesDraft } from "./use-channel-rules-draft.type";

// Matches the filler sandbox's debounce deliberately. Both drafts live on the channel page,
// and two different settle rhythms would make editing rules and editing filler feel like
// different applications.
const PREVIEW_DEBOUNCE_MS = 300;

// canonicalize — a stable string identity for a policy, used for BOTH the dirty check and the
// debounce key.
//
// ⚠ JSON.stringify of the object would be key-order dependent, so a policy that round-tripped
// through the server could read as "edited" purely because its keys came back in a different
// order. Sorting the keys makes the identity structural.
//
// ⚠ Rule ORDER is preserved, never sorted — list order IS priority (programming-design §6.6).
// Sorting rules would make a drag-to-reorder invisible to the dirty check, which is precisely
// the edit this draft exists to let you preview.
const canonicalize = (p: ChannelPolicy | undefined): string =>
  JSON.stringify(p ?? {}, (_k, v) =>
    v && typeof v === "object" && !Array.isArray(v)
      ? Object.fromEntries(Object.entries(v).sort(([a], [b]) => a.localeCompare(b)))
      : v,
  );

// useChannelRulesDraft — the scheduling-rules draft (design.md §12, programming-design §8.1).
//
// ⚠ **Why the rules draft rather than saving inline like the rest of the page.** Rules resolve
// FIRST-MATCH-BY-PRIORITY, so a rule's effect depends on every rule above it. Authoring one is
// inherently multi-step — add it, set its tokens, drag it into position — and each intermediate
// state is a DIFFERENT schedule. Saving each step would reconcile half-finished rule sets to
// Tunarr, so a viewer could tune in to a marathon that existed for four seconds because the
// operator had not yet dragged it below the kids block.
//
// Everything else on Programming stays seamless: the lineup, era/ceiling, ordering and
// auto-curate are each self-contained, and making them require an Apply click would restore
// the ceremony §12 removed.
//
// Shaped after useChannelFillerDraft on purpose — same interface, same debounce, same
// render-time resync — so the page's two drafts behave identically rather than each having
// its own personality.
const useChannelRulesDraft = (channelId: string, policy: ChannelPolicy | undefined): ChannelRulesDraft => {
  const queryClient = useQueryClient();

  // The saved policy this draft was seeded from, as a canonical string — its identity, not its
  // reference (the page hands a fresh `ch.policy` object every render). Resync only when THIS
  // changes: a genuine server update adopts, an incidental re-render does not.
  const savedKey = canonicalize(policy);
  const [draft, setDraft] = useState<ChannelPolicy>(policy ?? {});
  const [adoptedKey, setAdoptedKey] = useState(savedKey);
  if (savedKey !== adoptedKey) {
    setDraft(policy ?? {});
    setAdoptedKey(savedKey);
  }

  // The wall-clock to evaluate at ("" = now). A preview INPUT, so it shares the debounce.
  const [at, setAt] = useState("");

  const preview = channelsApi.usePreviewChannelProgramming();
  const draftKey = canonicalize(draft);

  // biome-ignore lint/correctness/useExhaustiveDependencies: draftKey + at ARE the dependencies
  useEffect(() => {
    const t = setTimeout(() => {
      preview.mutate({
        id: channelId,
        params: at ? { at } : undefined,
        data: { policy: JSON.parse(draftKey) as ChannelPolicy },
      });
    }, PREVIEW_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [channelId, draftKey, at]);

  const update = channelsApi.useUpdateChannel({
    mutation: {
      onSuccess: () => {
        void queryClient.invalidateQueries({
          queryKey: channelsApi.getGetChannelQueryKey(channelId),
        });
        // ⚠ The SAVED cycle preview is its own query key, not under the channel — invalidate it
        // explicitly or the verification pane would keep showing the pre-apply schedule.
        void queryClient.invalidateQueries({
          queryKey: channelsApi.getPreviewChannelCycleQueryKey(channelId),
        });
        toast.success("Programming rules updated");
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't update programming rules"),
    },
  });

  return {
    draft,
    setDraft,
    // ⚠ The transport mutator THROWS on non-2xx for mutations (unlike a query, which carries
    // the status in `data`), so a failed preview lands in `.error` — surfaced so a broken draft
    // reads as an error rather than an empty schedule.
    preview: unwrap(preview.data),
    isPreviewing: preview.isPending,
    previewError: preview.error,
    at,
    setAt,
    isDirty: draftKey !== savedKey,
    // PATCH replaces `policy` whole (only reconcile-owned `applied` and — since §10 V51f —
    // an OMITTED `filler` are preserved server-side), so the draft IS the policy: no merge
    // needed, unlike the filler draft which owns one key.
    //
    // ⚠ **This used to be safe only by accident.** The draft is seeded from `policy ?? {}`, so
    // before the page's channel resolves it is the EMPTY object — and applying then sent a policy
    // with no `filler`, wiping the channel's pins, exclusions and criteria with no way to get them
    // back (nothing re-proposes an operator's filler selection the way a refine re-suggests a
    // scope). The server now treats an absent `filler` as "unchanged", so the loss is unreachable
    // from here regardless of what this hook sends or when.
    apply: () => update.mutate({ id: channelId, data: { policy: draft } }),
    isApplying: update.isPending,
    discard: () => setDraft(policy ?? {}),
  };
};

export { canonicalize, PREVIEW_DEBOUNCE_MS, useChannelRulesDraft };
