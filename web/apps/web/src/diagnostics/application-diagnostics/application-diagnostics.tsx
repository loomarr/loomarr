import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import type { EventView } from "@loomarr/api/models/eventView";
import { ListDiagnosticEventsLevel } from "@loomarr/api/models/listDiagnosticEventsLevel";
import { ListDiagnosticEventsOrder } from "@loomarr/api/models/listDiagnosticEventsOrder";
import type { ListDiagnosticEventsParams } from "@loomarr/api/models/listDiagnosticEventsParams";
import { ListDiagnosticEventsSource } from "@loomarr/api/models/listDiagnosticEventsSource";
import {
  Action,
  Chip,
  CompactSelect,
  Field,
  Icon,
  icons,
  PressableRow,
  Stack,
  StatusDot,
  type StatusTone,
  Surface,
  Text,
  Tooltip,
} from "@loomarr/design-system";
import { SearchField } from "@loomarr/ui";
import { useQuery } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useRef, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useCopied } from "@/lib/use-copied";
import {
  type ApplicationFilters,
  type EventOrder,
  type EventRange,
  type EventSource,
  eventSources,
  sourceNames,
} from "../application-filters";
import { CopyValue } from "../copy-value";
import { DiagnosticsPager } from "../diagnostics-pager";
import { DiagnosticsSplitPane } from "../diagnostics-split-pane";

const RANGE_MS: Record<Exclude<EventRange, "all">, number> = {
  "1h": 3_600_000,
  "6h": 21_600_000,
  "24h": 86_400_000,
};
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

const formatTime = (millis: number) =>
  new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "medium" }).format(new Date(millis));

const levelTone = (level: EventView["level"]): StatusTone => {
  if (level === "error") return "error";
  if (level === "warn") return "warn";
  if (level === "info") return "ok";
  return "off";
};

const levelLabel = (level: EventView["level"]) => {
  if (level === "warn") return "Warning";
  return `${level.charAt(0).toUpperCase()}${level.slice(1)}`;
};

const sourceLabel = (source: EventView["source"]) => sourceNames[source];

const fallbackMessage = (event: EventView) =>
  event.event
    .split(/[._]/)
    .filter(Boolean)
    .map((part) => `${part.charAt(0).toUpperCase()}${part.slice(1)}`)
    .join(" ");

const eventParams = (
  filters: ApplicationFilters,
  cursor?: string,
  now = Date.now(),
): ListDiagnosticEventsParams => {
  const to = now;
  // Zero asks the API to apply its one-hour default, so one millisecond after the epoch is the
  // explicit lower bound for every retained Loomarr event.
  const timeWindow = filters.range === "all" ? { from: 1, to } : { from: to - RANGE_MS[filters.range], to };
  return {
    ...timeWindow,
    limit: 50,
    order: ListDiagnosticEventsOrder[filters.order],
    ...(cursor ? { cursor } : {}),
    ...(filters.level === "all" ? {} : { level: ListDiagnosticEventsLevel[filters.level] }),
    ...(filters.source === "all" ? {} : { source: ListDiagnosticEventsSource[filters.source] }),
    ...(filters.subsystem.trim() ? { subsystem: filters.subsystem.trim() } : {}),
    ...(filters.text.trim() ? { text: filters.text.trim() } : {}),
    ...(filters.requestId ? { requestId: filters.requestId } : {}),
    ...(filters.playbackSessionId ? { playbackSessionId: filters.playbackSessionId } : {}),
    ...(filters.channelId ? { channelId: filters.channelId } : {}),
    ...(filters.scheduleBlockId ? { scheduleBlockId: filters.scheduleBlockId } : {}),
    ...(filters.jobId ? { jobId: filters.jobId } : {}),
    ...(filters.processRunId ? { processRunId: filters.processRunId } : {}),
  };
};

const EventDetail = ({
  event,
  onOpenProcess,
  onOpenRelated,
}: {
  event: EventView;
  onOpenProcess?: (id: string) => void;
  onOpenRelated?: (kind: "channel" | "job", id: string) => void;
}) => {
  const { copied, copy } = useCopied();
  const correlations = [
    ["Request id", event.requestId],
    ["Playback session", event.playbackSessionId],
    ["Channel", event.channelId],
    ["Schedule block", event.scheduleBlockId],
    ["Job", event.jobId],
    ["Process run", event.processRunId],
    ["Instance", event.instanceId],
  ].filter((entry): entry is [string, string] => Boolean(entry[1]));
  const copyLog = () => void copy(JSON.stringify(event, null, 2));
  return (
    <div className="bg-muted/20 space-y-4 p-4">
      <Stack align="start" direction="row" gap="control" justify="between">
        <div>
          <Text as="span" className="inline-flex items-center gap-2" textRole="hint" tone="secondary">
            <StatusDot tone={levelTone(event.level)} label={levelLabel(event.level)} />
            {levelLabel(event.level)} · {sourceLabel(event.source)}
          </Text>
          <Text as="h3" className="mt-2" textRole="headline">
            {event.message || fallbackMessage(event)}
          </Text>
          <Text as="p" className="mt-1" textRole="hint" tone="secondary">
            {formatTime(event.occurredAt)}
            {event.subsystem ? ` · ${event.subsystem}` : ""}
          </Text>
        </div>
        <Tooltip content={copied ? "Copied" : "Copy complete log details"}>
          <Action
            icon={copied ? "success" : "copy"}
            layout="icon"
            onPress={copyLog}
            style={{ flexShrink: 0 }}
            variant="ghost"
          >
            {copied ? "Copied log details" : "Copy log details"}
          </Action>
        </Tooltip>
      </Stack>
      <Stack direction="row" gap="inline" wrap>
        {event.processRunId && onOpenProcess && (
          <Action icon="process" onPress={() => onOpenProcess(event.processRunId ?? "")} variant="outline">
            Open process output
          </Action>
        )}
        {event.channelId && onOpenRelated && (
          <Action onPress={() => onOpenRelated("channel", event.channelId ?? "")} variant="outline">
            Open related Channel
          </Action>
        )}
        {event.jobId && onOpenRelated && (
          <Action onPress={() => onOpenRelated("job", event.jobId ?? "")} variant="outline">
            Open related Job
          </Action>
        )}
      </Stack>
      <details className="border-border bg-background rounded-md border p-3">
        <Text as="summary" className="cursor-pointer" textRole="cardLabel">
          Technical details
        </Text>
        <div className="mt-3 space-y-3">
          <Text as="p" textRole="mono">
            {event.event}
          </Text>
          {correlations.length > 0 && (
            <div className="grid gap-1">
              {correlations.map(([label, value]) => (
                <Stack align="center" className="min-w-0" direction="row" gap="1" key={label}>
                  <Text as="span" className="w-28 shrink-0" textRole="hint" tone="secondary">
                    {label}
                  </Text>
                  <CopyValue label={label} value={value} />
                </Stack>
              ))}
            </div>
          )}
          {Object.keys(event.attributes).length === 0 ? (
            <Text as="p" textRole="hint" tone="secondary">
              No structured attributes.
            </Text>
          ) : (
            <Text
              as="pre"
              className="border-border bg-card max-h-56 overflow-auto rounded-md border p-3"
              textRole="mono"
              tone="primary"
            >
              {JSON.stringify(event.attributes, null, 2)}
            </Text>
          )}
        </div>
      </details>
    </div>
  );
};

const VerboseCaptureControl = ({ filters }: { filters: ApplicationFilters }) => {
  const [duration, setDuration] = useState("5");
  const query = diagnosticsApi.useGetDiagnosticVerboseCapture({
    query: { retry: false, refetchInterval: 5_000 },
  });
  const start = diagnosticsApi.useStartDiagnosticVerboseCapture();
  const stop = diagnosticsApi.useStopDiagnosticVerboseCapture();
  const capture = query.data?.status === 200 ? query.data.data : undefined;
  const busy = start.isPending || stop.isPending;
  const startCapture = async () => {
    await start.mutateAsync({
      data: {
        durationMinutes: Number(duration),
        ...(filters.subsystem.trim() ? { subsystem: filters.subsystem.trim() } : {}),
        ...(filters.channelId ? { channelId: filters.channelId } : {}),
      },
    });
    await query.refetch();
  };
  const stopCapture = async () => {
    await stop.mutateAsync();
    await query.refetch();
  };
  return (
    <section className="border-border bg-card rounded-lg border p-3" aria-labelledby="verbose-capture-title">
      <Stack align="center" direction="row" gap="control" wrap>
        <div className="min-w-64 flex-1">
          <Text as="h3" id="verbose-capture-title" textRole="cardLabel">
            Verbose capture
          </Text>
          <Text as="p" textRole="hint" tone="secondary">
            Temporarily retain debug events
            {filters.subsystem.trim() ? ` for ${filters.subsystem.trim()}` : ""}
            {filters.channelId ? ` on Channel ${filters.channelId}` : ""}. The same redaction, queue, and
            retention limits still apply.
          </Text>
        </div>
        {capture?.active ? (
          <>
            <Text as="span" className="inline-flex items-center gap-2" textRole="compact">
              <Icon decorative glyph={icons.capture} size="compact" tone="warning" />
              Active until {formatTime(capture.endsAt ?? Date.now())}
            </Text>
            <Action disabled={busy} icon="stop" onPress={() => void stopCapture()} variant="outline">
              Stop
            </Action>
          </>
        ) : (
          <>
            <CompactSelect
              label="Verbose capture duration"
              onValueChange={setDuration}
              options={[
                { value: "1", label: "1 minute" },
                { value: "5", label: "5 minutes" },
                { value: "10", label: "10 minutes" },
                { value: "15", label: "15 minutes" },
              ]}
              value={duration}
              width={128}
            />
            <Action
              disabled={busy || query.isError}
              icon="capture"
              onPress={() => void startCapture()}
              variant="outline"
            >
              Start capture
            </Action>
          </>
        )}
      </Stack>
      {query.isError && (
        <Text as="p" className="mt-2" textRole="hint" tone="danger">
          Verbose capture is unavailable.
        </Text>
      )}
      {(start.isError || stop.isError) && (
        <Text as="p" className="mt-2" textRole="hint" tone="danger">
          The capture control could not be updated.
        </Text>
      )}
    </section>
  );
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
  const cursor = cursorStack.at(-1);
  const query = useQuery({
    queryKey: ["diagnostic-events-page", filters, cursor],
    queryFn: ({ signal }) =>
      diagnosticsApi.listDiagnosticEvents(eventParams(filters, cursor), {
        signal,
        headers: { Accept: "application/json" },
      }),
    retry: false,
    refetchInterval: cursor ? false : 5_000,
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
    <section className="flex min-h-0 flex-1 flex-col gap-4" aria-labelledby="logs-title">
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

      <Stack className="border-border bg-card rounded-lg border p-3" direction="row" gap="inline" wrap>
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
      </Stack>

      {advanced && (
        <section
          className="border-border bg-card space-y-4 rounded-lg border p-4"
          aria-label="More log filters"
        >
          <div>
            <Text as="h3" textRole="cardLabel">
              More filters
            </Text>
            <Text as="p" textRole="hint" tone="secondary">
              Use these when you already know the affected source or identifier.
            </Text>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
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
          </div>
          <details>
            <Text as="summary" className="cursor-pointer" textRole="cardLabel" tone="secondary">
              IDs and correlations
            </Text>
            <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
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
            </div>
          </details>
          <details>
            <Text as="summary" className="cursor-pointer" textRole="cardLabel">
              Verbose capture
            </Text>
            <div className="mt-3">
              <VerboseCaptureControl filters={filters} />
            </div>
          </details>
        </section>
      )}

      <fieldset className="flex flex-wrap items-center gap-2">
        <legend className="sr-only">Severity filters</legend>
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
          <Text as="span" className="inline-flex items-center gap-1" textRole="hint" tone="danger">
            <Icon decorative glyph={icons.alert} size="compact" tone="inherit" /> Download failed
          </Text>
        )}
        {dropped > 0 && (
          <Text as="span" className="inline-flex items-center gap-1" textRole="hint" tone="warning">
            <Icon decorative glyph={icons.alert} size="compact" tone="inherit" /> {dropped} events dropped
            since startup
          </Text>
        )}
      </fieldset>

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
        <Text
          as="p"
          className="border-border bg-card rounded-lg border px-4 py-8 text-center"
          textRole="compact"
          tone="secondary"
        >
          Loading retained diagnostics…
        </Text>
      ) : events.length === 0 ? (
        <Text
          as="p"
          className="border-border bg-card rounded-lg border px-4 py-8 text-center"
          textRole="compact"
          tone="secondary"
        >
          No logs match these filters. Try a longer time range or clear More filters.
        </Text>
      ) : (
        <DiagnosticsSplitPane
          storageKey="diagnostics-log-details"
          revealKey={expanded}
          className="min-h-72 flex-1"
          primary={
            <section
              ref={listRef}
              className="border-border bg-card max-h-[55vh] overflow-x-hidden overflow-y-auto rounded-lg border lg:max-h-[40rem]"
              aria-label="Logs"
              style={{ height: `${Math.min(Math.max(virtualizer.getTotalSize(), 228), 640)}px` }}
            >
              <div className="relative w-full" style={{ height: `${virtualizer.getTotalSize()}px` }}>
                {virtualizer.getVirtualItems().map((virtualRow) => {
                  const event = events[virtualRow.index];
                  if (!event) return null;
                  const open = expanded === event.id;
                  return (
                    <article
                      key={event.id}
                      ref={virtualizer.measureElement}
                      data-index={virtualRow.index}
                      className="border-border absolute top-0 left-0 w-full border-b"
                      style={{ transform: `translateY(${virtualRow.start}px)` }}
                    >
                      <PressableRow
                        aria-expanded={open}
                        onPress={() => setExpanded(open ? undefined : event.id)}
                        selected={open}
                      >
                        <div className="grid w-full gap-2 text-left sm:grid-cols-[7rem_minmax(0,1fr)_8rem] sm:items-start">
                          <Text
                            as="span"
                            className="inline-flex items-center gap-2"
                            textRole="hint"
                            tone="primary"
                          >
                            <StatusDot tone={levelTone(event.level)} label={levelLabel(event.level)} />
                            {levelLabel(event.level)}
                          </Text>
                          <span className="min-w-0">
                            <Text as="span" className="block" textRole="cardLabel" tone="primary">
                              {event.message || fallbackMessage(event)}
                            </Text>
                            <Text as="span" className="mt-1 block" textRole="hint" tone="secondary">
                              {sourceLabel(event.source)}
                              {event.subsystem ? ` · ${event.subsystem}` : ""}
                              {event.channelId ? " · Channel involved" : ""}
                              {event.processRunId ? " · Process output available" : ""}
                            </Text>
                          </span>
                          <time className="sm:text-right" dateTime={new Date(event.occurredAt).toISOString()}>
                            <Text as="span" textRole="mono" tone="secondary">
                              {formatTime(event.occurredAt)}
                            </Text>
                          </time>
                        </div>
                      </PressableRow>
                      {open && (
                        <div className="border-border border-t lg:hidden">
                          <EventDetail
                            event={event}
                            onOpenProcess={onOpenProcess}
                            onOpenRelated={onOpenRelated}
                          />
                        </div>
                      )}
                    </article>
                  );
                })}
              </div>
            </section>
          }
          secondary={
            <aside
              className="border-border bg-card h-full overflow-x-hidden overflow-y-auto rounded-lg border"
              aria-label="Selected log"
            >
              {selectedEvent ? (
                <EventDetail
                  event={selectedEvent}
                  onOpenProcess={onOpenProcess}
                  onOpenRelated={onOpenRelated}
                />
              ) : (
                <div className="p-6">
                  <Text as="p" textRole="cardLabel">
                    Select a log to see more
                  </Text>
                  <Text as="p" className="mt-1" textRole="compact" tone="secondary">
                    Related actions and technical details will appear here.
                  </Text>
                </div>
              )}
            </aside>
          }
        />
      )}
    </section>
  );
};

export { ApplicationDiagnostics };
