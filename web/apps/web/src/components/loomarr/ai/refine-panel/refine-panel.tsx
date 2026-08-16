import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import { toProblem } from "@loomarr/api/mutator";
import { ChevronDown, Loader2, Sparkles } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { RefineReview } from "@/components/loomarr/ai/refine-review";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { GenerationProgress } from "@/components/loomarr/feedback/generation-progress";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { ProposalJobFailure } from "@/suggest/proposal-job-failure";
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

  const refine = useChannelRefine();
  const elapsed = useElapsed(refine.isRunning);

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

  // The run landed a proposal — move from the progress stepper to the diff, once.
  if (state === "running" && refine.proposal) setState("landed");
  // The authoritative job read says the run failed with no proposal (distinct from a
  // mutation error). Drop back to the form so the
  // typed change is preserved and a retry is one click away — a recoverable stop, not a
  // dead end (the seamless principle: fix it in place).
  if (state === "running" && !refine.isRunning && !refine.proposal && refine.failure) setState("open");

  const landed = state === "landed" ? refine.proposal : undefined;
  // Show the generation-failed notice on the form whenever the last run ended in `failed`
  // and nothing was applied. (`refine.error` below covers the separate case where the
  // refine REQUEST itself failed before any job started.)
  const generationFailed = state === "open" ? refine.failure : undefined;

  return (
    <section className={cn("flex flex-col gap-3 rounded-lg border border-suggest-tint-15 p-4", className)}>
      <button
        type="button"
        onClick={() => (state === "idle" ? setState("open") : close())}
        aria-expanded={state !== "idle"}
        className="flex w-full cursor-pointer items-center gap-2 text-left"
      >
        <Sparkles className="size-4 text-suggest-300" aria-hidden />
        <span className="font-semibold">Refine with AI</span>
        <span className="text-muted-foreground text-sm">
          Describe a change and review a diff before it applies
        </span>
        <ChevronDown
          className={cn(
            "ml-auto size-4 text-muted-foreground transition-transform",
            state !== "idle" && "rotate-180",
          )}
          aria-hidden
        />
      </button>

      {state === "open" && (
        <div className="flex flex-col gap-3">
          {generationFailed && (
            <ProposalJobFailure
              failure={generationFailed}
              isAdmin
              onRetry={() => {
                setState("running");
                refine.retry();
              }}
            />
          )}
          <textarea
            value={change}
            onChange={(e) => setChange(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                e.preventDefault();
                submit();
              }
            }}
            placeholder={`What should change on ${channelName}? e.g. "add more Schwarzenegger, drop the slow ones"`}
            rows={3}
            aria-label="What to change"
            className="w-full resize-none rounded-lg border border-input bg-transparent px-4 py-3 text-sm leading-relaxed shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:border-suggest focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-suggest focus-visible:ring-offset-2 focus-visible:ring-offset-background"
          />
          {refine.error != null && <ErrorState error={refine.error} />}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={close}>
              Cancel
            </Button>
            <Button variant="suggest" onClick={submit} disabled={change.trim().length === 0}>
              <Sparkles aria-hidden />
              Refine
            </Button>
          </div>
        </div>
      )}

      {state === "running" && (
        <div className="flex flex-col gap-3" role="status" aria-live="polite" aria-busy="true">
          {refine.phase ? (
            <GenerationProgress phase={refine.phase} round={refine.round} elapsedSeconds={elapsed} />
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
