import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import { toProblem } from "@loomarr/api/mutator";
import { Action, Field, Icon, icons, LoadingStrip, Stack, Surface, Text } from "@loomarr/design-system";
import { useEffect, useId, useState } from "react";
import { toast } from "sonner";
import { RefineReview } from "@/components/loomarr/ai/refine-review";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { GenerationProgress } from "@/components/loomarr/feedback/generation-progress";
import { useAppForm } from "@/lib/app-form";
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
  const headingId = useId();

  const refine = useChannelRefine();
  const elapsed = useElapsed(refine.isRunning);

  const form = useAppForm({
    defaultValues: { change: "" },
    onSubmit: ({ value }) => {
      const text = value.change.trim();
      if (!text) return;
      setState("running");
      refine.start(channelId, text);
    },
  });

  useEffect(() => {
    // A refused request (422 no stored intent, 409 already running) never starts a job, so
    // `refine.channelId` is never set and nothing below can fire: the request error alone
    // returns a submitted form to "open", where the error shows and Suggest changes retries.
    if (refine.error != null) {
      setState((s) => (s === "running" ? "open" : s));
      return;
    }
    if (refine.channelId !== channelId) return;
    if (refine.proposal) setState("landed");
    else if (refine.isRunning) setState("running");
    else if (refine.phase === "failed") setState("open");
  }, [channelId, refine.channelId, refine.error, refine.isRunning, refine.phase, refine.proposal]);

  const close = () => {
    setState("idle");
    form.reset();
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

  const landed = state === "landed" ? refine.proposal : undefined;
  // Show the generation-failed notice on the form whenever the last run ended in `failed`
  // and nothing was applied. (`refine.error` below covers the separate case where the
  // refine REQUEST itself failed before any job started.)
  const generationFailed = state === "open" && refine.phase === "failed" && !refine.proposal;
  const failureCopy = refine.failure ? suggestionFailureCopy(refine.failure) : undefined;

  return (
    // The web mock's card: always open, a one-line input and "Suggest changes". The running and
    // review states below are today's; the mock draws neither.
    <Stack
      aria-labelledby={headingId}
      className={cn("border-border bg-card rounded-lg border p-4", className)}
      gap="2.5"
      render="section"
    >
      <Stack align="center" direction="row" gap="inline" wrap>
        <Icon decorative glyph={icons.suggest} size="compact" tone="suggest" />
        <Text as="h3" id={headingId} textRole="cardLabel">
          Refine with AI
        </Text>
        <Text as="span" className="ml-auto" textRole="hint" tone="secondary">
          Describe what to change. You'll see the result before anything is saved.
        </Text>
      </Stack>

      {(state === "idle" || state === "open") && (
        <Stack gap="2.5">
          {generationFailed && (
            <Stack
              align="start"
              className="border-onair-tint-15 bg-onair-tint-10 text-onair-300 rounded-lg border px-3 py-2"
              direction="row"
              gap="inline"
              role="alert"
            >
              <Stack className="mt-0.5" direction="row" render="span">
                <Icon decorative glyph={icons.alert} size="compact" tone="inherit" />
              </Stack>
              {failureCopy ? (
                <Stack gap="0.5" render="span">
                  <Text as="span" textRole="cardLabel" tone="danger">
                    {failureCopy.title}
                  </Text>
                  <Text as="span" textRole="compact" tone="danger">
                    {failureCopy.message}
                  </Text>
                  <Text as="span" textRole="compact" tone="danger">
                    {failureCopy.guidance}
                  </Text>
                </Stack>
              ) : (
                <Text as="span" textRole="compact" tone="danger">
                  Loomarr couldn't update this channel. Try again or describe the change another way.
                </Text>
              )}
            </Stack>
          )}
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              void form.handleSubmit();
            }}
          >
            <form.Field name="change">
              {(field) => (
                <>
                  <Surface
                    backgroundColor="transparent"
                    borderWidth={0}
                    flexGrow={1}
                    flexShrink={1}
                    minWidth={0}
                  >
                    <Field
                      accessibilityLabel={`What to change on ${channelName}`}
                      onChangeText={field.handleChange}
                      placeholder="e.g. More 90s sci-fi, fewer movies after midnight"
                      value={field.state.value}
                    />
                  </Surface>
                  <Action disabled={field.state.value.trim().length === 0} submit>
                    Suggest changes
                  </Action>
                </>
              )}
            </form.Field>
          </form>
          {refine.error != null && <ErrorState error={refine.error} />}
        </Stack>
      )}

      {state === "running" && (
        <Stack gap="control">
          {refine.phase ? (
            <GenerationProgress
              phase={refine.phase}
              round={refine.round}
              elapsedSeconds={refine.progress ? undefined : elapsed}
              progress={refine.progress}
            />
          ) : (
            <Stack align="center" direction="row" gap="inline" render="p">
              <LoadingStrip decorative width={32} />
              <Text as="span" textRole="compact" tone="secondary">
                Starting…
              </Text>
            </Stack>
          )}
          {refine.error != null && <ErrorState error={refine.error} />}
        </Stack>
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
    </Stack>
  );
};

export { RefinePanel };
