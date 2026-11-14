import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { countryLabel, formatClipDuration, topicLabel } from "@loomarr/core/format";
import { Action, Stack, Text, TextLink } from "@loomarr/design-system";
import { useState } from "react";
import { AUDIENCE_LABEL, playsLine, ROLE_LABEL } from "@/components/loomarr/filler/clip-card";
import { ClipPreview } from "@/components/loomarr/filler/clip-preview";

// Inspection only. Corrections and Incoming choices are owned by their callers, not playback.
const ClipDetails = ({ clip, onEdit }: { clip: ClipDTO; onEdit?: () => void }) => {
  const [revealedHash, setRevealedHash] = useState<string>();
  const showPreview = !clip.held || revealedHash === clip.hash;
  const sourceURL = originalSourceURL(clip.sourceUrl);
  const sourceLabel =
    clip.sourceLabel ??
    (clip.source === "filler-dir" || clip.source === "folder"
      ? "Your clip folder"
      : clip.source
        ? "Imported source"
        : undefined);
  const suggestion = clip.contextSuggestion;
  const suggestedYear = suggestion?.year
    ? String(suggestion.year)
    : suggestion?.decade
      ? `${suggestion.decade}s`
      : undefined;
  const verifiedLocation =
    clip.geographicScope === "national" || clip.geographicScope === "local"
      ? [
          clip.country ? countryLabel(clip.country) : undefined,
          clip.geographicScope === "local" ? (clip.stationLabel ?? clip.station) : "National",
        ]
          .filter(Boolean)
          .join(" · ")
      : undefined;
  const facts = [
    ["Type", clip.role === "unclassified" ? undefined : ROLE_LABEL[clip.role]],
    [
      "Year",
      clip.era
        ? clip.era % 10 === 0
          ? `${clip.era}s`
          : String(clip.era)
        : suggestedYear
          ? `${suggestedYear} · Likely`
          : undefined,
    ],
    ["Audience", clip.audience ? AUDIENCE_LABEL[clip.audience] : undefined],
    ["Advertiser", clip.brand],
    ["Location", verifiedLocation ?? (suggestion?.country ? `${suggestion.country} · Likely` : undefined)],
    ["Topics", clip.assertedTags?.map(topicLabel).join(", ")],
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
      <div className="border-border overflow-hidden rounded-lg border">
        {showPreview ? (
          <ClipPreview clip={clip} />
        ) : (
          <div className="space-y-3 p-4">
            <Text as="p" textRole="compact" tone="secondary">
              This clip is not ready for channels yet. Open the preview when you are ready.
            </Text>
            <Stack direction="row">
              <Action onPress={() => setRevealedHash(clip.hash)} variant="outline">
                Preview clip
              </Action>
            </Stack>
          </div>
        )}
      </div>
      {enrichmentMessage ? (
        <div className="bg-muted/40 rounded-lg px-4 py-3">
          <Text as="p" textRole="cardLabel">
            {enrichmentMessage.title}
          </Text>
          <Text as="p" className="mt-1" textRole="compact" tone="secondary">
            {enrichmentMessage.detail}
          </Text>
        </div>
      ) : null}
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2">
        <Text as="dt" textRole="compact" tone="secondary">
          Length
        </Text>
        <Text as="dd" textRole="compact">
          {formatClipDuration(clip.durationMs)}
        </Text>
        {facts.map(([label, value]) => (
          <div key={label} className="contents">
            <Text as="dt" textRole="compact" tone="secondary">
              {label}
            </Text>
            <Text as="dd" className="break-words" textRole="compact">
              {value}
            </Text>
          </div>
        ))}
        {suggestion?.explanation || suggestion?.sources.length ? (
          <>
            <Text as="dt" textRole="compact" tone="secondary">
              Context
            </Text>
            <Text as="dd" className="min-w-0 space-y-1 break-words" textRole="compact">
              {suggestion.explanation ? <p>{suggestion.explanation}</p> : null}
              {suggestion.sources.length ? (
                <Stack columnGap="3" direction="row" rowGap="1" wrap>
                  {suggestion.sources.map((source) => (
                    <TextLink external href={source.url} key={source.url}>
                      {source.title}
                    </TextLink>
                  ))}
                </Stack>
              ) : null}
            </Text>
          </>
        ) : null}
        {sourceLabel || sourceURL ? (
          <>
            <Text as="dt" textRole="compact" tone="secondary">
              Source
            </Text>
            <Text
              as="dd"
              className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 break-words"
              textRole="compact"
            >
              {sourceLabel ? <span>{sourceLabel}</span> : null}
              {sourceURL ? (
                <TextLink external href={sourceURL}>
                  View original
                </TextLink>
              ) : null}
            </Text>
          </>
        ) : null}
      </dl>
      <Text as="p" textRole="hint" tone="secondary">
        {playsLine(clip)}
      </Text>
      {!clip.held && !clip.isComposite ? (
        <Text as="p" textRole="compact" tone="secondary">
          Channels choose clips from this library automatically, using their filler settings.
        </Text>
      ) : null}
      {onEdit ? (
        <Stack direction="row">
          <Action onPress={onEdit} variant="outline">
            Edit details
          </Action>
        </Stack>
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
