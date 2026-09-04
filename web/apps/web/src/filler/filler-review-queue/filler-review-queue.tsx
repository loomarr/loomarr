import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import type { FillerDecisionReviewDTO } from "@loomarr/api/models/fillerDecisionReviewDTO";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { formatClipDuration, formatRelative, pluralize } from "@loomarr/core/format";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ClipPlayer } from "@/components/loomarr/filler/clip-player";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScreeningSummary } from "./screening-summary";

const humanize = (value: string) => value.replaceAll("_", " ");

const ReviewCard = ({
  review,
  clip,
  clipLoading,
  clipUnavailable,
  onAbandon,
}: {
  review: FillerDecisionReviewDTO;
  clip?: ClipDTO;
  clipLoading: boolean;
  clipUnavailable: boolean;
  onAbandon: () => void;
}) => {
  const queryClient = useQueryClient();
  const [correcting, setCorrecting] = useState(false);
  const [answer, setAnswer] = useState("");
  const [verdict, setVerdict] = useState<"admit" | "reject">("admit");
  const [evidenceOpen, setEvidenceOpen] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [previewOpened, setPreviewOpened] = useState(false);
  const exactHash = /^[0-9a-f]{64}$/.test(review.clipHash);
  const screeningQuery = fillerApi.useGetFillerScreening(
    { hash: review.clipHash },
    { query: { enabled: exactHash && evidenceOpen } },
  );
  const screening = unwrap(screeningQuery.data, (body) => body);
  const canConfirm = Boolean(
    previewOpened &&
      clip &&
      screening?.clipHash === review.clipHash &&
      screening.state === "available" &&
      screening.outcome === "pass",
  );
  const action = fillerApi.useActOnFillerDecision({
    mutation: {
      onSuccess: (_result, variables) => {
        toast.success(variables.data.kind === "abandon" ? "Saved for later" : "Decision recorded");
        void queryClient.invalidateQueries({ queryKey: fillerApi.getFillerDecisionReviewsQueryKey() });
        void queryClient.invalidateQueries({ queryKey: fillerApi.getFillerDecisionOverviewQueryKey() });
        void queryClient.invalidateQueries({ queryKey: fillerApi.getFillerDecisionActivityQueryKey() });
      },
      onError: (error) => {
        const problem = toProblem(error);
        toast.error(problem.title ?? "The decision could not be recorded", {
          ...(problem.detail ? { description: problem.detail } : {}),
        });
      },
    },
  });

  const submit = (kind: "admit" | "reject" | "correct" | "abandon") => {
    action.mutate(
      {
        id: review.id,
        data: {
          actionId: crypto.randomUUID(),
          kind,
          ...(kind === "correct" ? { answer: answer.trim() } : {}),
          ...(kind === "correct" ? { correctedVerdict: verdict, reason: answer.trim() } : {}),
          ...(kind === "abandon" ? { reason: "skip for now" } : {}),
        },
      },
      { onSuccess: () => (kind === "abandon" ? onAbandon() : undefined) },
    );
  };

  return (
    <Card className="p-5" aria-labelledby={`review-${review.id}`}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <p className="text-muted-foreground text-xs">
            {clip
              ? `${clip.name} · ${formatClipDuration(clip.durationMs)}`
              : `Clip ${review.clipHash.slice(0, 10)}…`}
          </p>
          <h2 id={`review-${review.id}`} className="mt-1 font-semibold text-lg">
            {review.question}
          </h2>
        </div>
        <span className="text-muted-foreground text-xs">{formatRelative(review.createdAt)}</span>
      </div>

      {review.conflicts.length > 0 ? (
        <section className="mt-4 space-y-3" aria-label="Decisive conflicts">
          {review.conflicts.map((conflict) => (
            <div
              key={`${conflict.claim}-${conflict.values.join("-")}`}
              className="rounded-md border border-caution/35 bg-caution/5 p-3"
            >
              <p className="font-medium text-sm">Conflicting {humanize(conflict.claim)}</p>
              <p className="mt-1 text-muted-foreground text-sm">{conflict.values.join(" · ")}</p>
            </div>
          ))}
        </section>
      ) : null}

      <section className="mt-4 flex flex-wrap gap-2" aria-label="Why Loomarr asked">
        {review.reasonCodes.map((reason) => (
          <Badge key={reason} variant="neutral">
            {humanize(reason)}
          </Badge>
        ))}
        {review.evidenceRefs.length > 0 ? (
          <Badge variant="neutral">{pluralize(review.evidenceRefs.length, "evidence source")}</Badge>
        ) : null}
      </section>

      <div className="mt-4 rounded-md border border-border">
        <div className="flex flex-wrap items-center gap-3 p-3">
          <div className="min-w-0 flex-1">
            <p className="font-medium text-sm">Exact clip and screening</p>
            <p className="text-muted-foreground text-xs">
              Open the evidence and play this exact rendered clip before confirming it.
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            aria-expanded={evidenceOpen}
            aria-controls={`review-evidence-${review.id}`}
            onClick={() => setEvidenceOpen((open) => !open)}
          >
            {evidenceOpen ? "Hide evidence" : "Review evidence"}
          </Button>
        </div>
        {evidenceOpen ? (
          <div id={`review-evidence-${review.id}`} className="space-y-4 border-border border-t p-3">
            {clipUnavailable ? (
              <p className="text-onair-300 text-sm">The exact catalog clip could not be resolved.</p>
            ) : clip ? (
              <Button
                variant={previewOpened ? "outline" : "default"}
                size="sm"
                onClick={() => {
                  setPreviewOpened(true);
                  setPlaying(true);
                }}
              >
                {previewOpened ? "Play exact clip again" : "Play exact clip"}
              </Button>
            ) : clipLoading ? (
              <p aria-live="polite" className="text-muted-foreground text-sm">
                Resolving the exact catalog clip…
              </p>
            ) : (
              <p className="text-onair-300 text-sm">The exact catalog clip is no longer available.</p>
            )}

            {screeningQuery.error ? (
              <div className="rounded-md border border-caution/35 bg-caution/5 p-3 text-sm">
                Screening evidence could not be loaded. Positive confirmation remains unavailable.
              </div>
            ) : screening ? (
              <ScreeningSummary summary={screening} />
            ) : (
              <p aria-live="polite" className="text-muted-foreground text-sm">
                Verifying the current playback bytes and screening evidence…
              </p>
            )}
          </div>
        ) : null}
      </div>

      {correcting ? (
        <form
          className="mt-5 rounded-md border border-border bg-muted/20 p-4"
          onSubmit={(event) => {
            event.preventDefault();
            if (answer.trim() && (verdict === "reject" || canConfirm)) submit("correct");
          }}
        >
          <fieldset>
            <legend className="font-medium text-sm">What should Loomarr learn from this clip?</legend>
            <div className="mt-3 flex flex-wrap gap-4">
              <Label className="flex items-center gap-2">
                <input
                  type="radio"
                  name={`verdict-${review.id}`}
                  checked={verdict === "admit"}
                  onChange={() => setVerdict("admit")}
                />
                It is filler
              </Label>
              <Label className="flex items-center gap-2">
                <input
                  type="radio"
                  name={`verdict-${review.id}`}
                  checked={verdict === "reject"}
                  onChange={() => setVerdict("reject")}
                />
                It is not filler
              </Label>
            </div>
          </fieldset>
          <Label htmlFor={`correction-${review.id}`} className="mt-4 block">
            Correction
          </Label>
          <Input
            id={`correction-${review.id}`}
            className="mt-1"
            value={answer}
            onChange={(event) => setAnswer(event.target.value)}
            placeholder="For example: This is a soda commercial"
            autoFocus
            required
            maxLength={512}
          />
          <div className="mt-3 flex flex-wrap gap-2">
            <Button
              type="submit"
              disabled={action.isPending || !answer.trim() || (verdict === "admit" && !canConfirm)}
            >
              Save correction
            </Button>
            <Button
              type="button"
              variant="ghost"
              onClick={() => setCorrecting(false)}
              disabled={action.isPending}
            >
              Cancel
            </Button>
          </div>
        </form>
      ) : (
        <div className="mt-5 flex flex-wrap gap-2">
          <Button
            onClick={() => submit("admit")}
            disabled={action.isPending || !canConfirm}
            title={
              canConfirm
                ? "Confirm this screened exact clip for the library"
                : "Review the five screens and play the exact clip before confirming"
            }
          >
            Confirm for library
          </Button>
          <Button variant="outline" onClick={() => setCorrecting(true)} disabled={action.isPending}>
            Correct
          </Button>
          <Button variant="ghost" onClick={() => submit("reject")} disabled={action.isPending}>
            Reject
          </Button>
          <Button variant="ghost" onClick={() => submit("abandon")} disabled={action.isPending}>
            Skip for now
          </Button>
        </div>
      )}
      <ClipPlayer clip={playing && clip ? clip : null} onClose={() => setPlaying(false)} />
    </Card>
  );
};

const FillerReviewQueue = ({ hideEmpty = false }: { hideEmpty?: boolean }) => {
  const [abandoned, setAbandoned] = useState(() => new Set<string>());
  const query = fillerApi.useFillerDecisionReviews({ limit: 100 });
  const body = unwrap(query.data, (value) => value);
  const reviewHashes = body?.rows.map((review) => review.clipHash) ?? [];
  // Resolve the rendered identities in one bounded catalog read. A per-card query turned a
  // 100-row review page into 100 HTTP calls before the operator opened anything; only the much
  // heavier screening byte verification stays per-row and on-demand.
  const clipsQuery = fillerApi.useListFiller(
    { hashes: reviewHashes, includeHeld: true, includeComposites: true, limit: 100 },
    { query: { enabled: reviewHashes.length > 0 } },
  );
  const clips = unwrap(clipsQuery.data, (value) => value.clips) ?? [];
  const clipsByHash = new Map(clips.map((clip) => [clip.hash, clip]));

  if (query.error) return <ErrorState error={query.error} onRetry={() => query.refetch()} />;
  if (!body)
    return (
      <Card aria-live="polite" className="p-6">
        Checking for clips that need judgment…
      </Card>
    );
  const visibleRows = body.rows.filter((review) => !abandoned.has(review.id));
  if (visibleRows.length === 0) {
    if (abandoned.size === 0) {
      if (hideEmpty) return null;
      return (
        <EmptyState
          title="Nothing needs your attention"
          description="Loomarr is handling filler automatically. Automatic admits and rejects are recorded under Manage → Activity."
        />
      );
    }
    return (
      <EmptyState
        title="You're caught up for now"
        description="Skipped questions remain in Incoming for a later visit. Loomarr did not treat them as accepted or rejected."
      />
    );
  }

  return (
    <section aria-labelledby="review-queue-heading" className="flex flex-col gap-4">
      <div>
        <h1 id="review-queue-heading" className="font-semibold text-xl">
          A few clips need your judgment
        </h1>
        <p className="mt-1 text-muted-foreground text-sm">
          {pluralize(body.total, "plain question")}. Preparation and recoverable processing work continue
          below.
        </p>
      </div>
      {visibleRows.map((review) => (
        <ReviewCard
          key={review.id}
          review={review}
          clip={clipsByHash.get(review.clipHash)}
          clipLoading={clipsQuery.isLoading}
          clipUnavailable={Boolean(clipsQuery.error)}
          onAbandon={() => setAbandoned((current) => new Set(current).add(review.id))}
        />
      ))}
    </section>
  );
};

export { FillerReviewQueue };
