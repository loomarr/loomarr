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
              <Stack gap="1" marginTop={4} render="ul">
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
                    <Text as="div" flex={1} minWidth={0} numberOfLines={1} textRole="compact">
                      <TextLink
                        render={<a href={item.url} rel="noreferrer" target="_blank" title={item.title} />}
                        tone="inherit"
                        underline="hover"
                      >
                        {item.title}
                      </TextLink>
                    </Text>
                    {item.durationMs ? (
                      <Text as="span" flexShrink={0} textRole="hint" tone="secondary">
                        {formatClipDuration(item.durationMs)}
                      </Text>
                    ) : null}
                    <Action icon="play" onPress={() => setPreviewing(item)} variant="ghost">
                      Preview
                    </Action>
                  </Stack>
                ))}
              </Stack>
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
