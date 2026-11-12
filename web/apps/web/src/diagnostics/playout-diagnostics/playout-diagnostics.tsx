import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import type { ListDiagnosticProcessesParams } from "@loomarr/api/models/listDiagnosticProcessesParams";
import { ListDiagnosticProcessesStatus } from "@loomarr/api/models/listDiagnosticProcessesStatus";
import type { ProcessRunView } from "@loomarr/api/models/processRunView";
import {
  Action,
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
import { useEffect, useMemo, useRef, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { CopyValue } from "../copy-value";
import type { DiagnosticsSearch } from "../diagnostics-page";
import { DiagnosticsPager } from "../diagnostics-pager";
import { DiagnosticsSplitPane } from "../diagnostics-split-pane";

const RANGE_MS = { "1h": 3_600_000, "6h": 21_600_000, "24h": 86_400_000 } as const;
const VIRTUAL_VIEWPORT = { width: 352, height: 640 };

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
      <Text as="p" className="border-border rounded-md border p-4" textRole="compact" tone="secondary">
        Output retention expired. Run metadata remains available.
      </Text>
    );
  if (query.error instanceof OutputError && query.error.status === 501)
    return (
      <Text as="p" className="border-border rounded-md border p-4" textRole="compact" tone="secondary">
        Process output is unavailable in this runtime.
      </Text>
    );
  if (query.isError) return <ErrorState error={query.error} onRetry={() => query.refetch()} />;
  if (query.isPending)
    return (
      <Text as="p" textRole="compact" tone="secondary">
        Loading retained Process output…
      </Text>
    );

  return (
    <div className="space-y-2">
      <Stack align="center" direction="row" gap="inline" wrap>
        <Surface backgroundColor="transparent" borderWidth={0} flexBasis={192} flexGrow={1} minWidth={0}>
          <SearchField
            label="Search Process output"
            labelHidden
            onChangeText={setSearch}
            placeholder="Search output"
            value={search}
          />
        </Surface>
        <Action aria-pressed={wrap} onPress={() => setWrap(!wrap)} selected={wrap} tone="secondary">
          Wrap
        </Action>
        <Tooltip content="Keep the newest output line in view">
          <Action aria-pressed={follow} onPress={() => setFollow(!follow)} selected={follow} tone="secondary">
            Follow tail
          </Action>
        </Tooltip>
        <Action
          aria-pressed={timestamps}
          onPress={() => setTimestamps(!timestamps)}
          selected={timestamps}
          tone="secondary"
        >
          Timestamps
        </Action>
        <Action icon="download" onPress={() => void download()} variant="outline">
          Download output
        </Action>
      </Stack>
      {(query.data.truncated || query.data.discarded > 0) && (
        <Stack align="center" direction="row" gap="1" render="p">
          <Icon decorative glyph={icons.alert} size="compact" tone="warning" />
          <Text as="span" textRole="hint" tone="warning">
            Output is truncated; {query.data.discarded} earlier lines were discarded.
          </Text>
        </Stack>
      )}
      <pre
        ref={scrollRef}
        onScroll={(event) => {
          const output = event.currentTarget;
          if (follow && output.scrollHeight - output.scrollTop - output.clientHeight > 24) setFollow(false);
        }}
        className={`border-border bg-background max-h-[28rem] overflow-auto rounded-md border p-3 ${wrap ? "break-words whitespace-pre-wrap" : "whitespace-pre"}`}
      >
        <Text as="span" textRole="mono" tone="primary">
          {visible || "No retained output matches this search."}
        </Text>
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
      className="border-border bg-card space-y-4 rounded-lg border p-4"
      aria-label="Selected media process"
    >
      <Stack align="start" direction="row" gap="inline" justify="between" wrap>
        <div>
          <Text as="h3" textRole="heading">
            {run.purpose}
          </Text>
          <Text as="p" textRole="hint" tone="secondary">
            Started {formatTime(run.startedAt)}
          </Text>
        </div>
        <Text as="span" className="inline-flex items-center gap-2" textRole="compact">
          <StatusDot tone={tone(run.status)} label={run.status} />
          {run.status}
        </Text>
      </Stack>
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        <div>
          <Text as="span" textRole="hint" tone="secondary">
            Run ID
          </Text>
          <CopyValue label="Run ID" value={run.id} />
        </div>
        {run.channelId && (
          <div>
            <Text as="span" textRole="hint" tone="secondary">
              Channel
            </Text>
            <Stack direction="row">
              <CopyValue label="Channel" value={run.channelId} />
              {onOpenChannel && (
                <Action
                  icon="external"
                  layout="icon"
                  onPress={() => onOpenChannel(run.channelId ?? "")}
                  variant="ghost"
                >
                  Open Channel
                </Action>
              )}
            </Stack>
          </div>
        )}
        {run.jobId && (
          <div>
            <Text as="span" textRole="hint" tone="secondary">
              Job
            </Text>
            <CopyValue label="Job" value={run.jobId} />
          </div>
        )}
        <div>
          <Text as="span" textRole="hint" tone="secondary">
            Executable
          </Text>
          <Text as="p" textRole="compact">
            {run.executable ?? "Unavailable"}
            {run.executableVersion ? ` · ${run.executableVersion}` : ""}
          </Text>
        </div>
        <div>
          <Text as="span" textRole="hint" tone="secondary">
            Target
          </Text>
          <Text as="p" className="break-all" textRole="compact">
            {run.target ?? "Unavailable"}
          </Text>
        </div>
        <div>
          <Text as="span" textRole="hint" tone="secondary">
            Ended
          </Text>
          <Text as="p" textRole="compact">
            {formatTime(run.endedAt)}
          </Text>
        </div>
      </div>
      {query.isError ? (
        <ErrorState error={query.error} onRetry={() => query.refetch()} />
      ) : latest ? (
        <Stack
          aria-label="Latest Process progress"
          className="bg-muted/30 rounded-md p-3"
          direction="row"
          gap="5"
          role="status"
          wrap
        >
          <Text as="span" textRole="compact">
            Frame <strong>{latest.frame}</strong>
          </Text>
          <Text as="span" textRole="compact">
            Speed <strong>{latest.speed.toFixed(2)}×</strong>
          </Text>
          <Text as="span" textRole="compact">
            Output time <strong>{(latest.outTimeMs / 1000).toFixed(1)}s</strong>
          </Text>
          {detail?.progressTruncated && (
            <Text as="span" textRole="compact" tone="warning">
              Earlier progress samples omitted
            </Text>
          )}
        </Stack>
      ) : (
        <Text as="p" textRole="compact" tone="secondary">
          No progress samples retained for this run.
        </Text>
      )}
      <ProcessOutput key={run.id} run={run} />
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
  const listRef = useRef<HTMLDivElement>(null);
  const cursor = cursorStack.at(-1);
  const processParams = (now = Date.now()): ListDiagnosticProcessesParams => ({
    from: now - RANGE_MS[filters.processRange],
    to: now,
    limit: 50,
    ...(cursor ? { cursor } : {}),
    ...(filters.processStatus === "all"
      ? {}
      : { status: ListDiagnosticProcessesStatus[filters.processStatus] }),
    ...(filters.processPurpose.trim() ? { purpose: filters.processPurpose.trim() } : {}),
    ...(filters.processChannelId.trim() ? { channelId: filters.processChannelId.trim() } : {}),
    ...(filters.processJobId.trim() ? { jobId: filters.processJobId.trim() } : {}),
  });
  const query = useQuery({
    queryKey: [
      "diagnostic-process-page",
      filters.processRange,
      filters.processStatus,
      filters.processPurpose,
      filters.processChannelId,
      filters.processJobId,
      cursor,
    ],
    queryFn: ({ signal }) => diagnosticsApi.listDiagnosticProcesses(processParams(), { signal }),
    retry: false,
    refetchInterval: cursor ? false : 5_000,
  });
  const body = query.data?.status === 200 ? query.data.data : undefined;
  const runs = body?.items ?? [];
  const selected =
    runs.find((run) => run.id === filters.processId) ?? (filters.processId ? undefined : runs[0]);
  const virtualizer = useVirtualizer({
    count: runs.length,
    getScrollElement: () => listRef.current,
    getItemKey: (index) => runs[index]?.id ?? index,
    estimateSize: () => 76,
    overscan: 6,
    initialRect: VIRTUAL_VIEWPORT,
    observeElementRect: observeVirtualRect,
  });
  const update = (next: DiagnosticsSearch) => {
    setCursorStack([]);
    onFiltersChange(next);
  };
  return (
    <section className="space-y-4" aria-labelledby="media-processes-title">
      <Stack align="start" direction="row" gap="inline" justify="between" wrap>
        <div>
          <Text as="h2" id="media-processes-title" textRole="headline">
            Media processes
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            FFmpeg and streaming activity connected to the logs you are investigating.
          </Text>
        </div>
      </Stack>
      <div className="border-border bg-card grid gap-3 rounded-lg border p-3 sm:grid-cols-2">
        <CompactSelect<DiagnosticsSearch["processRange"]>
          label="Process time range"
          onValueChange={(processRange) => update({ ...filters, processRange })}
          options={[
            { value: "1h", label: "Last hour" },
            { value: "6h", label: "Last 6 hours" },
            { value: "24h", label: "Last 24 hours" },
          ]}
          value={filters.processRange}
        />
        <CompactSelect<DiagnosticsSearch["processStatus"]>
          label="Process status"
          onValueChange={(processStatus) => update({ ...filters, processStatus })}
          options={[
            { value: "all", label: "All statuses" },
            { value: "running", label: "Running" },
            { value: "succeeded", label: "Succeeded" },
            { value: "failed", label: "Failed" },
            { value: "cancelled", label: "Cancelled" },
            { value: "interrupted", label: "Interrupted" },
          ]}
          value={filters.processStatus}
        />
        <details className="sm:col-span-2">
          <Text as="summary" className="cursor-pointer py-1" textRole="compact" tone="secondary">
            More filters
          </Text>
          <div className="mt-2 grid gap-3 sm:grid-cols-3">
            <Field
              accessibilityLabel="Process purpose"
              autoCapitalize="none"
              autoCorrect={false}
              onChangeText={(processPurpose) => update({ ...filters, processPurpose })}
              placeholder="Purpose"
              value={filters.processPurpose}
            />
            <Field
              accessibilityLabel="Process Channel id"
              autoCapitalize="none"
              autoCorrect={false}
              onChangeText={(processChannelId) => update({ ...filters, processChannelId })}
              placeholder="Channel id"
              value={filters.processChannelId}
            />
            <Field
              accessibilityLabel="Process Job id"
              autoCapitalize="none"
              autoCorrect={false}
              onChangeText={(processJobId) => update({ ...filters, processJobId })}
              placeholder="Job id"
              value={filters.processJobId}
            />
          </div>
        </details>
      </div>
      <DiagnosticsPager
        label="Media process pages"
        page={cursorStack.length + 1}
        itemLabel="processes"
        canGoNewer={cursorStack.length > 0}
        canGoOlder={Boolean(body?.nextCursor)}
        onNewer={() => setCursorStack((stack) => stack.slice(0, -1))}
        onOlder={() => body?.nextCursor && setCursorStack((stack) => [...stack, body.nextCursor ?? ""])}
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
          Loading media processes…
        </Text>
      ) : runs.length === 0 ? (
        <Text
          as="p"
          className="border-border bg-card rounded-lg border px-4 py-8 text-center"
          textRole="compact"
          tone="secondary"
        >
          No media processes match this bounded window.
        </Text>
      ) : (
        <DiagnosticsSplitPane
          storageKey="diagnostics-media-process-details"
          revealKey={filters.processId}
          breakpoint="xl"
          secondaryOnMobile
          className="min-h-96"
          primary={
            <div
              ref={listRef}
              className="border-border bg-card max-h-[55vh] overflow-x-hidden overflow-y-auto rounded-lg border xl:max-h-[40rem]"
              style={{ height: `${Math.min(Math.max(virtualizer.getTotalSize(), 152), 640)}px` }}
            >
              <ul
                className="relative w-full"
                style={{ height: `${virtualizer.getTotalSize()}px` }}
                aria-label="Media processes"
              >
                {virtualizer.getVirtualItems().map((virtualRow) => {
                  const run = runs[virtualRow.index];
                  if (!run) return null;
                  return (
                    <li
                      key={run.id}
                      data-index={virtualRow.index}
                      className="absolute top-0 left-0 w-full"
                      style={{
                        height: `${virtualRow.size}px`,
                        transform: `translateY(${virtualRow.start}px)`,
                      }}
                    >
                      <PressableRow
                        aria-pressed={selected?.id === run.id}
                        divided
                        onPress={() => onFiltersChange({ ...filters, processId: run.id })}
                        selected={selected?.id === run.id}
                      >
                        <div className="grid w-full gap-1 text-left">
                          <Stack align="center" direction="row" gap="inline" justify="between" render="span">
                            <Text as="strong" className="truncate" strong textRole="cardLabel" tone="primary">
                              {run.purpose}
                            </Text>
                            <Text
                              as="span"
                              className="inline-flex items-center gap-1"
                              textRole="hint"
                              tone="primary"
                            >
                              <StatusDot tone={tone(run.status)} label={run.status} />
                              {run.status}
                            </Text>
                          </Stack>
                          <Text as="span" className="truncate" textRole="hint" tone="secondary">
                            {run.target ?? run.id}
                          </Text>
                          <Text as="time" textRole="hint" tone="secondary">
                            {formatTime(run.startedAt)}
                          </Text>
                        </div>
                      </PressableRow>
                    </li>
                  );
                })}
              </ul>
            </div>
          }
          secondary={
            selected ? (
              <ProcessDetail run={selected} onOpenChannel={onOpenChannel} />
            ) : (
              <Text
                as="p"
                className="border-border rounded-lg border p-6"
                textRole="compact"
                tone="secondary"
              >
                The selected media process is outside this page. Adjust the filters or return to a newer page.
              </Text>
            )
          }
        />
      )}
    </section>
  );
};

export { PlayoutDiagnostics };
