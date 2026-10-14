import type { FillerSourcePreviewItemDTO } from "@loomarr/api/models/fillerSourcePreviewItemDTO";
import { formatClipDuration } from "@loomarr/core/format";
import { ExternalLink, Loader2, Play } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { SourceItemPreviewDialog } from "../source-item-preview-dialog";

interface SourceContentPreviewProps {
  kind: "archive" | "youtube";
  canonicalUrl: string;
  previewItems?: FillerSourcePreviewItemDTO[];
  loading?: boolean;
  error?: string | null;
}

const SourceContentPreview = ({
  kind,
  canonicalUrl,
  previewItems,
  loading = false,
  error,
}: SourceContentPreviewProps) => {
  const [previewing, setPreviewing] = useState<FillerSourcePreviewItemDTO>();
  const provider = kind === "archive" ? "Archive.org" : "YouTube";
  const heading = kind === "archive" ? "Popular videos in this collection" : "A few videos from this source";
  const showExamples = loading || Boolean(error) || Boolean(previewItems?.length);

  return (
    <>
      <a
        className="inline-flex items-center gap-1 text-tune text-xs hover:underline"
        href={canonicalUrl}
        target="_blank"
        rel="noreferrer"
      >
        Open on {provider}
        <ExternalLink className="size-3" aria-hidden />
      </a>

      {showExamples && (
        <div className="mt-3 border-border border-t pt-3">
          <p className="font-medium text-xs">{heading}</p>
          {loading && (
            <div
              className="mt-2 flex items-center gap-2 text-muted-foreground text-xs"
              role="status"
              aria-live="polite"
            >
              <Loader2 className="size-4 animate-spin" aria-hidden />
              Loading examples…
            </div>
          )}
          {!loading && error && <p className="mt-2 text-muted-foreground text-xs">{error}</p>}
          {!loading && previewItems && previewItems.length > 0 && (
            <>
              <ul className="mt-1 space-y-1">
                {previewItems.map((item) => (
                  <li key={item.url} className="flex min-w-0 items-center gap-2">
                    <a
                      className="min-w-0 flex-1 truncate text-sm hover:underline"
                      href={item.url}
                      target="_blank"
                      rel="noreferrer"
                      title={item.title}
                    >
                      {item.title}
                    </a>
                    {item.durationMs ? (
                      <span className="shrink-0 text-muted-foreground text-xs">
                        {formatClipDuration(item.durationMs)}
                      </span>
                    ) : null}
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-7 shrink-0 px-2"
                      onClick={() => setPreviewing(item)}
                    >
                      <Play aria-hidden />
                      Preview
                    </Button>
                  </li>
                ))}
              </ul>
              <p className="mt-2 text-muted-foreground text-xs">A small sample from this source.</p>
            </>
          )}
        </div>
      )}

      <SourceItemPreviewDialog item={previewing} kind={kind} onClose={() => setPreviewing(undefined)} />
    </>
  );
};

export { SourceContentPreview };
