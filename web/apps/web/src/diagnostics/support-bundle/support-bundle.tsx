import type { BundlePreview } from "@loomarr/api/models/bundlePreview";
import type { BundleSelection } from "@loomarr/api/models/bundleSelection";
import { Archive, ArrowLeft, Download, LoaderCircle, Settings2, ShieldCheck } from "lucide-react";
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
type Step = "review" | "download";
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
  const [step, setStep] = useState<Step>("review");
  const [range, setRange] = useState<Range>(initialRange);
  const [events, setEvents] = useState(true);
  const [processes, setProcesses] = useState(true);
  const [processOutput, setProcessOutput] = useState(true);
  const [fields, setFields] = useState(correlations);
  const [preview, setPreview] = useState<BundlePreview>();
  const [artifact, setArtifact] = useState<{ blob: Blob; filename: string }>();
  const [busy, setBusy] = useState<"preview" | "prepare">();
  const [error, setError] = useState<string>();

  const selection = useMemo<BundleSelection>(() => {
    const to = Date.now();
    return { from: to - RANGE_MS[range], to, events, processes, processOutput, ...fields };
  }, [events, fields, processOutput, processes, range]);

  const selectionKey = JSON.stringify(selection);
  useEffect(() => {
    if (!open || step !== "review" || (!events && !processes && !processOutput)) return;
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
          setError(reason instanceof Error ? reason.message : "The bundle could not be reviewed.");
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
  }, [events, open, processOutput, processes, selectionKey, step]);

  const invalidate = () => {
    setStep("review");
    setPreview(undefined);
    setArtifact(undefined);
    setError(undefined);
  };

  const prepare = async () => {
    setBusy("prepare");
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
      const filename = disposition.match(/filename="([^"]+)"/)?.[1] ?? "loomarr-support.zip";
      setArtifact({ blob, filename });
      setStep("download");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "The support bundle could not be prepared.");
    } finally {
      setBusy(undefined);
    }
  };

  const download = () => {
    if (!artifact) return;
    const url = URL.createObjectURL(artifact.blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = artifact.filename;
    link.click();
    URL.revokeObjectURL(url);
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
  const warnings = preview?.manifest.truncationReasons ?? [];

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (next) {
          setStep("review");
          setArtifact(undefined);
          setPreview(undefined);
          setError(undefined);
        }
      }}
    >
      <DialogTrigger render={<Button variant="outline" />}>
        <Archive aria-hidden /> Create support bundle
      </DialogTrigger>
      <DialogContent className="max-h-[92vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <div className="mb-1 flex gap-2 text-xs">
            <span className={step === "review" ? "font-medium text-foreground" : "text-muted-foreground"}>
              1 · Review
            </span>
            <span aria-hidden className="text-muted-foreground">
              →
            </span>
            <span className={step === "download" ? "font-medium text-foreground" : "text-muted-foreground"}>
              2 · Download
            </span>
          </div>
          <DialogTitle>{step === "review" ? "Review support bundle" : "Download support bundle"}</DialogTitle>
          <DialogDescription>
            {step === "review"
              ? "Check the evidence Loomarr will package. Nothing is submitted or sent anywhere."
              : "This is the exact reviewed bundle. Save it when you are ready."}
          </DialogDescription>
        </DialogHeader>

        {step === "review" ? (
          <div className="space-y-4">
            <section className="rounded-lg border border-border bg-muted/20 p-4" aria-label="Bundle scope">
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
                  <SelectTrigger className="w-40" aria-label="Support bundle time window">
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
                  Secrets, URL credentials, and local paths are redacted again while the bundle is built.
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
                aria-label="Support bundle review"
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
                <Settings2 className="mr-2 inline size-4" aria-hidden /> Advanced options
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
        ) : (
          <section
            className="space-y-4 rounded-lg border border-border bg-card p-5"
            aria-label="Prepared support bundle"
          >
            <div className="flex items-center gap-3">
              <span className="flex size-10 items-center justify-center rounded-full bg-success/10 text-success">
                <ShieldCheck aria-hidden />
              </span>
              <div>
                <h3 className="font-medium">Bundle ready</h3>
                <p className="text-muted-foreground text-sm">
                  Final size {formatBytes(artifact?.blob.size ?? 0)}
                </p>
              </div>
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
            {warnings.length > 0 && (
              <p className="rounded-md border border-caution/30 bg-caution/5 p-3 text-sm">
                Included with declared limits: {warnings.join(", ")}.
              </p>
            )}
          </section>
        )}

        {error && (
          <p
            role="alert"
            className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-destructive text-sm"
          >
            {error}
          </p>
        )}

        <DialogFooter className="flex-col-reverse sm:flex-row">
          {step === "download" && (
            <Button variant="outline" onClick={() => setStep("review")}>
              <ArrowLeft aria-hidden /> Back to review
            </Button>
          )}
          {step === "review" ? (
            <Button
              onClick={() => void prepare()}
              disabled={!preview || busy !== undefined || (!events && !processes && !processOutput)}
            >
              {busy === "prepare" ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <Archive aria-hidden />
              )}
              Prepare download
            </Button>
          ) : (
            <Button onClick={download} disabled={!artifact}>
              <Download aria-hidden /> Download bundle
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export { formatBytes, SupportBundle };
