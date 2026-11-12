import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { formatClipDuration, pluralize } from "@loomarr/core/format";
import { Action, Disclosure, Stack, Text } from "@loomarr/design-system";
import { type ReactNode, useState } from "react";

interface CompositeCatalogGroupProps {
  clip: ClipDTO;
  onManage: () => void;
  renderParent: (clip: ClipDTO) => ReactNode;
  renderChild: (clip: ClipDTO) => ReactNode;
}

// A compilation in the catalog: the ordinary clip surface as the parent, plus a disclosure that
// loads its filed segments only once opened.
const CompositeCatalogGroup = ({ clip, onManage, renderParent, renderChild }: CompositeCatalogGroupProps) => {
  const [open, setOpen] = useState(false);
  const children = fillerApi.useListFiller(
    { parentHash: clip.hash, limit: 500 },
    { query: { enabled: open } },
  );
  const rows = unwrap(children.data, (body) => body.clips) ?? [];
  const total = unwrap(children.data, (body) => body.total) ?? 0;

  return (
    <Stack gap="inline">
      {/* Keep the ordinary clip surface as the parent. A compilation still needs the same
          inspection entry point and confirmed split action; replacing it with a
          bespoke group heading made those established actions disappear. The disclosure below
          adds hierarchy without creating a second, weaker representation of the parent. */}
      {renderParent(clip)}
      {/* The header is the whole press target, so the segment actions live in the open panel. */}
      <Disclosure
        label={`Segments from ${clip.name}`}
        description={`${formatClipDuration(clip.durationMs)} source reel`}
        expanded={open}
        onExpandedChange={setOpen}
      >
        {children.isFetching ? (
          <Text as="span" textRole="hint" tone="secondary">
            Loading segments…
          </Text>
        ) : (
          <Stack direction="row">
            <Action onPress={onManage} variant="outline">
              {`Manage ${pluralize(total, "segment")}`}
            </Action>
          </Stack>
        )}
        {children.error ? (
          <Text as="p" textRole="compact" tone="danger">
            Segments could not be loaded.
          </Text>
        ) : rows.length > 0 ? (
          <div className="border-border overflow-hidden rounded-lg border">{rows.map(renderChild)}</div>
        ) : children.isFetching ? null : (
          <Text as="p" textRole="compact" tone="secondary">
            No filed segments remain under this compilation.
          </Text>
        )}
      </Disclosure>
    </Stack>
  );
};

export type { CompositeCatalogGroupProps };
export { CompositeCatalogGroup };
