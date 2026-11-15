import type { FillerProviderDescriptionDTO } from "@loomarr/api/models/fillerProviderDescriptionDTO";
import type { FillerSourcePreviewItemDTO } from "@loomarr/api/models/fillerSourcePreviewItemDTO";
import { formatClipDuration } from "@loomarr/core/format";
import { Action, LoadingStrip, Stack, Text, TextLink } from "@loomarr/design-system";
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
      <Text as="span" textRole="hint">
        <TextLink external href={canonicalUrl}>
          Open on {provider.label}
        </TextLink>
      </Text>

      {showExamples && (
        <Stack borderColor="$borderDecorative" borderTopWidth={1} marginTop={12} paddingTop={12}>
          {provider.previewHeading && (
            <Text as="p" textRole="label" tone="primary">
              {provider.previewHeading}
            </Text>
          )}
          {loading && (
            <Stack align="center" aria-live="polite" direction="row" gap="inline" marginTop={8} role="status">
              <LoadingStrip decorative width={32} />
              <Text as="span" textRole="hint" tone="secondary">
                Loading examples…
              </Text>
            </Stack>
          )}
          {!loading && error && (
            <Stack marginTop={8}>
              <Text as="p" textRole="hint" tone="secondary">
                {error}
              </Text>
            </Stack>
          )}
          {!loading && previewItems && previewItems.length > 0 && (
            <>
              <ul className="mt-1 space-y-1">
                {previewItems.map((item) => (
                  <Stack
                    align="center"
                    direction="row"
                    flexShrink={1}
                    gap="inline"
                    key={item.url}
                    minWidth={0}
                    render="li"
                  >
                    <Text as="div" className="min-w-0 flex-1 truncate" textRole="compact">
                      <TextLink
                        render={<a href={item.url} rel="noreferrer" target="_blank" title={item.title} />}
                      >
                        {item.title}
                      </TextLink>
                    </Text>
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
              <Stack marginTop={8}>
                <Text as="p" textRole="hint" tone="secondary">
                  A small sample from this source.
                </Text>
              </Stack>
            </>
          )}
        </Stack>
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
