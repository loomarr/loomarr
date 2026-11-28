import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import {
  Action,
  breakpoints,
  Card,
  Chip,
  CompactSelect,
  Details,
  Field,
  Grid,
  Icon,
  icons,
  PressableRow,
  Stack,
  StatusDot,
  Surface,
  Text,
  Tooltip,
} from "@loomarr/design-system";
import { SearchField } from "@loomarr/design-system/fields";
import { useQuery } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { type ComponentProps, useRef, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import {
  type ApplicationFilters,
  type EventOrder,
  type EventRange,
  type EventSource,
  eventSources,
  sourceNames,
} from "../application-filters";
import { DiagnosticsPager } from "../diagnostics-pager";
import { DiagnosticsSplitPane, useMinWidth } from "../diagnostics-split-pane";
import { EventDetail } from "./event-detail";
import { fallbackMessage, formatTime, levelLabel, levelTone, sourceLabel } from "./event-format";
import { eventParams } from "./event-params";
import { VerboseCaptureControl } from "./verbose-capture-control";

const VIRTUAL_VIEWPORT = { width: 960, height: 520 };

const observeVirtualRect = (
  instance: { scrollElement: Element | Window | null },
  callback: (rect: { width: number; height: number }) => void,
) => {
  const element = instance.scrollElement;
  if (!(element instanceof Element)) return;
  const report = () => {
    const rect = element.getBoundingClientRect();
    callback({
      width: rect.width || VIRTUAL_VIEWPORT.width,
      height: rect.height || VIRTUAL_VIEWPORT.height,
    });
  };
  report();
  if (typeof ResizeObserver === "undefined") return;
  const observer = new ResizeObserver(report);
  observer.observe(element);
  return () => observer.disconnect();
};

const ApplicationDiagnostics = ({
  filters,
  onFiltersChange,
  onOpenProcess,
  onOpenRelated,
}: {
  filters: ApplicationFilters;
  onFiltersChange: (filters: ApplicationFilters) => void;
  onOpenProcess?: (id: string) => void;
  onOpenRelated?: (kind: "channel" | "job", id: string) => void;
}) => {
  const [cursorStack, setCursorStack] = useState<string[]>([]);
  const [expanded, setExpanded] = useState<string>();
  const [advanced, setAdvanced] = useState(false);
  const [downloadState, setDownloadState] = useState<"idle" | "working" | "failed">("idle");
  const listRef = useRef<HTMLElement>(null);
  // The list is capped at 55% of the window until the split pane has room beside it.
  const wide = useMinWidth(breakpoints.lg);
  // A row lays its time out as a right-hand column from `sm` up.
  const columns = useMinWidth(breakpoints.sm);
  const cursor = cursorStack.at(-1);
  const query = useQuery({
    // Keyed by the log's URL, so its `diagnostic_events` frame reads it again (#3352); the window
    // is not in the key because each read ends it at that read's now.
    queryKey: [...diagnosticsApi.getListDiagnosticEventsQueryKey(), { filters, cursor }],
    queryFn: ({ signal }) =>
      diagnosticsApi.listDiagnosticEvents(eventParams(filters, cursor), {
        signal,
        headers: { Accept: "application/json" },
      }),
    retry: false,
  });
  const body = query.data?.status === 200 && "data" in query.data ? query.data.data : undefined;
  const events = body?.items ?? [];
  const dropped = body && "dropped" in body && typeof body.dropped === "number" ? body.dropped : 0;
  const counts = { debug: 0, info: 0, warn: 0, error: 0 };
  for (const event of events) counts[event.level] += 1;
  const selectedEvent = events.find((event) => event.id === expanded);
  const virtualizer = useVirtualizer({
    count: events.length,
    getScrollElement: () => listRef.current,
    getItemKey: (index) => events[index]?.id ?? index,
    estimateSize: () => 76,
    overscan: 6,
    initialRect: VIRTUAL_VIEWPORT,
    observeElementRect: observeVirtualRect,
  });

  const updateFilters = (next: ApplicationFilters) => {
    setCursorStack([]);
    setExpanded(undefined);
    onFiltersChange(next);
  };

  const download = async () => {
    setDownloadState("working");
    try {
      const response = await fetch(diagnosticsApi.getListDiagnosticEventsUrl(eventParams(filters, cursor)), {
        credentials: "same-origin",
        headers: { Accept: "application/x-ndjson" },
      });
      if (!response.ok) throw new Error(`download failed: ${response.status}`);
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = `loomarr-diagnostics-${new Date().toISOString().replaceAll(":", "-")}.ndjson`;
      anchor.click();
      URL.revokeObjectURL(url);
      setDownloadState("idle");
    } catch {
      setDownloadState("failed");
    }
  };

  const correlationFilters = [
    ["Request id", "requestId"],
    ["Playback session", "playbackSessionId"],
    ["Channel", "channelId"],
    ["Schedule block", "scheduleBlockId"],
    ["Job", "jobId"],
    ["Process run", "processRunId"],
  ] as const;

  return (
    <Stack aria-labelledby="logs-title" flexBasis="0%" flexGrow={1} flexShrink={1} gap="4" render="section">
      <Stack align="start" direction="row" gap="control" justify="between" wrap>
        <div>
          <Text as="h2" id="logs-title" textRole="headline">
            Logs
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            Recent Loomarr, player, and process events. {filters.order === "newest" ? "Newest" : "Oldest"}{" "}
            first.
          </Text>
        </div>
        <Stack direction="row" gap="inline" wrap>
          <Action
            disabled={downloadState === "working"}
            icon="download"
            onPress={() => void download()}
            variant="outline"
          >
            {downloadState === "working" ? "Preparing…" : "Download logs"}
          </Action>
        </Stack>
      </Stack>

      <Card flexDirection="row" flexWrap="wrap" gap={8} padding={12}>
        <Surface backgroundColor="transparent" borderWidth={0} flexBasis={256} flexGrow={1} minWidth={0}>
          <SearchField
            label="Search logs"
            labelHidden
            onChangeText={(text) => updateFilters({ ...filters, text })}
            placeholder="Search logs"
            value={filters.text}
          />
        </Surface>
        <CompactSelect<EventRange>
          label="Time range"
          onValueChange={(range) => updateFilters({ ...filters, range })}
          options={[
            { value: "all", label: "All retained" },
            { value: "1h", label: "Last hour" },
            { value: "6h", label: "Last 6 hours" },
            { value: "24h", label: "Last 24 hours" },
          ]}
          value={filters.range}
          width={144}
        />
        <CompactSelect<EventOrder>
          label="Log order"
          onValueChange={(order) => updateFilters({ ...filters, order })}
          options={[
            { value: "newest", label: "Newest first" },
            { value: "oldest", label: "Oldest first" },
          ]}
          value={filters.order}
          width={144}
        />
        <Action aria-expanded={advanced} icon="filter" onPress={() => setAdvanced(!advanced)} variant="ghost">
          More filters
        </Action>
      </Card>

      {advanced && (
        <Card aria-label="More log filters" gap={16} padding={16} render="section">
          <div>
            <Text as="h3" textRole="cardLabel">
              More filters
            </Text>
            <Text as="p" textRole="hint" tone="secondary">
              Use these when you already know the affected source or identifier.
            </Text>
          </div>
          <Grid columns={{ base: 1, sm: 2 }} gap="3">
            <CompactSelect<EventSource>
              label="Source"
              onValueChange={(source) => updateFilters({ ...filters, source })}
              options={[
                { value: "all", label: "All sources" },
                ...eventSources.map((source) => ({ value: source, label: sourceNames[source] })),
              ]}
              value={filters.source}
            />
            <Field
              accessibilityLabel="Subsystem"
              autoCapitalize="none"
              autoCorrect={false}
              onChangeText={(subsystem) => updateFilters({ ...filters, subsystem })}
              placeholder="Subsystem"
              value={filters.subsystem}
            />
          </Grid>
          <Details summary="IDs and correlations" textRole="cardLabel" tone="secondary">
            <Stack paddingTop={12}>
              <Grid columns={{ base: 1, sm: 2, lg: 3 }} gap="3">
                {correlationFilters.map(([label, key]) => (
                  <Field
                    accessibilityLabel={label}
                    autoCapitalize="none"
                    autoCorrect={false}
                    key={key}
                    onChangeText={(text) => updateFilters({ ...filters, [key]: text || undefined })}
                    placeholder={label}
                    value={filters[key] ?? ""}
                  />
                ))}
              </Grid>
            </Stack>
          </Details>
          <Details summary="Verbose capture" textRole="cardLabel">
            <Stack marginTop={12}>
              <VerboseCaptureControl filters={filters} />
            </Stack>
          </Details>
        </Card>
      )}

      <Stack aria-label="Severity filters" align="center" direction="row" gap="inline" render="fieldset" wrap>
        {(
          [
            ["all", events.length, "muted", "All", "Show every severity"],
            ["error", counts.error, "danger", "error", "Show only errors"],
            ["warn", counts.warn, "warning", "warning", "Show only warnings"],
            ["info", counts.info, "success", "info", "Show informational logs"],
          ] as const
        ).map(([level, count, tone, label, hint]) => (
          <Tooltip key={level} content={hint}>
            <Chip
              onPress={() => updateFilters({ ...filters, level })}
              selected={filters.level === level}
              shape="pill"
              tone={tone}
            >
              {label === "All"
                ? `${label} ${count}`
                : `${count} ${label}${label === "info" || count === 1 ? "" : "s"}`}
            </Chip>
          </Tooltip>
        ))}
        {counts.debug > 0 && (
          <Chip
            onPress={() => updateFilters({ ...filters, level: filters.level === "debug" ? "all" : "debug" })}
            selected={filters.level === "debug"}
            shape="pill"
          >
            {`${counts.debug} debug`}
          </Chip>
        )}
        {downloadState === "failed" && (
          <Text as="span" textRole="hint" tone="danger">
            <Stack align="center" direction="row" gap="1" render="span">
              <Icon decorative glyph={icons.alert} size="compact" tone="inherit" /> Download failed
            </Stack>
          </Text>
        )}
        {dropped > 0 && (
          <Text as="span" textRole="hint" tone="warning">
            <Stack align="center" direction="row" gap="1" render="span">
              <Icon decorative glyph={icons.alert} size="compact" tone="inherit" /> {dropped} events dropped
              since startup
            </Stack>
          </Text>
        )}
      </Stack>

      <DiagnosticsPager
        label="Log pages"
        page={cursorStack.length + 1}
        itemLabel="logs"
        canGoNewer={filters.order === "newest" ? cursorStack.length > 0 : Boolean(body?.nextCursor)}
        canGoOlder={filters.order === "newest" ? Boolean(body?.nextCursor) : cursorStack.length > 0}
        onNewer={() => {
          if (filters.order === "newest") setCursorStack((stack) => stack.slice(0, -1));
          else if (body?.nextCursor) setCursorStack((stack) => [...stack, body.nextCursor ?? ""]);
        }}
        onOlder={() => {
          if (filters.order === "oldest") setCursorStack((stack) => stack.slice(0, -1));
          else if (body?.nextCursor) setCursorStack((stack) => [...stack, body.nextCursor ?? ""]);
        }}
      />

      {query.isError ? (
        <ErrorState error={query.error} onRetry={() => query.refetch()} />
      ) : query.isPending ? (
        <Card paddingHorizontal={16} paddingVertical={32}>
          <Text as="p" textAlign="center" textRole="compact" tone="secondary">
            Loading retained diagnostics…
          </Text>
        </Card>
      ) : events.length === 0 ? (
        <Card paddingHorizontal={16} paddingVertical={32}>
          <Text as="p" textAlign="center" textRole="compact" tone="secondary">
            No logs match these filters. Try a longer time range or clear More filters.
          </Text>
        </Card>
      ) : (
        <DiagnosticsSplitPane
          storageKey="diagnostics-log-details"
          revealKey={expanded}
          flex={1}
          minHeight={288}
          primary={
            <Card
              ref={listRef as ComponentProps<typeof Card>["ref"]}
              render="section"
              aria-label="Logs"
              height={Math.min(Math.max(virtualizer.getTotalSize(), 228), 640)}
              maxHeight={wide ? 640 : "55vh"}
              // A Card holds its height; the list has to give way to a short pane and scroll.
              flexShrink={1}
              overflowX="hidden"
              overflowY="auto"
            >
              <Stack flexShrink={0} height={virtualizer.getTotalSize()} position="relative" width="100%">
                {virtualizer.getVirtualItems().map((virtualRow) => {
                  const event = events[virtualRow.index];
                  if (!event) return null;
                  const open = expanded === event.id;
                  return (
                    <article
                      key={event.id}
                      ref={virtualizer.measureElement}
                      data-index={virtualRow.index}
                      style={{
                        borderBottom: "1px solid var(--borderDecorative)",
                        left: 0,
                        position: "absolute",
                        top: 0,
                        transform: `translateY(${virtualRow.start}px)`,
                        width: "100%",
                      }}
                    >
                      <PressableRow
                        aria-expanded={open}
                        onPress={() => setExpanded(open ? undefined : event.id)}
                        selected={open}
                      >
                        <Grid
                          align={{ base: "stretch", sm: "start" }}
                          columns={{ base: 1, sm: ["7rem", "minmax(0,1fr)", "8rem"] }}
                          gap="2"
                          width="100%"
                        >
                          <Text as="span" textAlign="left" textRole="hint" tone="primary">
                            <Stack align="center" direction="row" gap="2" render="span">
                              <StatusDot tone={levelTone(event.level)} label={levelLabel(event.level)} />
                              {levelLabel(event.level)}
                            </Stack>
                          </Text>
                          <Stack flexShrink={1} gap="1" minWidth={0} render="span">
                            <Text as="span" textAlign="left" textRole="cardLabel" tone="primary">
                              {event.message || fallbackMessage(event)}
                            </Text>
                            <Text as="span" textAlign="left" textRole="hint" tone="secondary">
                              {sourceLabel(event.source)}
                              {event.subsystem ? ` · ${event.subsystem}` : ""}
                              {event.channelId ? " · Channel involved" : ""}
                              {event.processRunId ? " · Process output available" : ""}
                            </Text>
                          </Stack>
                          <time dateTime={new Date(event.occurredAt).toISOString()}>
                            <Text
                              as="div"
                              textAlign={columns ? "right" : "left"}
                              textRole="mono"
                              tone="secondary"
                            >
                              {formatTime(event.occurredAt)}
                            </Text>
                          </time>
                        </Grid>
                      </PressableRow>
                      {open && (
                        <Stack
                          borderColor="borderDecorative"
                          borderTopWidth={1}
                          hidden={{ base: false, lg: true }}
                        >
                          <EventDetail
                            event={event}
                            onOpenProcess={onOpenProcess}
                            onOpenRelated={onOpenRelated}
                          />
                        </Stack>
                      )}
                    </article>
                  );
                })}
              </Stack>
            </Card>
          }
          secondary={
            <Card render="aside" aria-label="Selected log" height="100%" overflowX="hidden" overflowY="auto">
              {selectedEvent ? (
                <EventDetail
                  event={selectedEvent}
                  onOpenProcess={onOpenProcess}
                  onOpenRelated={onOpenRelated}
                />
              ) : (
                <Stack gap="1" padding={24}>
                  <Text as="p" textRole="cardLabel">
                    Select a log to see more
                  </Text>
                  <Text as="p" textRole="compact" tone="secondary">
                    Related actions and technical details will appear here.
                  </Text>
                </Stack>
              )}
            </Card>
          }
        />
      )}
    </Stack>
  );
};

export { ApplicationDiagnostics };
