import { fillerApi } from "@loomarr/api";
import { useState } from "react";
import { ErrorState } from "@/components/loomarr";
import { Button, Card, Input, Label } from "@/components/ui";
import { useLoomarrEventListener } from "@/events";
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

  if (!ingestAvailable) {
    return (
      <Card className={className}>
        <div className="p-4">
          <h2 className="font-semibold text-lg">Download clips</h2>
          {/* No setting can open this gate — it depends on which IMAGE is running
              (config-design §7's one environment-derived feature). Pointing the operator
              at Settings would be a dead end, so the copy names the image instead. */}
          <p className="mt-1 text-muted-foreground text-sm">
            This image doesn't include the download tooling. Run the{" "}
            <code className="font-mono text-static-200">loomarr:filler</code> image to fetch clips in-app —
            same config, same data, just a different tag. Clips added to the drop-folder by hand or with
            MeTube work on either image.
          </p>
        </div>
      </Card>
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
