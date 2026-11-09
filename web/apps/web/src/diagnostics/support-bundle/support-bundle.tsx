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
  Field,
  LoadingStrip,
  Toggle,
} from "@loomarr/design-system";
import { Settings2, ShieldCheck } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

type Range = "1h" | "6h" | "24h";
const RANGE_MS: Record<Range, number> = { "1h": 3_600_000, "6h": 21_600_000, "24h": 86_400_000 };
const RANGE_OPTIONS: { label: string; value: Range }[] = [
  { label: "Last hour", value: "1h" },
  { label: "Last 6 hours", value: "6h" },
  { label: "Last 24 hours", value: "24h" },
];

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
  const [range, setRange] = useState<Range>(initialRange);
  const [events, setEvents] = useState(true);
  const [processes, setProcesses] = useState(true);
  const [processOutput, setProcessOutput] = useState(true);
  const [fields, setFields] = useState(correlations);
  const [preview, setPreview] = useState<BundlePreview>();
  const [busy, setBusy] = useState<"preview" | "download">();
  const [error, setError] = useState<string>();

  // The window ends when the request is made, so a dialog left open still reports recent evidence.
  const selectionNow = useCallback((): BundleSelection => {
    const to = Date.now();
    return { from: to - RANGE_MS[range], to, events, processes, processOutput, ...fields };
  }, [events, fields, processOutput, processes, range]);

  useEffect(() => {
    if (!open || (!events && !processes && !processOutput)) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      setBusy("preview");
      setError(undefined);
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
  }, [events, open, processOutput, processes, selectionNow]);

  const invalidate = () => {
    setPreview(undefined);
    setError(undefined);
  };

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

  const updateCategory = (kind: "events" | "processes" | "output", checked: boolean) => {
    if (kind === "events") setEvents(checked);
    if (kind === "processes") {
      setProcesses(checked);
      if (!checked) setProcessOutput(false);
    }
    if (kind === "output") {
      setProcessOutput(checked);
      if (checked) setProcesses(true);
    }
    invalidate();
  };

  const correlationFields = [
    ["Request id", "requestId"],
    ["Playback session", "playbackSessionId"],
    ["Channel", "channelId"],
    ["Schedule block", "scheduleBlockId"],
    ["Job", "jobId"],
    ["Process run", "processRunId"],
  ] as const;

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
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h3 className="font-medium">Recent diagnostic evidence</h3>
                  <p className="text-muted-foreground mt-1 text-sm">
                    {events ? "Application and player logs" : ""}
                    {events && processes ? " plus " : ""}
                    {processes ? "related process details" : ""}
                    {processOutput ? " and retained output" : ""}.
                  </p>
                </div>
                <CompactSelect
                  label="Troubleshooting report time window"
                  onValueChange={(value) => {
                    setRange(value as Range);
                    invalidate();
                  }}
                  options={RANGE_OPTIONS}
                  value={range}
                  width={160}
                />
              </div>
              <div className="border-success/30 bg-success/5 mt-4 flex items-start gap-2 rounded-md border p-3">
                <ShieldCheck className="text-lock mt-0.5 size-4 shrink-0" aria-hidden />
                <p className="text-muted-foreground text-xs">
                  Secrets, URL credentials, and local paths are redacted again while the report is built.
                </p>
              </div>
            </section>

            {busy === "preview" && (
              <p className="text-muted-foreground inline-flex items-center gap-2 text-sm" role="status">
                <LoadingStrip decorative width={32} /> Reviewing the selected evidence…
              </p>
            )}

            {preview && (
              <section
                className="border-border bg-card space-y-3 rounded-lg border p-4"
                aria-label="Troubleshooting report summary"
              >
                <div className="flex flex-wrap items-baseline justify-between gap-2">
                  <h3 className="font-medium">What will be included</h3>
                  <span className="font-mono text-sm">About {formatBytes(preview.estimatedBytes)}</span>
                </div>
                <dl className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
                  <div>
                    <dt className="text-muted-foreground">Logs</dt>
                    <dd className="font-medium">{counts?.events ?? 0}</dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground">Processes</dt>
                    <dd className="font-medium">{counts?.processes ?? 0}</dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground">Outputs</dt>
                    <dd className="font-medium">{counts?.processOutputs ?? 0}</dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground">Redactions</dt>
                    <dd className="font-medium">{counts?.redactions ?? 0}</dd>
                  </div>
                </dl>
                {warnings.length > 0 ? (
                  <p className="border-caution/30 bg-caution/5 rounded-md border p-3 text-sm">
                    Some evidence is limited: {warnings.join(", ")}.
                  </p>
                ) : (
                  <p className="text-muted-foreground text-xs">
                    No evidence limits were reached in this review.
                  </p>
                )}
              </section>
            )}

            <details className="border-border rounded-lg border p-4">
              <summary className="cursor-pointer text-sm font-medium">
                <Settings2 className="mr-2 inline size-4" aria-hidden /> Customize report
              </summary>
              <div className="mt-4 space-y-4">
                <fieldset>
                  <legend className="mb-2 text-sm font-medium">Evidence types</legend>
                  <div className="grid gap-2 sm:grid-cols-3">
                    {[
                      ["Application and player logs", "events", events],
                      ["Process details", "processes", processes],
                      ["Process output", "output", processOutput],
                    ].map(([label, kind, checked]) => (
                      <div key={String(kind)} className="border-border rounded-md border px-3">
                        <Toggle
                          label={String(label)}
                          checked={Boolean(checked)}
                          onCheckedChange={(next) =>
                            updateCategory(kind as "events" | "processes" | "output", next)
                          }
                        />
                      </div>
                    ))}
                  </div>
                </fieldset>
                <div>
                  <h4 className="mb-2 text-sm font-medium">Exact identifiers</h4>
                  <div className="grid gap-3 sm:grid-cols-2">
                    {correlationFields.map(([label, key]) => (
                      <Field
                        autoCapitalize="none"
                        autoCorrect={false}
                        key={key}
                        label={label}
                        maxLength={128}
                        onChangeText={(text) => {
                          setFields((current) => ({ ...current, [key]: text || undefined }));
                          invalidate();
                        }}
                        value={fields[key] ?? ""}
                      />
                    ))}
                  </div>
                </div>
                {preview && (
                  <details>
                    <summary className="text-muted-foreground cursor-pointer text-sm">Files included</summary>
                    <ul className="mt-2 grid gap-1 font-mono text-xs sm:grid-cols-2">
                      {["manifest.json", ...preview.manifest.entries.map((entry) => entry.name)].map(
                        (name) => (
                          <li key={name}>{name}</li>
                        ),
                      )}
                    </ul>
                  </details>
                )}
              </div>
            </details>
          </div>

          {error && (
            <p
              role="alert"
              className="border-destructive/40 bg-destructive/10 text-destructive rounded-md border p-3 text-sm"
            >
              {error}
            </p>
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
