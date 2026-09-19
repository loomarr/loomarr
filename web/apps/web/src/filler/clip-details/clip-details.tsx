import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { formatClipDuration } from "@loomarr/core/format";
import { ExternalLink } from "lucide-react";
import { useState } from "react";
import { AUDIENCE_LABEL, KIND_LABEL, playsLine } from "@/components/loomarr/filler/clip-card";
import { ClipPreview } from "@/components/loomarr/filler/clip-preview";
import { Button } from "@/components/ui/button";

// Inspection only. Corrections and Incoming choices are owned by their callers, not playback.
const ClipDetails = ({ clip, onEdit }: { clip: ClipDTO; onEdit?: () => void }) => {
  const [revealedHash, setRevealedHash] = useState<string>();
  const showPreview = !clip.held || revealedHash === clip.hash;
  const sourceURL = originalSourceURL(clip.sourceUrl);
  const facts = [
    ["Type", clip.kind === "unclassified" ? undefined : KIND_LABEL[clip.kind]],
    ["Year", clip.era ? (clip.era % 10 === 0 ? `${clip.era}s` : String(clip.era)) : undefined],
    ["Audience", clip.audience ? AUDIENCE_LABEL[clip.audience] : undefined],
    ["Advertiser", clip.brand],
    [
      "Location",
      clip.geographicScope === "national" || clip.geographicScope === "local"
        ? [clip.country, clip.geographicScope === "local" ? clip.market : "National"]
            .filter(Boolean)
            .join(" · ")
        : undefined,
    ],
    ["Topics", clip.assertedTags?.join(", ")],
  ].filter(([, value]) => value);
  const enrichmentMessage =
    clip.enrichment?.state === "adding_details"
      ? {
          title: "Adding details",
          detail: "Loomarr is checking this clip in the background.",
        }
      : clip.enrichment?.state === "details_limited"
        ? {
            title: "Details limited",
            detail: "Some details couldn't be confirmed. That won't block playback.",
          }
        : undefined;

  return (
    <div className="space-y-5">
      <div className="overflow-hidden rounded-lg border border-border">
        {showPreview ? (
          <ClipPreview clip={clip} />
        ) : (
          <div className="space-y-3 p-4">
            <p className="text-muted-foreground text-sm">
              This clip is not ready for channels yet. Open the preview when you are ready.
            </p>
            <Button variant="outline" size="sm" onClick={() => setRevealedHash(clip.hash)}>
              Preview clip
            </Button>
          </div>
        )}
      </div>
      {enrichmentMessage ? (
        <div className="rounded-lg bg-muted/40 px-4 py-3 text-sm">
          <p className="font-medium">{enrichmentMessage.title}</p>
          <p className="mt-1 text-muted-foreground">{enrichmentMessage.detail}</p>
        </div>
      ) : null}
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm">
        <dt className="text-muted-foreground">Length</dt>
        <dd>{formatClipDuration(clip.durationMs)}</dd>
        {facts.map(([label, value]) => (
          <div key={label} className="contents">
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="break-words">{value}</dd>
          </div>
        ))}
        {clip.source || sourceURL ? (
          <>
            <dt className="text-muted-foreground">Source</dt>
            <dd className="min-w-0 break-words">
              {sourceURL ? (
                <a
                  href={sourceURL}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 text-signal underline underline-offset-4"
                >
                  View original <ExternalLink className="size-3.5 shrink-0" aria-hidden />
                </a>
              ) : clip.source === "filler-dir" || clip.source === "folder" ? (
                "Your clip folder"
              ) : (
                clip.source
              )}
            </dd>
          </>
        ) : null}
      </dl>
      <p className="text-muted-foreground text-xs">{playsLine(clip)}</p>
      {!clip.held && !clip.isComposite ? (
        <p className="text-muted-foreground text-sm">
          Channels choose clips from this library automatically, using their filler settings.
        </p>
      ) : null}
      {onEdit ? (
        <Button variant="outline" size="sm" onClick={onEdit}>
          Edit details
        </Button>
      ) : null}
    </div>
  );
};

const originalSourceURL = (source: string | undefined): string | undefined => {
  if (!source) return undefined;
  try {
    const url = new URL(source);
    return !url.username && !url.password && (url.protocol === "https:" || url.protocol === "http:")
      ? url.href
      : undefined;
  } catch {
    return undefined;
  }
};

export { ClipDetails };
