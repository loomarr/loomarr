import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import type { EventView } from "@loomarr/api/models/eventView";
import { ListDiagnosticEventsLevel } from "@loomarr/api/models/listDiagnosticEventsLevel";
import { ListDiagnosticEventsOrder } from "@loomarr/api/models/listDiagnosticEventsOrder";
import type { ListDiagnosticEventsParams } from "@loomarr/api/models/listDiagnosticEventsParams";
import { ListDiagnosticEventsSource } from "@loomarr/api/models/listDiagnosticEventsSource";
import { useQuery } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import {
  AlertTriangle,
  Check,
  Clipboard,
  Download,
  ListFilter,
  Radio,
  RadioTower,
  Square,
} from "lucide-react";
import { useRef, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { StatusDot, type StatusTone } from "@/components/ui/status-dot";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { DiagnosticsPager } from "../diagnostics-pager";
import { DiagnosticsSplitPane } from "../diagnostics-split-pane";

type EventRange = "all" | "1h" | "6h" | "24h";
type EventLevel = "all" | "debug" | "info" | "warn" | "error";
type EventSource = "all" | "server" | "web" | "android_tv";
type EventOrder = "newest" | "oldest";

type ApplicationFilters = {
  range: EventRange;
  order: EventOrder;
  level: EventLevel;
  source: EventSource;
  subsystem: string;
  text: string;
  requestId?: string;
  playbackSessionId?: string;
  channelId?: string;
  scheduleBlockId?: string;
  jobId?: string;
  processRunId?: string;
};

const DEFAULT_APPLICATION_FILTERS: ApplicationFilters = {
  range: "all",
  order: "newest",
  level: "all",
  source: "all",
  subsystem: "",
  text: "",
};

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

const sourceLabel = (source: EventView["source"]) => {
  if (source === "server") return "Loomarr";
  if (source === "android_tv") return "Android TV";
  return "Web player";
};

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

const CopyValue = ({ label, value }: { label: string; value: string }) => {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      variant="ghost"
      size="sm"
      className="h-auto justify-start px-2 py-1 font-mono text-xs"
      onClick={() => {
        void navigator.clipboard.writeText(value).then(() => {
          setCopied(true);
          window.setTimeout(() => setCopied(false), 1_500);
        });
      }}
      aria-label={`Copy ${label}`}
    >
      {copied ? <Check aria-hidden /> : <Clipboard aria-hidden />}
      <span className="truncate">{value}</span>
    </Button>
  );
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
  const [copied, setCopied] = useState(false);
  const correlations = [
    ["Request id", event.requestId],
    ["Playback session", event.playbackSessionId],
    ["Channel", event.channelId],
    ["Schedule block", event.scheduleBlockId],
    ["Job", event.jobId],
    ["Process run", event.processRunId],
    ["Instance", event.instanceId],
  ].filter((entry): entry is [string, string] => Boolean(entry[1]));
  const copyLog = () => {
    void navigator.clipboard.writeText(JSON.stringify(event, null, 2)).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1_500);
    });
  };
  return (
    <div className="space-y-4 bg-muted/20 p-4 text-sm">
      <div className="flex items-start justify-between gap-3">
        <div>
          <span className="inline-flex items-center gap-2 text-muted-foreground text-xs">
            <StatusDot tone={levelTone(event.level)} label={levelLabel(event.level)} />
            {levelLabel(event.level)} · {sourceLabel(event.source)}
          </span>
          <h3 className="mt-2 font-medium text-lg">{event.message || fallbackMessage(event)}</h3>
          <p className="mt-1 text-muted-foreground text-xs">
            {formatTime(event.occurredAt)}
            {event.subsystem ? ` · ${event.subsystem}` : ""}
          </p>
        </div>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant="ghost"
                size="icon"
                className="shrink-0"
                aria-label={copied ? "Copied log details" : "Copy log details"}
                onClick={copyLog}
              >
                {copied ? <Check aria-hidden /> : <Clipboard aria-hidden />}
              </Button>
            }
          />
          <TooltipContent>{copied ? "Copied" : "Copy complete log details"}</TooltipContent>
        </Tooltip>
      </div>
      <div className="flex flex-wrap gap-2">
        {event.processRunId && onOpenProcess && (
          <Button variant="outline" size="sm" onClick={() => onOpenProcess(event.processRunId ?? "")}>
            <RadioTower aria-hidden /> Open process output
          </Button>
        )}
        {event.channelId && onOpenRelated && (
          <Button variant="outline" size="sm" onClick={() => onOpenRelated("channel", event.channelId ?? "")}>
            Open related Channel
          </Button>
        )}
        {event.jobId && onOpenRelated && (
          <Button variant="outline" size="sm" onClick={() => onOpenRelated("job", event.jobId ?? "")}>
            Open related Job
          </Button>
        )}
      </div>
      <details className="rounded-md border border-border bg-background p-3">
        <summary className="cursor-pointer font-medium text-sm">Technical details</summary>
        <div className="mt-3 space-y-3">
          <p className="font-mono text-muted-foreground text-xs">{event.event}</p>
          {correlations.length > 0 && (
            <div className="grid gap-1">
              {correlations.map(([label, value]) => (
                <div key={label} className="flex min-w-0 items-center gap-1">
                  <span className="w-28 shrink-0 text-muted-foreground text-xs">{label}</span>
                  <CopyValue label={label} value={value} />
                </div>
              ))}
            </div>
          )}
          {Object.keys(event.attributes).length === 0 ? (
            <p className="text-muted-foreground text-xs">No structured attributes.</p>
          ) : (
            <pre className="max-h-56 overflow-auto rounded-md border border-border bg-card p-3 font-mono text-xs">
              {JSON.stringify(event.attributes, null, 2)}
            </pre>
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
    <section className="rounded-lg border border-border bg-card p-3" aria-labelledby="verbose-capture-title">
      <div className="flex flex-wrap items-center gap-3">
        <div className="min-w-64 flex-1">
          <h3 id="verbose-capture-title" className="font-medium text-sm">
            Verbose capture
          </h3>
          <p className="text-muted-foreground text-xs">
            Temporarily retain debug events
            {filters.subsystem.trim() ? ` for ${filters.subsystem.trim()}` : ""}
            {filters.channelId ? ` on Channel ${filters.channelId}` : ""}. The same redaction, queue, and
            retention limits still apply.
          </p>
        </div>
        {capture?.active ? (
          <>
            <span className="inline-flex items-center gap-2 text-sm">
              <Radio aria-hidden className="size-4 text-caution" />
              Active until {formatTime(capture.endsAt ?? Date.now())}
            </span>
            <Button variant="outline" size="sm" disabled={busy} onClick={() => void stopCapture()}>
              <Square aria-hidden /> Stop
            </Button>
          </>
        ) : (
          <>
            <Select value={duration} onValueChange={setDuration}>
              <SelectTrigger className="w-32" aria-label="Verbose capture duration">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="1">1 minute</SelectItem>
                <SelectItem value="5">5 minutes</SelectItem>
                <SelectItem value="10">10 minutes</SelectItem>
                <SelectItem value="15">15 minutes</SelectItem>
              </SelectContent>
            </Select>
            <Button
              variant="outline"
              size="sm"
              disabled={busy || query.isError}
              onClick={() => void startCapture()}
            >
              <Radio aria-hidden /> Start capture
            </Button>
          </>
        )}
      </div>
      {query.isError && <p className="mt-2 text-onair text-xs">Verbose capture is unavailable.</p>}
      {(start.isError || stop.isError) && (
        <p className="mt-2 text-onair text-xs">The capture control could not be updated.</p>
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
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 id="logs-title" className="font-medium text-lg">
            Logs
          </h2>
          <p className="text-muted-foreground text-sm">
            Recent Loomarr, player, and process events. {filters.order === "newest" ? "Newest" : "Oldest"}{" "}
            first.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={downloadState === "working"}
            onClick={() => void download()}
          >
            <Download aria-hidden /> {downloadState === "working" ? "Preparing…" : "Download logs"}
          </Button>
        </div>
      </div>

      <div className="flex flex-wrap gap-2 rounded-lg border border-border bg-card p-3">
        <Input
          className="min-w-64 flex-1"
          aria-label="Search logs"
          placeholder="Search logs"
          value={filters.text}
          onChange={(event) => updateFilters({ ...filters, text: event.target.value })}
        />
        <Select
          value={filters.range}
          onValueChange={(range) => updateFilters({ ...filters, range: range as EventRange })}
        >
          <SelectTrigger className="w-36" aria-label="Time range">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All retained</SelectItem>
            <SelectItem value="1h">Last hour</SelectItem>
            <SelectItem value="6h">Last 6 hours</SelectItem>
            <SelectItem value="24h">Last 24 hours</SelectItem>
          </SelectContent>
        </Select>
        <Select
          value={filters.order}
          onValueChange={(order) => updateFilters({ ...filters, order: order as EventOrder })}
        >
          <SelectTrigger className="w-36" aria-label="Log order">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="newest">Newest first</SelectItem>
            <SelectItem value="oldest">Oldest first</SelectItem>
          </SelectContent>
        </Select>
        <Button variant="ghost" aria-expanded={advanced} onClick={() => setAdvanced(!advanced)}>
          <ListFilter aria-hidden /> More filters
        </Button>
      </div>

      {advanced && (
        <section
          className="space-y-4 rounded-lg border border-border bg-card p-4"
          aria-label="More log filters"
        >
          <div>
            <h3 className="font-medium text-sm">More filters</h3>
            <p className="text-muted-foreground text-xs">
              Use these when you already know the affected source or identifier.
            </p>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <Select
              value={filters.source}
              onValueChange={(source) => updateFilters({ ...filters, source: source as EventSource })}
            >
              <SelectTrigger aria-label="Source">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All sources</SelectItem>
                <SelectItem value="server">Loomarr</SelectItem>
                <SelectItem value="web">Web player</SelectItem>
                <SelectItem value="android_tv">Android TV</SelectItem>
              </SelectContent>
            </Select>
            <Input
              aria-label="Subsystem"
              placeholder="Subsystem"
              value={filters.subsystem}
              onChange={(event) => updateFilters({ ...filters, subsystem: event.target.value })}
            />
          </div>
          <details>
            <summary className="cursor-pointer font-medium text-muted-foreground text-sm">
              IDs and correlations
            </summary>
            <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {correlationFilters.map(([label, key]) => (
                <Input
                  key={key}
                  aria-label={label}
                  placeholder={label}
                  value={filters[key] ?? ""}
                  onChange={(event) => updateFilters({ ...filters, [key]: event.target.value || undefined })}
                />
              ))}
            </div>
          </details>
          <details>
            <summary className="cursor-pointer font-medium text-sm">Verbose capture</summary>
            <div className="mt-3">
              <VerboseCaptureControl filters={filters} />
            </div>
          </details>
        </section>
      )}

      <fieldset className="flex flex-wrap items-center gap-2 text-xs">
        <legend className="sr-only">Severity filters</legend>
        {(
          [
            ["all", events.length, "text-foreground", "bg-muted", "All", "Show every severity"],
            ["error", counts.error, "text-onair", "bg-onair-tint-15", "error", "Show only errors"],
            ["warn", counts.warn, "text-caution", "bg-caution-tint-15", "warning", "Show only warnings"],
            ["info", counts.info, "text-lock", "bg-lock-tint-15", "info", "Show informational logs"],
          ] as const
        ).map(([level, count, color, fill, label, hint]) => (
          <Tooltip key={level}>
            <TooltipTrigger
              render={
                <button
                  type="button"
                  className={`cursor-pointer rounded-full border px-3 py-1 font-medium transition-colors ${color} ${filters.level === level ? `border-current ${fill} shadow-sm` : "border-border hover:border-current hover:bg-muted/30"}`}
                  aria-pressed={filters.level === level}
                  onClick={() => updateFilters({ ...filters, level })}
                >
                  {label === "All"
                    ? `${label} ${count}`
                    : `${count} ${label}${label === "info" || count === 1 ? "" : "s"}`}
                </button>
              }
            />
            <TooltipContent>{hint}</TooltipContent>
          </Tooltip>
        ))}
        {counts.debug > 0 && (
          <button
            type="button"
            className={`cursor-pointer rounded-full border px-2.5 py-1 font-medium text-muted-foreground ${filters.level === "debug" ? "border-current bg-current/10" : "border-border hover:border-current"}`}
            aria-pressed={filters.level === "debug"}
            onClick={() => updateFilters({ ...filters, level: filters.level === "debug" ? "all" : "debug" })}
          >
            {counts.debug} debug
          </button>
        )}
        {downloadState === "failed" && (
          <span className="inline-flex items-center gap-1 text-onair">
            <AlertTriangle aria-hidden className="size-3" /> Download failed
          </span>
        )}
        {dropped > 0 && (
          <span className="inline-flex items-center gap-1 text-caution">
            <AlertTriangle aria-hidden className="size-3" /> {dropped} events dropped since startup
          </span>
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
        <p className="rounded-lg border border-border bg-card px-4 py-8 text-center text-muted-foreground text-sm">
          Loading retained diagnostics…
        </p>
      ) : events.length === 0 ? (
        <p className="rounded-lg border border-border bg-card px-4 py-8 text-center text-muted-foreground text-sm">
          No logs match these filters. Try a longer time range or clear More filters.
        </p>
      ) : (
        <DiagnosticsSplitPane
          storageKey="diagnostics-log-details"
          revealKey={expanded}
          className="min-h-72 flex-1"
          primary={
            <section
              ref={listRef}
              className="max-h-[55vh] overflow-y-auto overflow-x-hidden rounded-lg border border-border bg-card lg:max-h-[40rem]"
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
                      className="absolute top-0 left-0 w-full border-border border-b"
                      style={{ transform: `translateY(${virtualRow.start}px)` }}
                    >
                      <button
                        type="button"
                        className={`grid w-full cursor-pointer gap-2 px-4 py-3 text-left hover:bg-muted/20 sm:grid-cols-[7rem_minmax(0,1fr)_8rem] sm:items-start ${open ? "bg-muted/20" : ""}`}
                        aria-expanded={open}
                        onClick={() => setExpanded(open ? undefined : event.id)}
                      >
                        <span className="inline-flex items-center gap-2 text-xs">
                          <StatusDot tone={levelTone(event.level)} label={levelLabel(event.level)} />
                          {levelLabel(event.level)}
                        </span>
                        <span className="min-w-0">
                          <span className="block font-medium text-sm">
                            {event.message || fallbackMessage(event)}
                          </span>
                          <span className="mt-1 block text-muted-foreground text-xs">
                            {sourceLabel(event.source)}
                            {event.subsystem ? ` · ${event.subsystem}` : ""}
                            {event.channelId ? " · Channel involved" : ""}
                            {event.processRunId ? " · Process output available" : ""}
                          </span>
                        </span>
                        <time
                          className="font-mono text-muted-foreground text-xs sm:text-right"
                          dateTime={new Date(event.occurredAt).toISOString()}
                        >
                          {formatTime(event.occurredAt)}
                        </time>
                      </button>
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
              className="h-full overflow-y-auto overflow-x-hidden rounded-lg border border-border bg-card"
              aria-label="Selected log"
            >
              {selectedEvent ? (
                <EventDetail
                  event={selectedEvent}
                  onOpenProcess={onOpenProcess}
                  onOpenRelated={onOpenRelated}
                />
              ) : (
                <div className="p-6 text-muted-foreground text-sm">
                  <p className="font-medium text-foreground">Select a log to see more</p>
                  <p className="mt-1">Related actions and technical details will appear here.</p>
                </div>
              )}
            </aside>
          }
        />
      )}
    </section>
  );
};

export type { ApplicationFilters, EventLevel, EventOrder, EventRange, EventSource };
export { ApplicationDiagnostics, DEFAULT_APPLICATION_FILTERS };
