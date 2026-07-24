import { type TitleDTO, TitleDTOState, titlesApi } from "@loomarr/api";
import { pluralize } from "@loomarr/core";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { RotateCw } from "lucide-react";
import { journeyProgress, stageOf } from "@/board";
import { EmptyState, ErrorState, StateBadge } from "@/components/loomarr";
import { Button } from "@/components/ui";
import { useDocumentTitle } from "@/lib";

// Board / My proposals (§12, §13) — where a member watches their submission land.
// It deliberately leads with the JOURNEY ("4 of 7 have landed") rather than a table of
// provisioning states, because the member framing §13 asks for is "where is my stuff",
// not "what does `requested` mean". The per-title state badge is still there for the
// operator who wants the detail. Live via `title` SSE.
const STAGE_COPY = {
  waiting: { title: "Waiting", hint: "Approved and queued — Loomarr will request these next." },
  acquiring: { title: "On the way", hint: "Requested from your *arr apps and downloading." },
  ready: { title: "Ready", hint: "In your library and playable on a channel." },
} as const;

const ORDER = ["acquiring", "waiting", "ready"] as const;

// The Board needs every provisioning state at once (§4) to collapse them into the three
// journey stages. GET /v1/titles is a single-state FILTER, though — it 400s without a
// `state` (a deliberate, §7-pinned contract: `TestListRequiresState`, so an unbounded
// "list all titles" can't be issued by accident) — so we fan out one query per state and
// merge. This is the aggregation the endpoint intentionally leaves to the client.
const STATES = Object.values(TitleDTOState);

const BoardScreen = () => {
  useDocumentTitle("Board");
  const queryClient = useQueryClient();
  const stateQueries = useQueries({
    queries: STATES.map((state) => titlesApi.getListTitlesQueryOptions({ state })),
  });
  const retry = titlesApi.useEnqueueTitle({
    mutation: {
      // A re-enqueue moves the title out of `unavailable` and back to `wanted`, and the
      // reconciler may advance it further before we refetch — so invalidate every state
      // query, not just the two ends of the transition. Cheap at household scale.
      onSuccess: () =>
        Promise.all(
          STATES.map((state) =>
            queryClient.invalidateQueries({ queryKey: titlesApi.getListTitlesQueryKey({ state }) }),
          ),
        ),
    },
  });

  // All-or-nothing: if ANY state query fails, show the error rather than a board that
  // silently omits a stage — a partial set would make journeyProgress misreport how far
  // along the member actually is.
  const failed = stateQueries.find((q) => q.error);
  if (failed) {
    return (
      <div className="p-6">
        <ErrorState
          error={failed.error}
          onRetry={() => {
            for (const q of stateQueries) q.refetch();
          }}
        />
      </div>
    );
  }

  const isLoading = stateQueries.some((q) => q.isLoading);
  const rows: TitleDTO[] = stateQueries.flatMap((q) =>
    q.data?.status === 200 ? (q.data.data.titles ?? []) : [],
  );
  const progress = journeyProgress(rows);

  return (
    <div className="flex h-full flex-col">
      <header className="border-border border-b px-6 py-4">
        <h1 className="font-semibold text-xl">Board</h1>
        {rows.length > 0 && (
          <p className="mt-1 text-muted-foreground text-sm">
            {`${progress.ready} of ${pluralize(progress.total, "title")} ${progress.total === 1 ? "has" : "have"} landed.`}{" "}
            Channels play what's ready and fill the rest, improving as more arrives.
          </p>
        )}
      </header>

      <div className="flex-1 overflow-auto p-6">
        {rows.length === 0 && !isLoading ? (
          <EmptyState
            title="Nothing in flight"
            description="Approved acquisitions show their journey here — from queued, to downloading, to playable."
          />
        ) : (
          <div className="flex flex-col gap-6">
            {ORDER.map((stage) => {
              const inStage = rows.filter((t) => stageOf(t) === stage);
              if (inStage.length === 0) return null;
              return (
                <section key={stage} className="flex flex-col gap-2">
                  <h2 className="font-semibold text-lg">
                    {STAGE_COPY[stage].title}{" "}
                    <span className="font-normal text-muted-foreground text-sm">({inStage.length})</span>
                  </h2>
                  <p className="text-muted-foreground text-sm">{STAGE_COPY[stage].hint}</p>
                  <ul className="flex flex-col gap-2">
                    {inStage.map((t) => (
                      <li
                        key={t.key}
                        className="flex items-center gap-3 rounded-md border border-border bg-card px-3 py-2.5"
                      >
                        <span className="min-w-0 flex-1 truncate text-sm">
                          {t.name ?? t.key}
                          {t.year ? <span className="text-muted-foreground"> ({t.year})</span> : null}
                        </span>
                        <StateBadge state={t.state} />
                        {/* A given-up title is the one thing a human can act on here:
                            re-enqueue puts it back in the queue (§4). */}
                        {t.state === "unavailable" && (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={retry.isPending}
                            onClick={() =>
                              // Re-enqueue by identity, not by key: the enqueue contract
                              // takes the title's real ids so the reconciler can request
                              // it again from scratch.
                              retry.mutate({
                                data: {
                                  mediaType: t.mediaType,
                                  tmdbId: t.tmdbId,
                                  tvdbId: t.tvdbId,
                                  name: t.name,
                                  year: t.year,
                                },
                              })
                            }
                          >
                            <RotateCw aria-hidden />
                            Try again
                          </Button>
                        )}
                      </li>
                    ))}
                  </ul>
                </section>
              );
            })}
          </div>
        )}
        {retry.error != null && <ErrorState error={retry.error} className="mt-4" />}
      </div>
    </div>
  );
};

const Route = createFileRoute("/_authed/board")({
  component: BoardScreen,
});

export { Route };
