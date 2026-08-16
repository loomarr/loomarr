import * as titlesApi from "@loomarr/api/endpoints/titles";
import type { TitleDTO } from "@loomarr/api/models/titleDTO";
import { TitleDTOState } from "@loomarr/api/models/titleDTOState";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { RotateCw } from "lucide-react";
import { StateBadge } from "@/components/loomarr/channels/state-badge";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { stageOf } from "@/queue/journey";
import { MyRequests } from "@/queue/my-requests";

// In flight — every title still being provisioned (or that has landed), grouped into the
// three journey stages. See `route.tsx` for why this fan-out is fetched here again rather
// than threaded down from the layout: same query keys, one shared cache entry.
const STATES = Object.values(TitleDTOState);

const STAGE_COPY = {
  waiting: { title: "Waiting", hint: "Approved and queued. Loomarr will request these next." },
  acquiring: { title: "On the way", hint: "Requested from your *arr apps and downloading." },
  ready: { title: "Ready", hint: "In your library and playable on a channel." },
} as const;

const ORDER = ["acquiring", "waiting", "ready"] as const;

const FlightScreen = () => {
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

  const failed = stateQueries.find((q) => q.error);
  if (failed) {
    return (
      <ErrorState
        error={failed.error}
        onRetry={() => {
          for (const q of stateQueries) q.refetch();
        }}
      />
    );
  }

  const isLoading = stateQueries.some((q) => q.isLoading);
  const rows: TitleDTO[] = stateQueries.flatMap((q) => unwrap(q.data, (b) => b.titles) ?? []);

  return (
    <>
      {/* The REQUESTS I submitted (V26), above the tracked titles and OUTSIDE the
          empty-titles branch on purpose — a request still waiting for approval has no
          titles yet, so folding it in would show "Nothing in flight" to exactly the
          member who most needs to see their request exists. Renders nothing when there
          are no requests, so the common path is unchanged. */}
      <MyRequests />

      {rows.length === 0 && !isLoading ? (
        <EmptyState
          title="Nothing in flight"
          description="Approved acquisitions show their journey here: from queued, to downloading, to playable."
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
    </>
  );
};

const Route = createFileRoute("/_authed/queue/flight")({
  component: FlightScreen,
});

export { Route };
