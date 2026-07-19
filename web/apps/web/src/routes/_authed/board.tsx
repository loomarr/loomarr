import { titlesApi } from "@loomarr/api";
import { pluralize } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { journeyProgress, stageOf } from "@/board";
import { EmptyState, ErrorState, StateBadge } from "@/components/loomarr";
import { Button } from "@/components/ui";

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

const BoardScreen = () => {
  const queryClient = useQueryClient();
  const titles = titlesApi.useListTitles();
  const retry = titlesApi.useEnqueueTitle({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: titlesApi.getListTitlesQueryKey() }),
    },
  });

  if (titles.error) {
    return (
      <div className="p-6">
        <ErrorState error={titles.error} onRetry={() => titles.refetch()} />
      </div>
    );
  }

  const rows = titles.data?.status === 200 ? (titles.data.data.titles ?? []) : [];
  const progress = journeyProgress(rows);

  return (
    <div className="flex h-full flex-col">
      <header className="border-border border-b px-6 py-4">
        <h1 className="font-semibold text-xl">Board</h1>
        {rows.length > 0 && (
          <p className="mt-1 text-muted-foreground text-sm">
            {`${progress.ready} of ${pluralize(progress.total, "title")} ${progress.total === 1 ? "has" : "have"} landed.`}{" "}
            Channels play what's ready and fill the rest, so they improve as the others arrive.
          </p>
        )}
      </header>

      <div className="flex-1 overflow-auto p-6">
        {rows.length === 0 && !titles.isLoading ? (
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
