import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerIncomingOutputBody } from "@loomarr/api/models/fillerIncomingOutputBody";
import type { IncomingClipGroupDTO } from "@loomarr/api/models/incomingClipGroupDTO";
import type { IncomingHelpGroupDTO } from "@loomarr/api/models/incomingHelpGroupDTO";
import type { IncomingStatusDTO } from "@loomarr/api/models/incomingStatusDTO";
import { isOk, unwrap } from "@loomarr/api/unwrap";
import { formatClipDuration, formatRelative, pluralize } from "@loomarr/core/format";
import {
  Action,
  Card,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  PressableRow,
  Stack,
  Text,
} from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { useHouseholdTimeZone } from "@/channels/use-household-time-zone";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { ServedImage } from "@/components/loomarr/served-image";
import { useLoomarrEventListener } from "@/events/events-provider";
import { ClipDetails } from "../clip-details";
import { IncomingPreparationSummary } from "./incoming-preparation-summary";
import { IncomingProcessingDetails } from "./incoming-processing-details";

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
  <Card className="divide-border divide-y">
    {group.rows.map((row) => (
      <PressableRow
        key={row.clipHash}
        onPress={(event) => onOpen(row, event.currentTarget as unknown as HTMLElement)}
      >
        {/* The name comes from the content, so it holds every visible word (WCAG 2.5.3). */}
        <span className="sr-only">View details for</span>{" "}
        <Stack
          backgroundColor="surfaceElevated"
          borderRadius={4}
          flexShrink={0}
          height={48}
          overflow="hidden"
          render="span"
          width={64}
        >
          {row.thumbImage ? <ServedImage image={row.thumbImage} alt="" sizes="64px" /> : null}
        </Stack>
        <Stack flexBasis="0%" flexGrow={1} gap="0.5" minWidth={0} render="span">
          <Text as="span" className="break-words" textRole="cardLabel" tone="primary">
            {row.name}
          </Text>
          <Text as="span" textRole="hint" tone="secondary">
            {row.from ? `${row.from} · ` : ""}
            {formatClipDuration(row.durationMs)}
          </Text>
        </Stack>
        <Text flexShrink={0} textRole="hint" tone="secondary">
          {row.statusLabel}
        </Text>
      </PressableRow>
    ))}
    {group.nextCursor ? (
      <Stack padding={8}>
        <Action disabled={loading} onPress={onMore} style={{ width: "100%" }} variant="ghost">
          {loading ? "Loading…" : moreLabel}
        </Action>
      </Stack>
    ) : null}
  </Card>
);

const Incoming = () => {
  const queryClient = useQueryClient();
  const query = fillerApi.useFillerIncoming();
  const timeZone = useHouseholdTimeZone();
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
      <Card aria-live="polite" padding={24}>
        <Text as="p" textRole="compact" tone="secondary">
          Checking what’s on the way…
        </Text>
      </Card>
    );
  }

  const total = view.preparing.total + view.needsHelp.total + view.recentlyReady.total;
  if (total === 0) {
    return (
      <section aria-labelledby="incoming-empty-heading">
        <Text as="h2" id="incoming-empty-heading" textRole="display">
          Nothing on the way
        </Text>
        <Stack align="center">
          <EmptyState
            title="Incoming is clear"
            description="New clips will appear here after you add a source."
          />
          <Stack marginTop={-28}>
            <Action accessibilityRole="link" render={<Link to="/filler/sources" />}>
              Add a source
            </Action>
          </Stack>
        </Stack>
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
    <Stack aria-labelledby="incoming-heading" gap="7" render="section">
      <Stack gap="1">
        <Text as="h2" id="incoming-heading" textRole="display">
          {summary}
        </Text>
        <Text as="p" textRole="compact" tone="secondary">
          {summaryDescription}
        </Text>
      </Stack>

      {view.preparing.total > 0 ? (
        <Stack aria-labelledby="preparing-heading" gap="3" render="section">
          <div>
            <Text as="h3" id="preparing-heading" textRole="headline" strong>
              Getting clips ready
            </Text>
            <Text as="p" textRole="compact" tone="secondary">
              You can leave this page. Work continues on its own.
            </Text>
          </div>
          <StatusRows
            group={view.preparing}
            loading={loadingGroup === "preparing"}
            moreLabel="Show more clips being prepared"
            onOpen={openDetails}
            onMore={() => void loadMore("preparing")}
          />
        </Stack>
      ) : null}

      {view.needsHelp.total > 0 ? (
        <Stack aria-labelledby="needs-help-heading" gap="3" render="section">
          <div>
            <Text as="h3" id="needs-help-heading" textRole="headline" strong>
              Needs your help
            </Text>
            <Text as="p" textRole="compact" tone="secondary">
              Only choices Loomarr can’t safely make for you appear here.
            </Text>
          </div>
          {helpTask ? (
            <Card gap={16} padding={16}>
              <Stack align="center" direction="row" gap="control" justify="between">
                <Text as="span" textRole="hint" tone="secondary">
                  {helpIndex + 1} of {view.needsHelp.total}
                </Text>
                {view.needsHelp.total > 1 ? (
                  <Text as="span" textRole="hint" tone="secondary">
                    One choice at a time
                  </Text>
                ) : null}
              </Stack>
              <Stack
                align={{ base: "start", sm: "center" }}
                direction={{ base: "column", sm: "row" }}
                flexShrink={0}
                gap="4"
              >
                <div className="min-w-0 flex-1">
                  <Text as="p" className="break-words" textRole="cardLabel">
                    {helpTask.name}
                  </Text>
                  <Text as="p" className="mt-1" textRole="compact" tone="secondary">
                    {helpTask.question}
                  </Text>
                </div>
                <Action accessibilityRole="link" render={<a href={helpTask.actionHref} />}>
                  {helpTask.actionLabel}
                </Action>
              </Stack>
              {view.needsHelp.total > 1 ? (
                <Stack
                  borderColor="borderDecorative"
                  borderTopWidth={1}
                  direction="row"
                  gap="inline"
                  justify="end"
                  paddingTop={12}
                >
                  <Action
                    disabled={helpIndex === 0}
                    onPress={() => setHelpIndex((current) => Math.max(0, current - 1))}
                    variant="ghost"
                  >
                    Previous
                  </Action>
                  <Action
                    disabled={loadingGroup === "needsHelp" || helpIndex + 1 >= view.needsHelp.total}
                    onPress={() => void showNextHelp()}
                    variant="outline"
                  >
                    {loadingGroup === "needsHelp" ? "Loading…" : "Next"}
                  </Action>
                </Stack>
              ) : null}
            </Card>
          ) : null}
        </Stack>
      ) : null}

      {view.recentlyReady.total > 0 ? (
        <Stack aria-labelledby="ready-heading" gap="3" render="section">
          <Stack align="end" direction="row" gap="inline" justify="between" wrap>
            <div>
              <Text as="h3" id="ready-heading" textRole="headline" strong>
                Ready
              </Text>
              <Text as="p" textRole="compact" tone="secondary">
                Added in {readyWindowLabel(view.readyWindowSeconds)}. These clips stay in your Library.{" "}
                <Link
                  to="/filler/settings/$section"
                  params={{ section: "incoming" }}
                  className="underline underline-offset-4"
                >
                  Change
                </Link>
              </Text>
            </div>
            <Action accessibilityRole="link" render={<Link to="/filler/library" />} variant="outline">
              Open Library
            </Action>
          </Stack>
          <StatusRows
            group={view.recentlyReady}
            loading={loadingGroup === "recentlyReady"}
            moreLabel="Show more ready clips"
            onOpen={openDetails}
            onMore={() => void loadMore("recentlyReady")}
          />
        </Stack>
      ) : null}

      <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(undefined)}>
        {selected ? (
          <DialogContent finalFocus={detailTrigger} placement="side">
            <DialogHeader>
              <DialogTitle>{selected.name}</DialogTitle>
              <DialogDescription>{selected.statusLabel}</DialogDescription>
            </DialogHeader>
            <Stack gap="5" padding={24}>
              {selectedClip ? (
                <ClipDetails clip={selectedClip} />
              ) : (
                <div role="status">
                  <Text as="p" textRole="compact" tone="secondary">
                    {selectedClipQuery.isLoading
                      ? "Loading preview…"
                      : selectedClipQuery.error
                        ? "Preview could not be loaded."
                        : "Preview is not available yet."}
                  </Text>
                </div>
              )}
              <IncomingPreparationSummary preparation={selected.preparation} />
              <Text as="p" textRole="hint" tone="secondary">
                Updated {formatRelative(selected.updatedAt)}
              </Text>
              <IncomingProcessingDetails
                key={selected.clipHash}
                processing={selected.processing}
                timeZone={timeZone}
              />
            </Stack>
          </DialogContent>
        ) : null}
      </Dialog>
    </Stack>
  );
};

export { Incoming };
