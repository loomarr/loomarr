import type { BundlePreview } from "@loomarr/api/models/bundlePreview";
import type { BundleSelection } from "@loomarr/api/models/bundleSelection";
import { Archive, Download, Eye, LoaderCircle, ShieldCheck } from "lucide-react";
import { useMemo, useState } from "react";
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
  const [finalBytes, setFinalBytes] = useState<number>();
  const [artifact, setArtifact] = useState<{ blob: Blob; filename: string }>();
  const [busy, setBusy] = useState<"preview" | "download">();
  const [error, setError] = useState<string>();

  const selection = useMemo<BundleSelection>(() => {
    const to = Date.now();
    return { from: to - RANGE_MS[range], to, events, processes, processOutput, ...fields };
  }, [events, fields, processOutput, processes, range]);

  const resetResult = () => {
    setPreview(undefined);
    setFinalBytes(undefined);
    setArtifact(undefined);
    setError(undefined);
  };

  const runPreview = async () => {
    setBusy("preview");
    setError(undefined);
    setFinalBytes(undefined);
    setArtifact(undefined);
    try {
      const response = await fetch("/v1/diagnostics/support-bundle/preview", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(selection),
      });
      if (!response.ok) throw new Error(await problemDetail(response));
      setPreview((await response.json()) as BundlePreview);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "The preview could not be created.");
    } finally {
      setBusy(undefined);
    }
  };

  const prepare = async () => {
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
      const filename = disposition.match(/filename="([^"]+)"/)?.[1] ?? "loomarr-support.zip";
      setFinalBytes(blob.size);
      setArtifact({ blob, filename });
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "The bundle could not be downloaded.");
    } finally {
      setBusy(undefined);
    }
  };

  const save = () => {
    if (!artifact) return;
    const url = URL.createObjectURL(artifact.blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = artifact.filename;
    link.click();
    URL.revokeObjectURL(url);
  };

  const category = (label: string, checked: boolean, setChecked: (value: boolean) => void) => (
    <label
      htmlFor={`bundle-${label.replaceAll(" ", "-").toLowerCase()}`}
      className="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm"
    >
      <Checkbox
        id={`bundle-${label.replaceAll(" ", "-").toLowerCase()}`}
        checked={checked}
        onChange={(event) => {
          setChecked(event.currentTarget.checked);
          resetResult();
        }}
      />
      {label}
    </label>
  );
  const correlationFields = [
    ["Request id", "requestId"],
    ["Playback session", "playbackSessionId"],
    ["Channel", "channelId"],
    ["Schedule block", "scheduleBlockId"],
    ["Job", "jobId"],
    ["Process run", "processRunId"],
  ] as const;

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button variant="outline" />}>
        <Archive aria-hidden /> Support bundle
      </DialogTrigger>
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Create a Support bundle</DialogTitle>
          <DialogDescription>
            Select the evidence to package. Loomarr previews and redacts it locally; nothing is submitted.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1 text-sm">
              <span className="font-medium">Time window</span>
              <Select
                value={range}
                onValueChange={(value) => {
                  setRange(value as Range);
                  resetResult();
                }}
              >
                <SelectTrigger aria-label="Support bundle time window">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="1h">Last hour</SelectItem>
                  <SelectItem value="6h">Last 6 hours</SelectItem>
                  <SelectItem value="24h">Last 24 hours</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="rounded-md border border-border bg-muted/20 p-3 text-muted-foreground text-xs">
              <ShieldCheck className="mb-1 size-4 text-success" aria-hidden />
              Secrets, URL credentials, and local paths are defensively redacted again at download.
            </div>
          </div>
          <fieldset className="space-y-2">
            <legend className="mb-2 font-medium text-sm">Contents</legend>
            <div className="grid gap-2 sm:grid-cols-3">
              {category("Application events", events, setEvents)}
              {category("Process metadata", processes, (checked) => {
                setProcesses(checked);
                if (!checked) setProcessOutput(false);
              })}
              {category("Process output", processOutput, (checked) => {
                setProcessOutput(checked);
                if (checked) setProcesses(true);
              })}
            </div>
          </fieldset>
          <details className="rounded-md border border-border p-3">
            <summary className="cursor-pointer font-medium text-sm">Narrow by correlation</summary>
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              {correlationFields.map(([label, key]) => (
                <label htmlFor={`bundle-${key}`} key={key} className="space-y-1 text-sm">
                  <span>{label}</span>
                  <Input
                    id={`bundle-${key}`}
                    value={fields[key] ?? ""}
                    maxLength={128}
                    onChange={(event) => {
                      setFields((current) => ({ ...current, [key]: event.target.value || undefined }));
                      resetResult();
                    }}
                  />
                </label>
              ))}
            </div>
          </details>
          {error && (
            <p
              role="alert"
              className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-destructive text-sm"
            >
              {error}
            </p>
          )}
          {preview && (
            <section
              className="space-y-3 rounded-md border border-border bg-muted/20 p-4"
              aria-label="Support bundle preview"
            >
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <h3 className="font-medium">Bundle preview</h3>
                <span className="font-mono text-sm">Estimated {formatBytes(preview.estimatedBytes)}</span>
              </div>
              <dl className="grid grid-cols-2 gap-2 text-sm sm:grid-cols-4">
                <div>
                  <dt className="text-muted-foreground">Events</dt>
                  <dd className="font-medium">{preview.manifest.counts.events}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Processes</dt>
                  <dd className="font-medium">{preview.manifest.counts.processes}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Outputs</dt>
                  <dd className="font-medium">{preview.manifest.counts.processOutputs}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">Redactions</dt>
                  <dd className="font-medium">{preview.manifest.counts.redactions}</dd>
                </div>
              </dl>
              <div>
                <p className="text-muted-foreground text-xs">Archive contents</p>
                <ul className="mt-1 grid gap-1 font-mono text-xs sm:grid-cols-2">
                  {["manifest.json", ...preview.manifest.entries.map((entry) => entry.name)].map((name) => (
                    <li key={name}>{name}</li>
                  ))}
                </ul>
              </div>
              {preview.manifest.truncationReasons.length > 0 && (
                <p className="text-warning text-xs">
                  Truncated: {preview.manifest.truncationReasons.join(", ")}
                </p>
              )}
              {finalBytes !== undefined && (
                <p className="font-medium text-sm text-success">
                  Prepared · final size {formatBytes(finalBytes)}
                </p>
              )}
            </section>
          )}
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => void runPreview()}
            disabled={busy !== undefined || (!events && !processes && !processOutput)}
          >
            {busy === "preview" ? <LoaderCircle className="animate-spin" aria-hidden /> : <Eye aria-hidden />}{" "}
            Preview contents
          </Button>
          {artifact ? (
            <Button onClick={save}>
              <Download aria-hidden /> Save ZIP
            </Button>
          ) : (
            <Button onClick={() => void prepare()} disabled={!preview || busy !== undefined}>
              {busy === "download" ? (
                <LoaderCircle className="animate-spin" aria-hidden />
              ) : (
                <Archive aria-hidden />
              )}{" "}
              Prepare ZIP
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export { formatBytes, SupportBundle };
