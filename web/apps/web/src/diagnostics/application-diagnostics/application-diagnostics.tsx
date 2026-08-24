import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import type { EventView } from "@loomarr/api/models/eventView";
import { ListDiagnosticEventsLevel } from "@loomarr/api/models/listDiagnosticEventsLevel";
import type { ListDiagnosticEventsParams } from "@loomarr/api/models/listDiagnosticEventsParams";
import { ListDiagnosticEventsSource } from "@loomarr/api/models/listDiagnosticEventsSource";
import { AlertTriangle, Check, Clipboard, Download, Radio, RotateCcw, Square } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { StatusDot, type StatusTone } from "@/components/ui/status-dot";

type EventRange = "1h" | "6h" | "24h";
type EventLevel = "all" | "debug" | "info" | "warn" | "error";
type EventSource = "all" | "server" | "web" | "android_tv";

type ApplicationFilters = {
  range: EventRange;
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
  range: "1h",
  level: "all",
  source: "all",
  subsystem: "",
  text: "",
};

const RANGE_MS: Record<EventRange, number> = { "1h": 3_600_000, "6h": 21_600_000, "24h": 86_400_000 };

const formatTime = (millis: number) =>
  new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "medium" }).format(new Date(millis));

const levelTone = (level: EventView["level"]): StatusTone => {
  if (level === "error") return "error";
  if (level === "warn") return "warn";
  if (level === "info") return "ok";
  return "off";
};

const eventParams = (
  filters: ApplicationFilters,
  cursor?: string,
  now = Date.now(),
): ListDiagnosticEventsParams => {
  const to = now;
  return {
    from: to - RANGE_MS[filters.range],
    to,
    limit: 200,
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
  const correlations = [
    ["Request id", event.requestId],
    ["Playback session", event.playbackSessionId],
    ["Channel", event.channelId],
    ["Schedule block", event.scheduleBlockId],
    ["Job", event.jobId],
    ["Process run", event.processRunId],
    ["Instance", event.instanceId],
  ].filter((entry): entry is [string, string] => Boolean(entry[1]));
  return (
    <div className="space-y-3 border-border border-t bg-muted/20 px-4 py-4 text-sm">
      {event.message && <p>{event.message}</p>}
      {correlations.length > 0 && (
        <div className="grid gap-1 sm:grid-cols-2">
          {correlations.map(([label, value]) => (
            <div key={label} className="flex min-w-0 items-center gap-1">
              <span className="w-28 shrink-0 text-muted-foreground text-xs">{label}</span>
              <CopyValue label={label} value={value} />
              {label === "Process run" && onOpenProcess && (
                <Button
                  variant="link"
                  size="sm"
                  className="h-auto px-1 py-0"
                  onClick={() => onOpenProcess(value)}
                  aria-label="Open Process run"
                >
                  Open
                </Button>
              )}
              {label === "Channel" && onOpenRelated && (
                <Button
                  variant="link"
                  size="sm"
                  className="h-auto px-1 py-0"
                  onClick={() => onOpenRelated("channel", value)}
                  aria-label="Open Channel"
                >
                  Open
                </Button>
              )}
              {label === "Job" && onOpenRelated && (
                <Button
                  variant="link"
                  size="sm"
                  className="h-auto px-1 py-0"
                  onClick={() => onOpenRelated("job", value)}
                  aria-label="Open Job"
                >
                  Open
                </Button>
              )}
            </div>
          ))}
        </div>
      )}
      <div>
        <h4 className="mb-1 font-medium text-xs">Structured attributes</h4>
        {Object.keys(event.attributes).length === 0 ? (
          <p className="text-muted-foreground text-xs">No structured attributes.</p>
        ) : (
          <pre className="max-h-56 overflow-auto rounded-md border border-border bg-background p-3 font-mono text-xs">
            {JSON.stringify(event.attributes, null, 2)}
          </pre>
        )}
      </div>
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
      {query.isError && <p className="mt-2 text-danger text-xs">Verbose capture is unavailable.</p>}
      {(start.isError || stop.isError) && (
        <p className="mt-2 text-danger text-xs">The capture control could not be updated.</p>
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
  const [downloadState, setDownloadState] = useState<"idle" | "working" | "failed">("idle");
  const [now, setNow] = useState(() => Date.now());
  const cursor = cursorStack.at(-1);
  const params = useMemo(() => eventParams(filters, cursor, now), [filters, cursor, now]);
  const query = diagnosticsApi.useListDiagnosticEvents(params, {
    query: { retry: false },
    request: { headers: { Accept: "application/json" } },
  });
  const body = query.data?.status === 200 && "data" in query.data ? query.data.data : undefined;
  const events = body?.items ?? [];
  const dropped = body && "dropped" in body && typeof body.dropped === "number" ? body.dropped : 0;
  useEffect(() => {
    if (cursor) return;
    const interval = window.setInterval(() => setNow(Date.now()), 15_000);
    return () => window.clearInterval(interval);
  }, [cursor]);

  const counts = { debug: 0, info: 0, warn: 0, error: 0 };
  for (const event of events) counts[event.level] += 1;

  const updateFilters = (next: ApplicationFilters) => {
    setCursorStack([]);
    setExpanded(undefined);
    setNow(Date.now());
    onFiltersChange(next);
  };

  const download = async () => {
    setDownloadState("working");
    try {
      const response = await fetch(diagnosticsApi.getListDiagnosticEventsUrl(params), {
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

  return (
    <section className="flex min-h-0 flex-1 flex-col gap-4" aria-labelledby="application-diagnostics-title">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 id="application-diagnostics-title" className="font-medium text-lg">
            Application
          </h2>
          <p className="text-muted-foreground text-sm">
            Server and client observations, newest first. Filters are reflected in this page's URL.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          disabled={downloadState === "working"}
          onClick={() => void download()}
        >
          <Download aria-hidden />{" "}
          {downloadState === "working" ? "Preparing…" : "Download this page (NDJSON)"}
        </Button>
      </div>

      <div className="grid gap-3 rounded-lg border border-border bg-card p-3 sm:grid-cols-2 lg:grid-cols-5">
        <Select
          value={filters.range}
          onValueChange={(range) => updateFilters({ ...filters, range: range as EventRange })}
        >
          <SelectTrigger aria-label="Time range">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="1h">Last hour</SelectItem>
            <SelectItem value="6h">Last 6 hours</SelectItem>
            <SelectItem value="24h">Last 24 hours</SelectItem>
          </SelectContent>
        </Select>
        <Select
          value={filters.level}
          onValueChange={(level) => updateFilters({ ...filters, level: level as EventLevel })}
        >
          <SelectTrigger aria-label="Severity">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All severities</SelectItem>
            <SelectItem value="error">Error</SelectItem>
            <SelectItem value="warn">Warning</SelectItem>
            <SelectItem value="info">Info</SelectItem>
            <SelectItem value="debug">Debug</SelectItem>
          </SelectContent>
        </Select>
        <Select
          value={filters.source}
          onValueChange={(source) => updateFilters({ ...filters, source: source as EventSource })}
        >
          <SelectTrigger aria-label="Source">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All sources</SelectItem>
            <SelectItem value="server">Server</SelectItem>
            <SelectItem value="web">Web</SelectItem>
            <SelectItem value="android_tv">Android TV</SelectItem>
          </SelectContent>
        </Select>
        <Input
          aria-label="Subsystem"
          placeholder="Subsystem"
          value={filters.subsystem}
          onChange={(event) => updateFilters({ ...filters, subsystem: event.target.value })}
        />
        <Input
          aria-label="Search diagnostics"
          placeholder="Search events and attributes"
          value={filters.text}
          onChange={(event) => updateFilters({ ...filters, text: event.target.value })}
        />
      </div>

      <VerboseCaptureControl filters={filters} />

      <div
        className="flex flex-wrap items-center gap-4 text-xs"
        role="status"
        aria-label="Current page summary"
      >
        <span className="text-muted-foreground">This page</span>
        <span>{counts.error} errors</span>
        <span>{counts.warn} warnings</span>
        <span>{counts.info} info</span>
        <span>{counts.debug} debug</span>
        {downloadState === "failed" && (
          <span className="inline-flex items-center gap-1 text-danger">
            <AlertTriangle aria-hidden className="size-3" /> Download failed
          </span>
        )}
        {dropped > 0 && (
          <span className="inline-flex items-center gap-1 text-caution">
            <AlertTriangle aria-hidden className="size-3" /> {dropped} events dropped since startup
          </span>
        )}
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto"
          onClick={() => query.refetch()}
          disabled={query.isFetching}
        >
          <RotateCcw aria-hidden /> Refresh
        </Button>
      </div>

      {query.isError ? (
        <ErrorState error={query.error} onRetry={() => query.refetch()} />
      ) : query.isPending ? (
        <p className="rounded-lg border border-border bg-card px-4 py-8 text-center text-muted-foreground text-sm">
          Loading retained diagnostics…
        </p>
      ) : events.length === 0 ? (
        <p className="rounded-lg border border-border bg-card px-4 py-8 text-center text-muted-foreground text-sm">
          No retained events match this bounded window.
        </p>
      ) : (
        <section
          className="min-h-72 flex-1 overflow-auto rounded-lg border border-border bg-card"
          aria-label="Diagnostic event timeline"
        >
          <div>
            {events.map((event) => {
              const open = expanded === event.id;
              return (
                <article
                  key={event.id}
                  className="border-border border-b [contain-intrinsic-size:auto_72px] [content-visibility:auto]"
                >
                  <button
                    type="button"
                    className="grid w-full gap-2 px-4 py-3 text-left hover:bg-muted/20 sm:grid-cols-[8rem_7rem_minmax(0,1fr)_auto] sm:items-center"
                    aria-expanded={open}
                    onClick={() => setExpanded(open ? undefined : event.id)}
                  >
                    <time
                      className="font-mono text-muted-foreground text-xs"
                      dateTime={new Date(event.occurredAt).toISOString()}
                    >
                      {formatTime(event.occurredAt)}
                    </time>
                    <span className="inline-flex items-center gap-2 text-xs">
                      <StatusDot tone={levelTone(event.level)} label={event.level} />
                      {event.level}
                    </span>
                    <span className="min-w-0">
                      <span className="block truncate font-medium text-sm">{event.event}</span>
                      <span className="block truncate text-muted-foreground text-xs">
                        {event.source}
                        {event.subsystem ? ` · ${event.subsystem}` : ""}
                        {event.message ? ` · ${event.message}` : ""}
                      </span>
                    </span>
                    <span className="text-muted-foreground text-xs">{open ? "Hide" : "Details"}</span>
                  </button>
                  {open && (
                    <EventDetail event={event} onOpenProcess={onOpenProcess} onOpenRelated={onOpenRelated} />
                  )}
                </article>
              );
            })}
          </div>
        </section>
      )}

      <div className="flex items-center justify-between">
        <Button
          variant="outline"
          size="sm"
          disabled={cursorStack.length === 0}
          onClick={() => setCursorStack((stack) => stack.slice(0, -1))}
        >
          Newer
        </Button>
        <span className="text-muted-foreground text-xs">
          Page {cursorStack.length + 1} · at most 200 records
        </span>
        <Button
          variant="outline"
          size="sm"
          disabled={!body?.nextCursor}
          onClick={() => body?.nextCursor && setCursorStack((stack) => [...stack, body.nextCursor ?? ""])}
        >
          Older
        </Button>
      </div>
    </section>
  );
};

export type { ApplicationFilters, EventLevel, EventRange, EventSource };
export { ApplicationDiagnostics, DEFAULT_APPLICATION_FILTERS };
