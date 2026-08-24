import type { BundlePreview } from "@loomarr/api/models/bundlePreview";
import type { BundleSelection } from "@loomarr/api/models/bundleSelection";
import { Download, LoaderCircle, Settings2, ShieldCheck } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

type Range = "1h" | "6h" | "24h";
const RANGE_MS: Record<Range, number> = { "1h": 3_600_000, "6h": 21_600_000, "24h": 86_400_000 };

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

  const selection = useMemo<BundleSelection>(() => {
    const to = Date.now();
    return { from: to - RANGE_MS[range], to, events, processes, processOutput, ...fields };
  }, [events, fields, processOutput, processes, range]);

  const selectionKey = JSON.stringify(selection);
  useEffect(() => {
    if (!open || (!events && !processes && !processOutput)) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      setBusy("preview");
      setError(undefined);
      void fetch("/v1/diagnostics/support-bundle/preview", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: selectionKey,
        signal: controller.signal,
      })
        .then(async (response) => {
          if (!response.ok) throw new Error(await problemDetail(response));
          setPreview((await response.json()) as BundlePreview);
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
  }, [events, open, processOutput, processes, selectionKey]);

  const invalidate = () => {
    setPreview(undefined);
    setError(undefined);
  };

  const download = async () => {
    setBusy("download");
    setError(undefined);
    try {
      const response = await fetch("/v1/diagnostics/support-bundle", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(selection),
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

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (next) {
          setPreview(undefined);
          setError(undefined);
        }
      }}
    >
      <DialogTrigger render={<Button variant="outline" />}>
        <Download aria-hidden /> Download troubleshooting report
      </DialogTrigger>
      <DialogContent className="max-h-[92vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Download troubleshooting report</DialogTitle>
          <DialogDescription>
            Save redacted logs and process details as a ZIP to use when troubleshooting. Nothing is submitted
            or sent anywhere.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <section className="rounded-lg border border-border bg-muted/20 p-4" aria-label="Report scope">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h3 className="font-medium">Recent diagnostic evidence</h3>
                <p className="mt-1 text-muted-foreground text-sm">
                  {events ? "Application and player logs" : ""}
                  {events && processes ? " plus " : ""}
                  {processes ? "related process details" : ""}
                  {processOutput ? " and retained output" : ""}.
                </p>
              </div>
              <Select
                value={range}
                onValueChange={(value) => {
                  setRange(value as Range);
                  invalidate();
                }}
              >
                <SelectTrigger className="w-40" aria-label="Troubleshooting report time window">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="1h">Last hour</SelectItem>
                  <SelectItem value="6h">Last 6 hours</SelectItem>
                  <SelectItem value="24h">Last 24 hours</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="mt-4 flex items-start gap-2 rounded-md border border-success/30 bg-success/5 p-3">
              <ShieldCheck className="mt-0.5 size-4 shrink-0 text-success" aria-hidden />
              <p className="text-muted-foreground text-xs">
                Secrets, URL credentials, and local paths are redacted again while the report is built.
              </p>
            </div>
          </section>

          {busy === "preview" && (
            <p className="inline-flex items-center gap-2 text-muted-foreground text-sm" role="status">
              <LoaderCircle className="size-4 animate-spin" aria-hidden /> Reviewing the selected evidence…
            </p>
          )}

          {preview && (
            <section
              className="space-y-3 rounded-lg border border-border bg-card p-4"
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
                <p className="rounded-md border border-caution/30 bg-caution/5 p-3 text-sm">
                  Some evidence is limited: {warnings.join(", ")}.
                </p>
              ) : (
                <p className="text-muted-foreground text-xs">
                  No evidence limits were reached in this review.
                </p>
              )}
            </section>
          )}

          <details className="rounded-lg border border-border p-4">
            <summary className="cursor-pointer font-medium text-sm">
              <Settings2 className="mr-2 inline size-4" aria-hidden /> Customize report
            </summary>
            <div className="mt-4 space-y-4">
              <fieldset>
                <legend className="mb-2 font-medium text-sm">Evidence types</legend>
                <div className="grid gap-2 sm:grid-cols-3">
                  {[
                    ["Application and player logs", "events", events],
                    ["Process details", "processes", processes],
                    ["Process output", "output", processOutput],
                  ].map(([label, kind, checked]) => (
                    <div
                      key={String(kind)}
                      className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm"
                    >
                      <Checkbox
                        aria-label={String(label)}
                        checked={Boolean(checked)}
                        onChange={(event) =>
                          updateCategory(
                            kind as "events" | "processes" | "output",
                            event.currentTarget.checked,
                          )
                        }
                      />
                      {label}
                    </div>
                  ))}
                </div>
              </fieldset>
              <div>
                <h4 className="mb-2 font-medium text-sm">Exact identifiers</h4>
                <div className="grid gap-3 sm:grid-cols-2">
                  {correlationFields.map(([label, key]) => (
                    <div key={key} className="space-y-1 text-sm">
                      <span>{label}</span>
                      <Input
                        aria-label={label}
                        value={fields[key] ?? ""}
                        maxLength={128}
                        onChange={(event) => {
                          setFields((current) => ({ ...current, [key]: event.target.value || undefined }));
                          invalidate();
                        }}
                      />
                    </div>
                  ))}
                </div>
              </div>
              {preview && (
                <details>
                  <summary className="cursor-pointer text-muted-foreground text-sm">Files included</summary>
                  <ul className="mt-2 grid gap-1 font-mono text-xs sm:grid-cols-2">
                    {["manifest.json", ...preview.manifest.entries.map((entry) => entry.name)].map((name) => (
                      <li key={name}>{name}</li>
                    ))}
                  </ul>
                </details>
              )}
            </div>
          </details>
        </div>

        {error && (
          <p
            role="alert"
            className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-destructive text-sm"
          >
            {error}
          </p>
        )}

        <DialogFooter className="flex-col-reverse sm:flex-row">
          <Button
            onClick={() => void download()}
            disabled={!preview || busy !== undefined || (!events && !processes && !processOutput)}
          >
            {busy === "download" ? (
              <LoaderCircle className="animate-spin" aria-hidden />
            ) : (
              <Download aria-hidden />
            )}
            {busy === "download" ? "Preparing report…" : "Download report"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export { formatBytes, SupportBundle };
