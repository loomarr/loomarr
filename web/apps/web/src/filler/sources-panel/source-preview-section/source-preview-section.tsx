import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import type { FillerSourceSuggestionDTO } from "@loomarr/api/models/fillerSourceSuggestionDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { useEffect, useRef, useState } from "react";
import { useFillerProviders } from "../../use-filler-providers";
import { SourceContentPreview } from "../source-content-preview";

const registeredSourceURL = (source: FillerSourceDTO) => {
  const uri = source.uri?.trim();
  if (!uri) return;
  if (source.kind === "youtube") return uri;
  if (source.kind !== "archive") return;
  if (/^https?:\/\//i.test(uri)) return uri;
  return `https://archive.org/details/${encodeURIComponent(uri)}`;
};

interface SourcePreviewSectionProps {
  source: FillerSourceDTO;
}

// "From this source": a few examples of what an Archive.org or YouTube source holds, resolved once
// when the source is opened. Remount it per source (`key`) — the resolve is a mount effect, and the
// examples it keeps belong to the source it was mounted for. Renders nothing for local sources and
// for a source with no address to resolve.
const SourcePreviewSection = ({ source }: SourcePreviewSectionProps) => {
  const [suggestion, setSuggestion] = useState<FillerSourceSuggestionDTO>();
  const resolveSource = fillerApi.useResolveFillerSource();
  const resolved = useRef(false);
  const remote = useFillerProviders().discoverable(source.kind);

  // The provider registry may answer after the drawer opens, so the resolve waits for it and still
  // runs once.
  useEffect(() => {
    const uri = source.uri?.trim();
    if (resolved.current || !remote || !source.providerEnabled || !uri) return;
    resolved.current = true;
    resolveSource.mutate(
      { kind: remote.id, data: { input: uri } },
      {
        onSuccess: (response) => {
          const result = unwrap(response, (body) => body);
          if (result) setSuggestion(result);
        },
      },
    );
  }, [remote, source, resolveSource]);

  const url = registeredSourceURL(source);
  if (!url || !remote) return null;

  return (
    <section aria-labelledby="source-preview-heading" className="border-border border-t pt-5">
      <h3 id="source-preview-heading" className="font-medium">
        From this source
      </h3>
      <div className="mt-2">
        <SourceContentPreview
          kind={remote.id}
          providerLabel={remote.label}
          canonicalUrl={suggestion?.canonicalUrl ?? url}
          previewItems={suggestion?.previewItems}
          loading={source.providerEnabled && resolveSource.isPending}
          error={
            !source.providerEnabled
              ? `Resume ${remote.label} to refresh these examples.`
              : resolveSource.error
                ? "Examples aren’t available right now. You can still open the source."
                : null
          }
        />
      </div>
    </section>
  );
};

export { registeredSourceURL, SourcePreviewSection };
