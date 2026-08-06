import { fillerApi, toProblem, unwrap } from "@loomarr/api";
import { useState } from "react";
import { toast } from "sonner";
import { ErrorState, IncomingPanel } from "@/components/loomarr";
import { TunePanel } from "../tune-panel";
import { useFillerInvalidate } from "../use-filler-invalidate";
import type { IncomingTabProps } from "./incoming-tab.type";

// IncomingTab — the review queue (§10 V38): what has been downloaded but is not yet filed.
//
// ⚠ It owns its own query and its own mutations, and that is the point of it existing. All of
// this lived in `filler-page` beside the Catalog's and Sources' state, so a member sitting on
// the Catalog tab still mounted the Incoming queue, its four filing mutations and their busy
// state — paying for a surface they cannot even reach (the queue is admin-only server-side).
// Routing renders one tab at a time; the hooks now follow the routing.
//
// ⚠ ADMIN-ONLY, enforced server-side. The shell does not render this tab for a member, and
// nothing here re-checks that: a second, weaker copy of an authorization rule is worse than one
// rule in one place. The query gate below is about a member's console staying free of 403s, not
// about access.
const IncomingTab = ({ onEditTags }: IncomingTabProps) => {
  const { invalidateLifecycle } = useFillerInvalidate();

  const incomingQuery = fillerApi.useFillerIncoming();
  const asks = unwrap(incomingQuery.data, (b) => b.asks) ?? [];
  const reels = unwrap(incomingQuery.data, (b) => b.reels) ?? [];
  const recentlyFiled = unwrap(incomingQuery.data, (b) => b.recentlyFiled) ?? [];

  // Which clip a write is in flight for, so ONE row disables rather than the whole list. The
  // mutation's own isPending is global to the hook — using it alone greys out every button on
  // the page while a single confirm lands, which reads as the page having frozen.
  const [busyClip, setBusyClip] = useState<string>();
  const settle = () => setBusyClip(undefined);

  const fileClips = fillerApi.useFileFillerClips({
    mutation: { onSettled: settle, onSuccess: invalidateLifecycle },
  });
  const holdClips = fillerApi.useHoldFillerClips({
    mutation: {
      onSettled: settle,
      onSuccess: () => {
        toast.success("Sent back", { description: "It's out of rotation and back in the queue." });
        invalidateLifecycle();
      },
    },
  });
  const removeClips = fillerApi.useBulkRemoveFiller({
    mutation: {
      onSettled: settle,
      onSuccess: invalidateLifecycle,
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't remove those clips"),
    },
  });
  // Era-suggestion confirm (§10 V34). ⚠ The PATCH body must carry the clip's CURRENT
  // audience/category: UpdateClipTags writes all three columns unconditionally, so a bare
  // `{era}` would wipe the other two. Setting era confirms and clears the suggestion in the
  // same write (the BE's rule).
  const confirmEra = fillerApi.useTagFillerClip({
    mutation: {
      onSettled: settle,
      onSuccess: invalidateLifecycle,
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't confirm the era"),
    },
  });

  const busy = confirmEra.isPending || removeClips.isPending || fileClips.isPending || holdClips.isPending;

  return (
    <div id="panel-incoming" role="tabpanel" aria-labelledby="tab-incoming" className="flex flex-col gap-4">
      {incomingQuery.error != null && (
        <ErrorState error={incomingQuery.error} onRetry={() => incomingQuery.refetch()} />
      )}
      {/* ⚠ ABOVE the queue, matching the mock: the policy is the context the rows below are
          read in. Its counts come from this tab's query rather than a second one — the panel
          reports on the queue it sits over. */}
      <TunePanel filed={recentlyFiled.length} needsYou={asks.length} />
      <IncomingPanel
        asks={asks}
        reels={reels}
        recentlyFiled={recentlyFiled}
        // ⚠ Carries the clip's CURRENT audience and category, not just the era. The BE's
        // UpdateClipTags writes all three columns unconditionally, so a bare `{era}` would
        // silently wipe the other two — the hazard the comment above `confirmEra` records,
        // and one this call site reproduced on its first draft.
        onConfirmEra={(ask) => {
          setBusyClip(ask.path);
          confirmEra.mutate({
            id: ask.path,
            data: {
              era: ask.suggestedEra ?? 0,
              ...(ask.audience ? { audience: ask.audience as never } : {}),
              ...(ask.category ? { category: ask.category } : {}),
            },
          });
        }}
        onEditTags={(ask) => onEditTags(ask.path)}
        // "Don't use it" removes the clip from the CATALOG. The file stays where the operator
        // put it — the server's action is a tombstone, never a delete.
        onDismiss={(ask) => {
          setBusyClip(ask.path);
          removeClips.mutate({ data: { paths: [ask.path] } });
        }}
        // "Use it" files a clip as it stands — its tags are right enough. No era to confirm, so
        // this is the plain file, not the confirm-then-file above.
        onFile={(ask) => {
          setBusyClip(ask.path);
          fileClips.mutate({ data: { paths: [ask.path] } });
        }}
        // ⚠ `asSuggested` is what makes this per-CLIP: the server confirms each clip's own
        // proposed era. Sending one era for the whole selection is what the bulk tag bar does,
        // and it is the wrong answer for a queue of different guesses.
        onFileAllAsSuggested={() =>
          fileClips.mutate({ data: { paths: asks.map((a) => a.path), asSuggested: true } })
        }
        // The undo for auto-filing. ⚠ NOT a removal: the clip and its file both stay, it simply
        // stops being matched into pods until someone decides.
        onSendBack={(clip) => {
          setBusyClip(clip.path);
          holdClips.mutate({ data: { paths: [clip.path] } });
        }}
        {...(busy ? { busyPath: busyClip } : {})}
      />
      {/* ⚠ `IngestPanel` — the paste-a-URL box — is retired-ok here (V38b), not merely moved.
          Clips arrive because you added a SOURCE: Sources registers one, its per-row search
          queues a result, an approved pull fetches in bulk, and auto-fetch polls on a
          schedule. Every one of those paths is better than re-supplying a URL by hand, and
          a box that made you find the URL yourself was the odd one out.

          The ingest SSE frame it consumed is deliberately NOT deleted — auto-fetch and
          queued downloads still emit progress; nothing renders it yet. `events-provider`
          records why that fan-out is guarded structurally. */}
    </div>
  );
};

export { IncomingTab };
