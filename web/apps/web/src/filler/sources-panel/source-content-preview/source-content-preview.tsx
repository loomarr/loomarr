import type { FillerProviderDescriptionDTO } from "@loomarr/api/models/fillerProviderDescriptionDTO";
import type { FillerSourcePreviewItemDTO } from "@loomarr/api/models/fillerSourcePreviewItemDTO";
import { formatClipDuration } from "@loomarr/core/format";
import { Action, Icon, icons, LoadingStrip, Stack, Text } from "@loomarr/design-system";
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
          {provider.previewHeading && (
            <Text as="p" textRole="label" tone="primary">
              {provider.previewHeading}
            </Text>
          )}
          {loading && (
            <Stack
              align="center"
              aria-live="polite"
              className="mt-2"
              direction="row"
              gap="inline"
              role="status"
            >
              <LoadingStrip decorative width={32} />
              <Text as="span" textRole="hint" tone="secondary">
                Loading examples…
              </Text>
            </Stack>
          )}
          {!loading && error && (
            <Text as="p" className="mt-2" textRole="hint" tone="secondary">
              {error}
            </Text>
          )}
          {!loading && previewItems && previewItems.length > 0 && (
            <>
              <ul className="mt-1 space-y-1">
                {previewItems.map((item) => (
                  <Stack
                    align="center"
                    className="min-w-0"
                    direction="row"
                    flexShrink={1}
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
                      <Text as="span" className="shrink-0" textRole="hint" tone="secondary">
                        {formatClipDuration(item.durationMs)}
                      </Text>
                    ) : null}
                    <Action icon="play" onPress={() => setPreviewing(item)} variant="ghost">
                      Preview
                    </Action>
                  </Stack>
                ))}
              </ul>
              <Text as="p" className="mt-2" textRole="hint" tone="secondary">
                A small sample from this source.
              </Text>
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
