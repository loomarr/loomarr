import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerIncomingOutputBody } from "@loomarr/api/models/fillerIncomingOutputBody";
import type { IncomingClipGroupDTO } from "@loomarr/api/models/incomingClipGroupDTO";
import type { IncomingHelpGroupDTO } from "@loomarr/api/models/incomingHelpGroupDTO";
import type { IncomingStatusDTO } from "@loomarr/api/models/incomingStatusDTO";
import { isOk, unwrap } from "@loomarr/api/unwrap";
import { formatClipDuration, formatRelative, pluralize } from "@loomarr/core/format";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Image } from "@/components/ui/image";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useLoomarrEventListener } from "@/events/events-provider";
import { ClipDetails } from "../clip-details";

type IncomingView = Pick<
  FillerIncomingOutputBody,
  "preparing" | "needsHelp" | "recentlyReady" | "readyWindowSeconds"
>;
type GroupName = "preparing" | "needsHelp" | "recentlyReady";

const readyWindowLabel = (seconds: number): string => {
  if (seconds === 3600) return "the last hour";
  if (seconds === 86400) return "the last 24 hours";
  if (seconds % 86400 === 0) return `the last ${seconds / 86400} days`;
  if (seconds % 3600 === 0) return `the last ${seconds / 3600} hours`;
  return `the last ${Math.max(1, Math.round(seconds / 3600))} hours`;
};

const mergeRows = <T extends { clipHash: string }>(current: T[], next: T[]) => {
  const seen = new Set(current.map((row) => row.clipHash));
  return [...current, ...next.filter((row) => !seen.has(row.clipHash))];
};

const StatusRows = ({
  group,
  loading,
  moreLabel,
  onOpen,
  onMore,
}: {
  group: IncomingClipGroupDTO;
  loading: boolean;
  moreLabel: string;
  onOpen: (row: IncomingStatusDTO, trigger: HTMLElement) => void;
  onMore: () => void;
}) => (
  <div className="divide-y divide-border rounded-lg border border-border">
    {group.rows.map((row) => (
      <button
        key={row.clipHash}
        type="button"
        className="flex min-h-16 w-full min-w-0 items-center gap-3 px-3 py-2 text-left transition-colors hover:bg-muted/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
        aria-label={`View details for ${row.name}`}
        onClick={(event) => onOpen(row, event.currentTarget)}
      >
        <span className="h-12 w-16 shrink-0 overflow-hidden rounded bg-muted">
          {row.thumbImage ? (
            <Image image={row.thumbImage} alt="" sizes="64px" className="size-full object-cover" />
          ) : null}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block break-words font-medium text-sm">{row.name}</span>
          <span className="mt-0.5 block text-muted-foreground text-xs">
            {row.from ? `${row.from} · ` : ""}
            {formatClipDuration(row.durationMs)}
          </span>
        </span>
        <span className="shrink-0 text-muted-foreground text-xs">{row.statusLabel}</span>
      </button>
    ))}
    {group.nextCursor ? (
      <div className="p-2">
        <Button className="w-full" variant="ghost" size="sm" disabled={loading} onClick={onMore}>
          {loading ? "Loading…" : moreLabel}
        </Button>
      </div>
    ) : null}
  </div>
);

const Incoming = () => {
  const queryClient = useQueryClient();
  const query = fillerApi.useFillerIncoming();
  const body = unwrap(query.data, (value) => value);
  const [view, setView] = useState<IncomingView>();
  const [loadingGroup, setLoadingGroup] = useState<GroupName>();
  const [selected, setSelected] = useState<IncomingStatusDTO>();
  const [helpIndex, setHelpIndex] = useState(0);
  const detailTrigger = useRef<HTMLElement>(null);

  // Events are a wake-up hint only. Refetching the bounded server projection keeps reconnects,
  // missed frames, terminal moves, totals, and group membership on one source of truth.
  useLoomarrEventListener({
    onFillerClip: () => {
      void queryClient.invalidateQueries({ queryKey: fillerApi.getFillerIncomingQueryKey() });
    },
  });

  useEffect(() => {
    if (body) {
      setView({
        preparing: body.preparing,
        needsHelp: body.needsHelp,
        recentlyReady: body.recentlyReady,
        readyWindowSeconds: body.readyWindowSeconds,
      });
      setHelpIndex(0);
    }
  }, [body]);

  const selectedClipQuery = fillerApi.useListFiller(
    {
      hashes: selected ? [selected.clipHash] : [],
      includeHeld: true,
      includeComposites: true,
      limit: 1,
    },
    { query: { enabled: Boolean(selected) } },
  );
  const selectedClip = unwrap(selectedClipQuery.data, (value) => value.clips[0]);
  // The ordered history can contain identical records; give each occurrence its own identity.
  const stageOccurrences = new Map<string, number>();

  const loadMore = async (name: GroupName) => {
    if (!view) return false;
    const cursor = view[name].nextCursor;
    if (!cursor) return false;
    setLoadingGroup(name);
    try {
      const params =
        name === "preparing"
          ? { preparingCursor: cursor }
          : name === "needsHelp"
            ? { needsHelpCursor: cursor }
            : { readyCursor: cursor };
      const response = await fillerApi.fillerIncoming(params);
      if (!isOk(response)) throw new Error("The next page could not be loaded");
      const next = response.data[name];
      setView((current) => {
        if (!current) return current;
        if (name === "needsHelp") {
          return {
            ...current,
            needsHelp: {
              ...next,
              rows: mergeRows(current.needsHelp.rows, (next as IncomingHelpGroupDTO).rows),
            } as IncomingHelpGroupDTO,
          };
        }
        const clipGroup = next as IncomingClipGroupDTO;
        return {
          ...current,
          [name]: { ...clipGroup, rows: mergeRows(current[name].rows, clipGroup.rows) },
        };
      });
      return true;
    } catch {
      toast.error("Couldn't load more clips");
      return false;
    } finally {
      setLoadingGroup(undefined);
    }
  };

  const showNextHelp = async () => {
    if (!view) return;
    if (helpIndex + 1 < view.needsHelp.rows.length) {
      setHelpIndex(helpIndex + 1);
      return;
    }
    if (view.needsHelp.nextCursor) {
      const nextIndex = view.needsHelp.rows.length;
      if (await loadMore("needsHelp")) setHelpIndex(nextIndex);
    }
  };

  if (query.error) return <ErrorState error={query.error} onRetry={() => query.refetch()} />;
  if (!view) {
    return (
      <Card aria-live="polite" className="p-6 text-muted-foreground text-sm">
        Checking what’s on the way…
      </Card>
    );
  }

  const total = view.preparing.total + view.needsHelp.total + view.recentlyReady.total;
  if (total === 0) {
    return (
      <section aria-labelledby="incoming-empty-heading">
        <h2 id="incoming-empty-heading" className="font-semibold text-2xl">
          Nothing on the way
        </h2>
        <div className="flex flex-col items-center">
          <EmptyState
            title="Incoming is clear"
            description="New clips will appear here after you add a source."
          />
          <Button className="-mt-7" render={<Link to="/filler/sources" />} size="sm">
            Add a source
          </Button>
        </div>
      </section>
    );
  }

  const summary =
    view.preparing.total > 0
      ? `${pluralize(view.preparing.total, "clip")} ${view.preparing.total === 1 ? "is" : "are"} getting ready`
      : view.needsHelp.total > 0
        ? `${pluralize(view.needsHelp.total, "clip")} ${view.needsHelp.total === 1 ? "needs" : "need"} your help`
        : `${pluralize(view.recentlyReady.total, "clip")} added to your Library`;
  const summaryDescription =
    view.preparing.total > 0
      ? "Loomarr is taking care of these in the background."
      : view.needsHelp.total > 0
        ? "Loomarr needs one choice from you before it can continue."
        : view.recentlyReady.total === 1
          ? "This clip is ready whenever a channel needs it."
          : "These clips are ready whenever a channel needs them.";

  const openDetails = (row: IncomingStatusDTO, trigger: HTMLElement) => {
    detailTrigger.current = trigger;
    setSelected(row);
  };
  const helpTask = view.needsHelp.rows[helpIndex];

  return (
    <section aria-labelledby="incoming-heading" className="flex flex-col gap-7">
      <div>
        <h2 id="incoming-heading" className="font-semibold text-2xl">
          {summary}
        </h2>
        <p className="mt-1 text-muted-foreground text-sm">{summaryDescription}</p>
      </div>

      {view.preparing.total > 0 ? (
        <section aria-labelledby="preparing-heading" className="space-y-3">
          <div>
            <h3 id="preparing-heading" className="font-semibold text-lg">
              Getting clips ready
            </h3>
            <p className="text-muted-foreground text-sm">
              You can leave this page. Work continues on its own.
            </p>
          </div>
          <StatusRows
            group={view.preparing}
            loading={loadingGroup === "preparing"}
            moreLabel="Show more clips being prepared"
            onOpen={openDetails}
            onMore={() => void loadMore("preparing")}
          />
        </section>
      ) : null}

      {view.needsHelp.total > 0 ? (
        <section aria-labelledby="needs-help-heading" className="space-y-3">
          <div>
            <h3 id="needs-help-heading" className="font-semibold text-lg">
              Needs your help
            </h3>
            <p className="text-muted-foreground text-sm">
              Only choices Loomarr can’t safely make for you appear here.
            </p>
          </div>
          {helpTask ? (
            <Card className="space-y-4 p-4">
              <div className="flex items-center justify-between gap-3 text-muted-foreground text-xs">
                <span>
                  {helpIndex + 1} of {view.needsHelp.total}
                </span>
                {view.needsHelp.total > 1 ? <span>One choice at a time</span> : null}
              </div>
              <div className="flex flex-col items-start gap-4 sm:flex-row sm:items-center">
                <div className="min-w-0 flex-1">
                  <p className="break-words font-medium text-sm">{helpTask.name}</p>
                  <p className="mt-1 text-muted-foreground text-sm">{helpTask.question}</p>
                </div>
                <Button render={<a href={helpTask.actionHref} />} size="sm">
                  {helpTask.actionLabel}
                </Button>
              </div>
              {view.needsHelp.total > 1 ? (
                <div className="flex justify-end gap-2 border-border border-t pt-3">
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={helpIndex === 0}
                    onClick={() => setHelpIndex((current) => Math.max(0, current - 1))}
                  >
                    Previous
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={loadingGroup === "needsHelp" || helpIndex + 1 >= view.needsHelp.total}
                    onClick={() => void showNextHelp()}
                  >
                    {loadingGroup === "needsHelp" ? "Loading…" : "Next"}
                  </Button>
                </div>
              ) : null}
            </Card>
          ) : null}
        </section>
      ) : null}

      {view.recentlyReady.total > 0 ? (
        <section aria-labelledby="ready-heading" className="space-y-3">
          <div className="flex flex-wrap items-end justify-between gap-2">
            <div>
              <h3 id="ready-heading" className="font-semibold text-lg">
                Ready
              </h3>
              <p className="text-muted-foreground text-sm">
                Added in {readyWindowLabel(view.readyWindowSeconds)}. These clips stay in your Library.{" "}
                <Link
                  to="/filler/settings"
                  search={{ section: "incoming" }}
                  className="underline underline-offset-4"
                >
                  Change
                </Link>
              </p>
            </div>
            <Button render={<Link to="/filler/library" />} variant="outline" size="sm">
              Open Library
            </Button>
          </div>
          <StatusRows
            group={view.recentlyReady}
            loading={loadingGroup === "recentlyReady"}
            moreLabel="Show more ready clips"
            onOpen={openDetails}
            onMore={() => void loadMore("recentlyReady")}
          />
        </section>
      ) : null}

      <Sheet
        open={Boolean(selected)}
        onOpenChange={(open) => !open && setSelected(undefined)}
        swipeDirection="right"
      >
        {selected ? (
          <SheetContent finalFocus={detailTrigger}>
            <SheetHeader>
              <SheetTitle>{selected.name}</SheetTitle>
              <SheetDescription>{selected.statusLabel}</SheetDescription>
            </SheetHeader>
            <div className="space-y-5 p-6">
              {selectedClip ? (
                <ClipDetails clip={selectedClip} />
              ) : (
                <p role="status" className="text-muted-foreground text-sm">
                  {selectedClipQuery.isLoading
                    ? "Loading preview…"
                    : selectedClipQuery.error
                      ? "Preview could not be loaded."
                      : "Preview is not available yet."}
                </p>
              )}
              <p className="text-muted-foreground text-xs">Updated {formatRelative(selected.updatedAt)}</p>
              <details className="rounded-lg border border-border p-4 text-sm">
                <summary className="cursor-pointer font-medium">Technical details</summary>
                <div className="mt-4 space-y-4">
                  {selected.technical.attempts ? (
                    <p className="text-muted-foreground">
                      {pluralize(selected.technical.attempts, "processing attempt")}
                    </p>
                  ) : null}
                  {selected.technical.nextTryAt ? (
                    <p className="text-muted-foreground">
                      Next try {formatRelative(selected.technical.nextTryAt)}
                    </p>
                  ) : null}
                  {selected.technical.stages.length ? (
                    <ol className="space-y-3">
                      {selected.technical.stages.map((stage) => {
                        const identity = JSON.stringify([
                          selected.clipHash,
                          stage.at,
                          stage.label,
                          stage.status,
                          stage.note,
                        ]);
                        const occurrence = stageOccurrences.get(identity) ?? 0;
                        stageOccurrences.set(identity, occurrence + 1);
                        return (
                          <li
                            key={JSON.stringify([identity, occurrence])}
                            className="grid grid-cols-[1fr_auto] gap-x-3"
                          >
                            <span>{stage.label}</span>
                            <span className="text-muted-foreground">{stage.status}</span>
                            {stage.note ? (
                              <span className="col-span-2 mt-0.5 break-words text-muted-foreground text-xs">
                                {stage.note}
                              </span>
                            ) : null}
                          </li>
                        );
                      })}
                    </ol>
                  ) : (
                    <p className="text-muted-foreground">No processing steps recorded yet.</p>
                  )}
                </div>
              </details>
            </div>
          </SheetContent>
        ) : null}
      </Sheet>
    </section>
  );
};

export { Incoming };
