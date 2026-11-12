import type { FillerProviderDescriptionDTO } from "@loomarr/api/models/fillerProviderDescriptionDTO";
import type { FillerSourcePreviewItemDTO } from "@loomarr/api/models/fillerSourcePreviewItemDTO";
import { formatClipDuration } from "@loomarr/core/format";
import { Action, Icon, icons, LoadingStrip, Stack } from "@loomarr/design-system";
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
        <Icon decorative glyph={icons.external} size="compact" tone="inherit" />
      </a>

      {showExamples && (
        <div className="border-border mt-3 border-t pt-3">
          {provider.previewHeading && <p className="text-xs font-medium">{provider.previewHeading}</p>}
          {loading && (
            <Stack
              align="center"
              aria-live="polite"
              className="text-muted-foreground mt-2 text-xs"
              direction="row"
              gap="inline"
              role="status"
            >
              <LoadingStrip decorative width={32} />
              Loading examples…
            </Stack>
          )}
          {!loading && error && <p className="text-muted-foreground mt-2 text-xs">{error}</p>}
          {!loading && previewItems && previewItems.length > 0 && (
            <>
              <ul className="mt-1 space-y-1">
                {previewItems.map((item) => (
                  <Stack
                    align="center"
                    className="min-w-0"
                    direction="row"
                    gap="inline"
                    key={item.url}
                    render="li"
                  >
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
                  </Stack>
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
