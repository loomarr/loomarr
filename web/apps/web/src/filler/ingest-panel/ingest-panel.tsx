import { fillerApi } from "@loomarr/api";
import { useState } from "react";
import { ErrorState } from "@/components/loomarr";
import { Button, Card, Input, Label } from "@/components/ui";
import { useLoomarrEventListener } from "@/events";
import { cn } from "@/lib";
import type { IngestPanelProps } from "./ingest-panel.type";

// IngestPanel — download clips into the drop-folder (§10). Downloads run for minutes to
// hours, so the request returns a job id immediately and progress arrives on the SSE bus
// as `filler_ingest` frames — the same shape the §8.1 model pull uses.
const IngestPanel = ({ ingestAvailable, onIngested, className }: IngestPanelProps) => {
  const [urls, setUrls] = useState("");
  const [job, setJob] = useState<{ status: string; fetched?: number; error?: string }>();

  const ingest = fillerApi.useIngestFiller({
    mutation: { onSuccess: () => setJob({ status: "starting" }) },
  });

  useLoomarrEventListener({
    onFillerIngest: (e) => {
      setJob({ status: e.status ?? "", fetched: e.fetched, error: e.error });
      // A finished download changes the drop-folder, but Tunarr has to scan it before
      // the clips exist as programs — so refresh the catalog rather than assuming.
      if (e.status === "success") onIngested?.();
    },
  });

  // ⚠ A NOTE, not a Card. This branch is an infrastructure caveat ("your image lacks the
  // downloader"), and rendering it as a full card with an <h2> gave it the same visual
  // weight as the features around it — three equal slabs where one was an apology. It is a
  // footnote to Find clips, so it reads as one.
  //
  // ⚠ This gate changed MEANING and the copy had not caught up. It used to say "switch to
  // the loomarr:filler image" — a tag that no longer exists. That was right when the
  // tooling was an opt-in variant, but the
  // single image (§16) ships yt-dlp and ffmpeg always, so an operator sent looking for that
  // tag finds nothing. Unavailable now means the binaries are missing or unrunnable in THIS
  // install: a degraded deployment, not a choice not yet made.
  //
  // Still not a Settings pointer: the paths are configurable (§15) but a wrong path is the
  // rare cause here, and the common one — a hand-built image without the vendored binaries —
  // no setting can fix. The copy says what is true and offers the manual route that works.
  if (!ingestAvailable) {
    return (
      <p className={cn("text-muted-foreground text-sm", className)}>
        Downloading isn't available in this install, so add clips by dropping them into the filler folder. The
        official image ships the downloader; a custom build may be missing it.
      </p>
    );
  }

  const submit = () => {
    const list = urls
      .split(/[\n,]/)
      .map((u) => u.trim())
      .filter(Boolean);
    if (list.length > 0) ingest.mutate({ data: { urls: list } });
  };

  return (
    <Card className={className}>
      <div className="flex flex-col gap-3 p-4">
        <div>
          <h2 className="font-semibold text-lg">Download clips</h2>
          <p className="mt-1 text-muted-foreground text-sm">
            YouTube playlists or videos, and Archive.org collections or items. Files land in the drop-folder;
            run Sync afterwards so Tunarr scans them.
          </p>
        </div>

        {ingest.error != null && <ErrorState error={ingest.error} />}

        <div>
          <Label htmlFor="ingest-urls">URLs</Label>
          <Input
            id="ingest-urls"
            value={urls}
            placeholder="https://archive.org/details/classic-tv-commercials"
            onChange={(e) => setUrls(e.target.value)}
          />
          <p className="mt-1 text-static-400 text-xs">Separate several with commas.</p>
        </div>

        {job && (
          <p
            role="status"
            className={job.status === "error" ? "text-onair-300 text-sm" : "text-muted-foreground text-sm"}
          >
            {job.status === "error"
              ? (job.error ?? "The download failed.")
              : job.status === "success"
                ? `Downloaded ${job.fetched ?? 0} clip${job.fetched === 1 ? "" : "s"}. Run Sync to add them.`
                : "Downloading… this can take a while for a large playlist."}
          </p>
        )}

        <div className="flex justify-end">
          <Button size="sm" disabled={ingest.isPending || urls.trim() === ""} onClick={submit}>
            {ingest.isPending ? "Starting…" : "Download"}
          </Button>
        </div>
      </div>
    </Card>
  );
};

export { IngestPanel };
