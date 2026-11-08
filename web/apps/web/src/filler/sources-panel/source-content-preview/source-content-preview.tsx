import type { FillerProviderDescriptionDTO } from "@loomarr/api/models/fillerProviderDescriptionDTO";
import type { FillerSourcePreviewItemDTO } from "@loomarr/api/models/fillerSourcePreviewItemDTO";
import { formatClipDuration } from "@loomarr/core/format";
import { Action } from "@loomarr/design-system";
import { ExternalLink, Loader2 } from "lucide-react";
import { useState } from "react";
import { SourceItemPreviewDialog } from "../source-item-preview-dialog";

interface SourceContentPreviewProps {
  /** The provider the source belongs to, from the provider registry. */
  provider: FillerProviderDescriptionDTO;
  canonicalUrl: string;
  previewItems?: FillerSourcePreviewItemDTO[];
  loading?: boolean;
  error?: string | null;
}

const SourceContentPreview = ({
  provider,
  canonicalUrl,
  previewItems,
  loading = false,
  error,
}: SourceContentPreviewProps) => {
  const [previewing, setPreviewing] = useState<FillerSourcePreviewItemDTO>();
  const showExamples = loading || Boolean(error) || Boolean(previewItems?.length);

  return (
    <>
      <a
        className="text-tune inline-flex items-center gap-1 text-xs hover:underline"
        href={canonicalUrl}
        target="_blank"
        rel="noreferrer"
      >
        Open on {provider.label}
        <ExternalLink className="size-3" aria-hidden />
      </a>

      {showExamples && (
        <div className="border-border mt-3 border-t pt-3">
          {provider.previewHeading && <p className="text-xs font-medium">{provider.previewHeading}</p>}
          {loading && (
            <div
              className="text-muted-foreground mt-2 flex items-center gap-2 text-xs"
              role="status"
              aria-live="polite"
            >
              <Loader2 className="size-4 animate-spin" aria-hidden />
              Loading examples…
            </div>
          )}
          {!loading && error && <p className="text-muted-foreground mt-2 text-xs">{error}</p>}
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
                      <span className="text-muted-foreground shrink-0 text-xs">
                        {formatClipDuration(item.durationMs)}
                      </span>
                    ) : null}
                    <Action icon="play" onPress={() => setPreviewing(item)} variant="ghost">
                      Preview
                    </Action>
                  </li>
                ))}
              </ul>
              <p className="text-muted-foreground mt-2 text-xs">A small sample from this source.</p>
            </>
          )}
        </div>
      )}

      <SourceItemPreviewDialog
        item={previewing}
        providerLabel={provider.label}
        onClose={() => setPreviewing(undefined)}
      />
    </>
  );
};

export { SourceContentPreview };
