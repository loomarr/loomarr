import {
  getDownloadDiagnosticSupportBundleUrl,
  previewDiagnosticSupportBundle,
} from "@loomarr/api/endpoints/diagnostics";
import type { BundlePreview } from "@loomarr/api/models/bundlePreview";
import type { BundleSelection } from "@loomarr/api/models/bundleSelection";
import {
  Action,
  CompactSelect,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Icon,
  icons,
  LoadingStrip,
  Stack,
  Text,
  Toggle,
} from "@loomarr/design-system";
import { useStore } from "@tanstack/react-form";
import { useEffect, useState } from "react";
import { useAppForm } from "@/lib/app-form";

type Range = "1h" | "6h" | "24h";
const RANGE_MS: Record<Range, number> = { "1h": 3_600_000, "6h": 21_600_000, "24h": 86_400_000 };
const RANGE_OPTIONS: { label: string; value: Range }[] = [
  { label: "Last hour", value: "1h" },
  { label: "Last 6 hours", value: "6h" },
  { label: "Last 24 hours", value: "24h" },
];

const EVIDENCE_TYPES = [
  ["Application and player logs", "events"],
  ["Process details", "processes"],
  ["Process output", "processOutput"],
] as const;

const CORRELATION_FIELDS = [
  ["Request id", "requestId"],
  ["Playback session", "playbackSessionId"],
  ["Channel", "channelId"],
  ["Schedule block", "scheduleBlockId"],
  ["Job", "jobId"],
  ["Process run", "processRunId"],
] as const;

const formatBytes = (bytes: number) => {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
};

const problemDetail = async (response: Response) => {
  try {
    const problem = (await response.json()) as { detail?: string; title?: string };
    return problem.detail ?? problem.title ?? `Request failed (${response.status})`;
  } catch {
    return `Request failed (${response.status})`;
  }
};

const truncationLabel = (reason: string) => {
  if (reason === "processes_limit") return "only the newest process runs fit";
  if (reason === "process_output_retention") return "some earlier process output has expired";
  if (reason === "process_output_limit") return "only the newest process output files fit";
  return "some diagnostic evidence was limited";
};

const SupportBundle = ({
  initialRange = "1h",
  correlations = {},
}: {
  initialRange?: Range;
  correlations?: Partial<
    Pick<
      BundleSelection,
      "requestId" | "playbackSessionId" | "channelId" | "scheduleBlockId" | "jobId" | "processRunId"
    >
  >;
}) => {
  const [open, setOpen] = useState(false);
  const [preview, setPreview] = useState<BundlePreview>();
  const [busy, setBusy] = useState<"preview" | "download">();
  const [error, setError] = useState<string>();

  const form = useAppForm({
    defaultValues: {
      range: initialRange,
      events: true,
      processes: true,
      processOutput: true,
      requestId: correlations.requestId ?? "",
      playbackSessionId: correlations.playbackSessionId ?? "",
      channelId: correlations.channelId ?? "",
      scheduleBlockId: correlations.scheduleBlockId ?? "",
      jobId: correlations.jobId ?? "",
      processRunId: correlations.processRunId ?? "",
    },
  });
  const values = useStore(form.store, (state) => state.values);
  const { events, processes, processOutput } = values;

  // The window ends when the request is made, so a dialog left open still reports recent evidence.
  const selectionNow = (): BundleSelection => {
    const to = Date.now();
    const { range, ...rest } = form.state.values;
    return {
      from: to - RANGE_MS[range],
      to,
      ...rest,
      ...Object.fromEntries(CORRELATION_FIELDS.map(([, key]) => [key, rest[key] || undefined])),
    };
  };

  // Any change to the selection retires the review it was made against.
  // biome-ignore lint/correctness/useExhaustiveDependencies: `values` is the trigger, not an input
  useEffect(() => {
    setPreview(undefined);
    setError(undefined);
    if (!open || (!events && !processes && !processOutput)) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      setBusy("preview");
      void previewDiagnosticSupportBundle(selectionNow(), { signal: controller.signal })
        .then((response) => {
          if (response.status === 200) setPreview(response.data);
        })
        .catch((reason: unknown) => {
          if (reason instanceof DOMException && reason.name === "AbortError") return;
          setError(reason instanceof Error ? reason.message : "The report could not be reviewed.");
          setPreview(undefined);
        })
        .finally(() => {
          if (!controller.signal.aborted) setBusy(undefined);
        });
    }, 200);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [open, values]);

  const download = async () => {
    setBusy("download");
    setError(undefined);
    try {
      // A ZIP body cannot go through the shared JSON mutator, so the download reads the response itself.
      const response = await fetch(getDownloadDiagnosticSupportBundleUrl(), {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(selectionNow()),
      });
      if (!response.ok) throw new Error(await problemDetail(response));
      const blob = await response.blob();
      const disposition = response.headers.get("Content-Disposition") ?? "";
      const filename = disposition.match(/filename="([^"]+)"/)?.[1] ?? "loomarr-troubleshooting.zip";
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = filename;
      link.click();
      URL.revokeObjectURL(url);
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "The troubleshooting report could not be downloaded.",
      );
    } finally {
      setBusy(undefined);
    }
  };

  // Process output is the output of a process, so it never outlives its process details.
  const updateCategory = (kind: "events" | "processes" | "processOutput", checked: boolean) => {
    form.setFieldValue(kind, checked);
    if (kind === "processes" && !checked) form.setFieldValue("processOutput", false);
    if (kind === "processOutput" && checked) form.setFieldValue("processes", true);
  };

  const counts = preview?.manifest.counts;
  const warnings = [...new Set((preview?.manifest.truncationReasons ?? []).map(truncationLabel))];

  const changeOpen = (next: boolean) => {
    setOpen(next);
    if (next) {
      setPreview(undefined);
      setError(undefined);
    }
  };

  return (
    <>
      {/* Opened from onPress rather than DialogTrigger: on web the trigger drives its child by
          onClick, which an Action does not take. */}
      <Action icon="download" onPress={() => changeOpen(true)} variant="outline">
        Download troubleshooting report
      </Action>
      <Dialog onOpenChange={changeOpen} open={open}>
        <DialogContent size="form">
          <DialogHeader>
            <DialogTitle>Download troubleshooting report</DialogTitle>
            <DialogDescription>
              Save redacted logs and process details as a ZIP to use when troubleshooting. Nothing is
              submitted or sent anywhere.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <section className="border-border bg-muted/20 rounded-lg border p-4" aria-label="Report scope">
              <Stack align="start" direction="row" gap="control" justify="between" wrap>
                <div>
                  <Text as="h3" textRole="heading">
                    Recent diagnostic evidence
                  </Text>
                  <Text as="p" className="mt-1" textRole="compact" tone="secondary">
                    {events ? "Application and player logs" : ""}
                    {events && processes ? " plus " : ""}
                    {processes ? "related process details" : ""}
                    {processOutput ? " and retained output" : ""}.
                  </Text>
                </div>
                <form.Field name="range">
                  {(field) => (
                    <CompactSelect
                      label="Troubleshooting report time window"
                      onValueChange={(value) => field.handleChange(value as Range)}
                      options={RANGE_OPTIONS}
                      value={field.state.value}
                      width={160}
                    />
                  )}
                </form.Field>
              </Stack>
              <Stack
                align="start"
                className="border-success/30 bg-success/5 mt-4 rounded-md border p-3"
                direction="row"
                gap="inline"
              >
                <Stack className="mt-0.5" direction="row" render="span">
                  <Icon decorative glyph={icons.verified} size="compact" tone="success" />
                </Stack>
                <Text as="p" textRole="hint" tone="secondary">
                  Secrets, URL credentials, and local paths are redacted again while the report is built.
                </Text>
              </Stack>
            </section>

            {busy === "preview" && (
              <div role="status">
                <Text as="p" className="inline-flex items-center gap-2" textRole="compact" tone="secondary">
                  <LoadingStrip decorative width={32} /> Reviewing the selected evidence…
                </Text>
              </div>
            )}

            {preview && (
              <section
                className="border-border bg-card space-y-3 rounded-lg border p-4"
                aria-label="Troubleshooting report summary"
              >
                <Stack align="baseline" direction="row" gap="inline" justify="between" wrap>
                  <Text as="h3" textRole="heading">
                    What will be included
                  </Text>
                  <span className="font-mono text-sm">About {formatBytes(preview.estimatedBytes)}</span>
                </Stack>
                <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                  <div>
                    <Text as="dt" textRole="compact" tone="secondary">
                      Logs
                    </Text>
                    <Text as="dd" textRole="cardLabel">
                      {counts?.events ?? 0}
                    </Text>
                  </div>
                  <div>
                    <Text as="dt" textRole="compact" tone="secondary">
                      Processes
                    </Text>
                    <Text as="dd" textRole="cardLabel">
                      {counts?.processes ?? 0}
                    </Text>
                  </div>
                  <div>
                    <Text as="dt" textRole="compact" tone="secondary">
                      Outputs
                    </Text>
                    <Text as="dd" textRole="cardLabel">
                      {counts?.processOutputs ?? 0}
                    </Text>
                  </div>
                  <div>
                    <Text as="dt" textRole="compact" tone="secondary">
                      Redactions
                    </Text>
                    <Text as="dd" textRole="cardLabel">
                      {counts?.redactions ?? 0}
                    </Text>
                  </div>
                </dl>
                {warnings.length > 0 ? (
                  <Text
                    as="p"
                    className="border-caution/30 bg-caution/5 rounded-md border p-3"
                    textRole="compact"
                  >
                    Some evidence is limited: {warnings.join(", ")}.
                  </Text>
                ) : (
                  <Text as="p" textRole="hint" tone="secondary">
                    No evidence limits were reached in this review.
                  </Text>
                )}
              </section>
            )}

            <details className="border-border rounded-lg border p-4">
              <Text as="summary" className="cursor-pointer" textRole="cardLabel">
                <span className="mr-2 inline-flex align-middle">
                  <Icon decorative glyph={icons.settingsAdjust} size="compact" tone="inherit" />
                </span>{" "}
                Customize report
              </Text>
              <div className="mt-4 space-y-4">
                <fieldset>
                  <Text as="legend" className="mb-2" textRole="cardLabel">
                    Evidence types
                  </Text>
                  <div className="grid gap-2 sm:grid-cols-3">
                    {EVIDENCE_TYPES.map(([label, kind]) => (
                      <div key={kind} className="border-border rounded-md border px-3">
                        <Toggle
                          label={label}
                          checked={values[kind]}
                          onCheckedChange={(next) => updateCategory(kind, next)}
                        />
                      </div>
                    ))}
                  </div>
                </fieldset>
                <div>
                  <Text as="h4" className="mb-2" textRole="cardLabel">
                    Exact identifiers
                  </Text>
                  <div className="grid gap-3 sm:grid-cols-2">
                    {CORRELATION_FIELDS.map(([label, key]) => (
                      <form.AppField key={key} name={key}>
                        {(field) => <field.TextField label={label} maxLength={128} />}
                      </form.AppField>
                    ))}
                  </div>
                </div>
                {preview && (
                  <details>
                    <Text as="summary" className="cursor-pointer" textRole="compact" tone="secondary">
                      Files included
                    </Text>
                    <Text as="ul" className="mt-2 grid gap-1 sm:grid-cols-2" textRole="mono" tone="primary">
                      {["manifest.json", ...preview.manifest.entries.map((entry) => entry.name)].map(
                        (name) => (
                          <li key={name}>{name}</li>
                        ),
                      )}
                    </Text>
                  </details>
                )}
              </div>
            </details>
          </div>

          {error && (
            <div role="alert" className="border-destructive/40 bg-destructive/10 rounded-md border p-3">
              <Text as="p" textRole="compact" tone="danger">
                {error}
              </Text>
            </div>
          )}

          <DialogFooter>
            <Action
              disabled={!preview || busy !== undefined || (!events && !processes && !processOutput)}
              icon={busy === "download" ? "loading" : "download"}
              onPress={() => void download()}
            >
              {busy === "download" ? "Preparing report…" : "Download report"}
            </Action>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
};

export { SupportBundle };
