import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import type { ListDiagnosticProcessesParams } from "@loomarr/api/models/listDiagnosticProcessesParams";
import { ListDiagnosticProcessesStatus } from "@loomarr/api/models/listDiagnosticProcessesStatus";
import type { ProcessRunView } from "@loomarr/api/models/processRunView";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, Check, Clipboard, Download, ExternalLink, RotateCcw } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { StatusDot, type StatusTone } from "@/components/ui/status-dot";
import type { DiagnosticsSearch } from "../diagnostics-page";

const RANGE_MS = { "1h": 3_600_000, "6h": 21_600_000, "24h": 86_400_000 } as const;

type OutputResult = {
  text: string;
  discarded: number;
  truncated: boolean;
};

class OutputError extends Error {
  constructor(readonly status: number) {
    super(`Process output request failed: ${status}`);
  }
}

const formatTime = (millis?: number) =>
  millis
    ? new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "medium" }).format(
        new Date(millis),
      )
    : "—";

const tone = (status: ProcessRunView["status"]): StatusTone => {
  if (status === "failed") return "error";
  if (status === "running") return "warn";
  if (status === "succeeded") return "ok";
  return "off";
};

const readOutput = async (id: string): Promise<OutputResult> => {
  const response = await fetch(diagnosticsApi.getGetDiagnosticProcessOutputUrl(id), {
    credentials: "same-origin",
    headers: { Accept: "text/plain" },
  });
  if (!response.ok) throw new OutputError(response.status);
  return {
    text: await response.text(),
    discarded: Number(response.headers.get("X-Diagnostic-Discarded-Lines") ?? 0),
    truncated: response.headers.get("X-Diagnostic-Truncated") === "true",
  };
};

const CopyValue = ({ label, value }: { label: string; value: string }) => {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      variant="ghost"
      size="sm"
      className="h-auto max-w-full justify-start px-2 py-1 font-mono text-xs"
      aria-label={`Copy ${label}`}
      onClick={() =>
        void navigator.clipboard.writeText(value).then(() => {
          setCopied(true);
          window.setTimeout(() => setCopied(false), 1_500);
        })
      }
    >
      {copied ? <Check aria-hidden /> : <Clipboard aria-hidden />}
      <span className="truncate">{value}</span>
    </Button>
  );
};

const ProcessOutput = ({ run }: { run: ProcessRunView }) => {
  const [search, setSearch] = useState("");
  const [wrap, setWrap] = useState(true);
  const [follow, setFollow] = useState(run.status === "running");
  const [timestamps, setTimestamps] = useState(true);
  const scrollRef = useRef<HTMLPreElement>(null);
  const query = useQuery({
    queryKey: ["diagnostic-process-output", run.id],
    queryFn: () => readOutput(run.id),
    retry: false,
    refetchInterval: run.status === "running" ? 5_000 : false,
  });
  const visible = useMemo(() => {
    const lines = (query.data?.text ?? "").split("\n");
    return lines
      .filter((line) => !search.trim() || line.toLowerCase().includes(search.trim().toLowerCase()))
      .map((line) => (timestamps ? line : line.replace(/^\[[^\]]+\]\s?/, "")))
      .join("\n");
  }, [query.data?.text, search, timestamps]);
  // The rendered output is an intentional trigger: each live refresh should advance the tail.
  // biome-ignore lint/correctness/useExhaustiveDependencies: visible changes when filtered output changes
  useEffect(() => {
    if (follow && scrollRef.current) scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
  }, [follow, query.data?.text, search, timestamps]);

  const download = async () => {
    const output = query.data ?? (await readOutput(run.id));
    const url = URL.createObjectURL(new Blob([output.text], { type: "text/plain" }));
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `loomarr-process-${run.id}.log`;
    anchor.click();
    URL.revokeObjectURL(url);
  };

  if (query.error instanceof OutputError && query.error.status === 410)
    return (
      <p className="rounded-md border border-border p-4 text-muted-foreground text-sm">
        Output retention expired. Run metadata remains available.
      </p>
    );
  if (query.error instanceof OutputError && query.error.status === 501)
    return (
      <p className="rounded-md border border-border p-4 text-muted-foreground text-sm">
        Process output is unavailable in this runtime.
      </p>
    );
  if (query.isError) return <ErrorState error={query.error} onRetry={() => query.refetch()} />;
  if (query.isPending)
    return <p className="text-muted-foreground text-sm">Loading retained Process output…</p>;

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          className="min-w-48 flex-1"
          aria-label="Search Process output"
          placeholder="Search output"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        <Button
          variant={wrap ? "secondary" : "outline"}
          size="sm"
          aria-pressed={wrap}
          onClick={() => setWrap(!wrap)}
        >
          Wrap
        </Button>
        <Button
          variant={follow ? "secondary" : "outline"}
          size="sm"
          aria-pressed={follow}
          onClick={() => setFollow(!follow)}
        >
          Follow tail
        </Button>
        <Button
          variant={timestamps ? "secondary" : "outline"}
          size="sm"
          aria-pressed={timestamps}
          onClick={() => setTimestamps(!timestamps)}
        >
          Timestamps
        </Button>
        <Button variant="outline" size="sm" onClick={() => void download()}>
          <Download aria-hidden /> Download output
        </Button>
      </div>
      {(query.data.truncated || query.data.discarded > 0) && (
        <p className="flex items-center gap-1 text-caution text-xs">
          <AlertTriangle aria-hidden className="size-3" /> Output is truncated; {query.data.discarded} earlier
          lines were discarded.
        </p>
      )}
      <pre
        ref={scrollRef}
        className={`max-h-[28rem] overflow-auto rounded-md border border-border bg-background p-3 font-mono text-xs ${wrap ? "whitespace-pre-wrap break-words" : "whitespace-pre"}`}
      >
        {visible || "No retained output matches this search."}
      </pre>
    </div>
  );
};

const ProcessDetail = ({
  run,
  onOpenChannel,
}: {
  run: ProcessRunView;
  onOpenChannel?: (id: string) => void;
}) => {
  const query = diagnosticsApi.useGetDiagnosticProcess(run.id, {
    query: { retry: false, refetchInterval: run.status === "running" ? 5_000 : false },
  });
  const detail = query.data?.status === 200 ? query.data.data : undefined;
  const latest = detail?.progress.at(-1);
  return (
    <section
      className="space-y-4 rounded-lg border border-border bg-card p-4"
      aria-label="Selected Process run"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h3 className="font-medium">{run.purpose}</h3>
          <p className="text-muted-foreground text-xs">Started {formatTime(run.startedAt)}</p>
        </div>
        <span className="inline-flex items-center gap-2 text-sm">
          <StatusDot tone={tone(run.status)} label={run.status} />
          {run.status}
        </span>
      </div>
      <div className="grid gap-2 text-sm sm:grid-cols-2 lg:grid-cols-3">
        <div>
          <span className="text-muted-foreground text-xs">Process run</span>
          <CopyValue label="Process run" value={run.id} />
        </div>
        {run.channelId && (
          <div>
            <span className="text-muted-foreground text-xs">Channel</span>
            <div className="flex">
              <CopyValue label="Channel" value={run.channelId} />
              {onOpenChannel && (
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Open Channel"
                  onClick={() => onOpenChannel(run.channelId ?? "")}
                >
                  <ExternalLink aria-hidden />
                </Button>
              )}
            </div>
          </div>
        )}
        {run.jobId && (
          <div>
            <span className="text-muted-foreground text-xs">Job</span>
            <CopyValue label="Job" value={run.jobId} />
          </div>
        )}
        <div>
          <span className="text-muted-foreground text-xs">Executable</span>
          <p>
            {run.executable ?? "Unavailable"}
            {run.executableVersion ? ` · ${run.executableVersion}` : ""}
          </p>
        </div>
        <div>
          <span className="text-muted-foreground text-xs">Target</span>
          <p className="break-all">{run.target ?? "Unavailable"}</p>
        </div>
        <div>
          <span className="text-muted-foreground text-xs">Ended</span>
          <p>{formatTime(run.endedAt)}</p>
        </div>
      </div>
      {query.isError ? (
        <ErrorState error={query.error} onRetry={() => query.refetch()} />
      ) : latest ? (
        <div
          role="status"
          className="flex flex-wrap gap-5 rounded-md bg-muted/30 p-3 text-sm"
          aria-label="Latest Process progress"
        >
          <span>
            Frame <strong>{latest.frame}</strong>
          </span>
          <span>
            Speed <strong>{latest.speed.toFixed(2)}×</strong>
          </span>
          <span>
            Output time <strong>{(latest.outTimeMs / 1000).toFixed(1)}s</strong>
          </span>
          {detail?.progressTruncated && (
            <span className="text-caution">Earlier progress samples omitted</span>
          )}
        </div>
      ) : (
        <p className="text-muted-foreground text-sm">No progress samples retained for this run.</p>
      )}
      <ProcessOutput run={run} />
    </section>
  );
};

const PlayoutDiagnostics = ({
  filters,
  onFiltersChange,
  onOpenChannel,
}: {
  filters: DiagnosticsSearch;
  onFiltersChange: (filters: DiagnosticsSearch) => void;
  onOpenChannel?: (id: string) => void;
}) => {
  const [cursorStack, setCursorStack] = useState<string[]>([]);
  const [now, setNow] = useState(Date.now());
  const cursor = cursorStack.at(-1);
  useEffect(() => {
    if (cursor) return;
    const interval = window.setInterval(() => setNow(Date.now()), 5_000);
    return () => window.clearInterval(interval);
  }, [cursor]);
  const params = useMemo<ListDiagnosticProcessesParams>(
    () => ({
      from: now - RANGE_MS[filters.processRange],
      to: now,
      limit: 200,
      ...(cursor ? { cursor } : {}),
      ...(filters.processStatus === "all"
        ? {}
        : { status: ListDiagnosticProcessesStatus[filters.processStatus] }),
      ...(filters.processPurpose.trim() ? { purpose: filters.processPurpose.trim() } : {}),
      ...(filters.processChannelId.trim() ? { channelId: filters.processChannelId.trim() } : {}),
      ...(filters.processJobId.trim() ? { jobId: filters.processJobId.trim() } : {}),
    }),
    [cursor, filters, now],
  );
  const query = diagnosticsApi.useListDiagnosticProcesses(params, { query: { retry: false } });
  const body = query.data?.status === 200 ? query.data.data : undefined;
  const runs = body?.items ?? [];
  const selected =
    runs.find((run) => run.id === filters.processId) ?? (filters.processId ? undefined : runs[0]);
  const update = (next: DiagnosticsSearch) => {
    setCursorStack([]);
    setNow(Date.now());
    onFiltersChange(next);
  };
  return (
    <section className="space-y-4" aria-labelledby="process-runs-title">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h2 id="process-runs-title" className="font-medium text-lg">
            Process runs
          </h2>
          <p className="text-muted-foreground text-sm">
            FFmpeg and other media processes connected to the logs you are investigating.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => query.refetch()} disabled={query.isFetching}>
          <RotateCcw aria-hidden /> Refresh
        </Button>
      </div>
      <div className="grid gap-3 rounded-lg border border-border bg-card p-3 sm:grid-cols-2">
        <Select
          value={filters.processRange}
          onValueChange={(value) =>
            update({ ...filters, processRange: value as DiagnosticsSearch["processRange"] })
          }
        >
          <SelectTrigger aria-label="Process time range">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="1h">Last hour</SelectItem>
            <SelectItem value="6h">Last 6 hours</SelectItem>
            <SelectItem value="24h">Last 24 hours</SelectItem>
          </SelectContent>
        </Select>
        <Select
          value={filters.processStatus}
          onValueChange={(value) =>
            update({ ...filters, processStatus: value as DiagnosticsSearch["processStatus"] })
          }
        >
          <SelectTrigger aria-label="Process status">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            <SelectItem value="running">Running</SelectItem>
            <SelectItem value="succeeded">Succeeded</SelectItem>
            <SelectItem value="failed">Failed</SelectItem>
            <SelectItem value="cancelled">Cancelled</SelectItem>
          </SelectContent>
        </Select>
        <details className="sm:col-span-2">
          <summary className="cursor-pointer py-1 text-muted-foreground text-sm">More filters</summary>
          <div className="mt-2 grid gap-3 sm:grid-cols-3">
            <Input
              aria-label="Process purpose"
              placeholder="Purpose"
              value={filters.processPurpose}
              onChange={(event) => update({ ...filters, processPurpose: event.target.value })}
            />
            <Input
              aria-label="Process Channel id"
              placeholder="Channel id"
              value={filters.processChannelId}
              onChange={(event) => update({ ...filters, processChannelId: event.target.value })}
            />
            <Input
              aria-label="Process Job id"
              placeholder="Job id"
              value={filters.processJobId}
              onChange={(event) => update({ ...filters, processJobId: event.target.value })}
            />
          </div>
        </details>
      </div>
      {query.isError ? (
        <ErrorState error={query.error} onRetry={() => query.refetch()} />
      ) : query.isPending ? (
        <p className="rounded-lg border border-border bg-card px-4 py-8 text-center text-muted-foreground text-sm">
          Loading Process runs…
        </p>
      ) : runs.length === 0 ? (
        <p className="rounded-lg border border-border bg-card px-4 py-8 text-center text-muted-foreground text-sm">
          No Process runs match this bounded window.
        </p>
      ) : (
        <div className="grid min-h-96 gap-4 xl:grid-cols-[22rem_minmax(0,1fr)]">
          <ul
            className="max-h-[48rem] overflow-auto rounded-lg border border-border bg-card"
            aria-label="Process runs"
          >
            {runs.map((run) => (
              <li key={run.id} className="[contain-intrinsic-size:auto_76px] [content-visibility:auto]">
                <button
                  type="button"
                  className={`grid w-full gap-1 border-border border-b px-3 py-3 text-left hover:bg-muted/20 ${selected?.id === run.id ? "bg-muted/30" : ""}`}
                  aria-pressed={selected?.id === run.id}
                  onClick={() => onFiltersChange({ ...filters, processId: run.id })}
                >
                  <span className="flex items-center justify-between gap-2">
                    <strong className="truncate text-sm">{run.purpose}</strong>
                    <span className="inline-flex items-center gap-1 text-xs">
                      <StatusDot tone={tone(run.status)} label={run.status} />
                      {run.status}
                    </span>
                  </span>
                  <span className="truncate text-muted-foreground text-xs">{run.target ?? run.id}</span>
                  <time className="text-muted-foreground text-xs">{formatTime(run.startedAt)}</time>
                </button>
              </li>
            ))}
          </ul>
          {selected ? (
            <ProcessDetail run={selected} onOpenChannel={onOpenChannel} />
          ) : (
            <p className="rounded-lg border border-border p-6 text-muted-foreground text-sm">
              The selected Process run is outside this page. Adjust the filters or return to a newer page.
            </p>
          )}
        </div>
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
          Page {cursorStack.length + 1} · at most 200 runs
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

export { PlayoutDiagnostics };
