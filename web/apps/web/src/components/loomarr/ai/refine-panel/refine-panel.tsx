import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import { toProblem } from "@loomarr/api/mutator";
import { Loader2, Sparkles, TriangleAlert } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { toast } from "sonner";
import { RefineReview } from "@/components/loomarr/ai/refine-review";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { GenerationProgress } from "@/components/loomarr/feedback/generation-progress";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { suggestionFailureCopy } from "@/suggest/suggestion-failure-copy";
import { useChannelRefine } from "@/suggest/use-channel-refine";
import { useElapsed } from "@/suggest/use-elapsed";
import type { RefinePanelProps } from "./refine-panel.type";

type PanelState = "idle" | "open" | "running" | "landed";

// RefinePanel — the "Refine with AI" entry on a channel's detail page (§7 refine): an
// admin describes a change in plain words, the LLM re-proposes using the channel's
// current lineup as context (same suggester pipeline as a fresh intent, §8), and the
// result lands as a diff to review before anything is applied. Nothing here mutates
// the channel until Apply — the diff review IS the gate (§7), same as any other
// suggestion.
const RefinePanel = ({
  channelId,
  channelName,
  current,
  currentPolicy,
  onApplied,
  className,
}: RefinePanelProps) => {
  const [state, setState] = useState<PanelState>("idle");
  const [change, setChange] = useState("");
  const headingId = useId();

  const refine = useChannelRefine();
  const elapsed = useElapsed(refine.isRunning);

  useEffect(() => {
    if (refine.channelId !== channelId) return;
    if (refine.proposal) setState("landed");
    else if (refine.isRunning) setState("running");
    else if (refine.phase === "failed") setState("open");
  }, [channelId, refine.channelId, refine.isRunning, refine.phase, refine.proposal]);

  const close = () => {
    setState("idle");
    setChange("");
    refine.reset();
  };

  const approve = proposalsApi.useApproveProposal({
    mutation: {
      onSuccess: () => {
        toast.success("Applied");
        close();
        onApplied?.();
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't apply that change"),
    },
  });

  const submit = () => {
    const text = change.trim();
    if (!text) return;
    setState("running");
    refine.start(channelId, text);
  };

  const landed = state === "landed" ? refine.proposal : undefined;
  // Show the generation-failed notice on the form whenever the last run ended in `failed`
  // and nothing was applied. (`refine.error` below covers the separate case where the
  // refine REQUEST itself failed before any job started.)
  const generationFailed = state === "open" && refine.phase === "failed" && !refine.proposal;
  const failureCopy = refine.failure ? suggestionFailureCopy(refine.failure) : undefined;

  return (
    // The web mock's card: always open, a one-line input and "Suggest changes". The running and
    // review states below are today's; the mock draws neither.
    <section
      aria-labelledby={headingId}
      className={cn("flex flex-col gap-2.5 rounded-lg border border-border bg-card p-4", className)}
    >
      <div className="flex flex-wrap items-center gap-2">
        <Sparkles className="size-4 shrink-0 text-suggest-300" aria-hidden />
        <h3 id={headingId} className="font-medium text-sm">
          Refine with AI
        </h3>
        <span className="ml-auto text-muted-foreground text-xs">
          Describe what to change. You'll see the result before anything is saved.
        </span>
      </div>

      {(state === "idle" || state === "open") && (
        <div className="flex flex-col gap-2.5">
          {generationFailed && (
            <div
              role="alert"
              className="flex items-start gap-2 rounded-lg border border-onair-tint-15 bg-onair-tint-10 px-3 py-2 text-onair-300 text-sm"
            >
              <TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
              {failureCopy ? (
                <span className="flex flex-col gap-0.5">
                  <span className="font-medium">{failureCopy.title}</span>
                  <span>{failureCopy.message}</span>
                  <span>{failureCopy.guidance}</span>
                </span>
              ) : (
                <span>
                  Loomarr couldn't update this channel. Try again or describe the change another way.
                </span>
              )}
            </div>
          )}
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          >
            <Input
              value={change}
              onChange={(e) => setChange(e.target.value)}
              placeholder="e.g. More 90s sci-fi, fewer movies after midnight"
              aria-label={`What to change on ${channelName}`}
              className="flex-1"
            />
            <Button type="submit" disabled={change.trim().length === 0}>
              Suggest changes
            </Button>
          </form>
          {refine.error != null && <ErrorState error={refine.error} />}
        </div>
      )}

      {state === "running" && (
        <div className="flex flex-col gap-3">
          {refine.phase ? (
            <GenerationProgress
              phase={refine.phase}
              round={refine.round}
              elapsedSeconds={refine.progress ? undefined : elapsed}
              progress={refine.progress}
            />
          ) : (
            <p className="flex items-center gap-2 text-muted-foreground text-sm">
              <Loader2 className="size-4 animate-spin text-suggest-300" aria-hidden />
              Starting…
            </p>
          )}
          {refine.error != null && <ErrorState error={refine.error} />}
        </div>
      )}

      {landed && (
        <RefineReview
          proposed={landed.proposal.lineup ?? []}
          acquisitions={landed.proposal.acquisitions ?? []}
          current={current}
          currentPolicy={currentPolicy}
          proposedPolicy={landed.proposal.policy}
          busy={approve.isPending}
          onApply={() => approve.mutate({ id: landed.id, data: {} })}
          onDiscard={close}
        />
      )}
    </section>
  );
};

export { RefinePanel };
