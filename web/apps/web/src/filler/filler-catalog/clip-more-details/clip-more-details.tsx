import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import type { ClipEnrichmentFactDTOAxis } from "@loomarr/api/models/clipEnrichmentFactDTOAxis";
import type { ClipEnrichmentFactDTOEvidence } from "@loomarr/api/models/clipEnrichmentFactDTOEvidence";
import { formatBytes } from "@loomarr/core/format";
import { Stack, Text } from "@loomarr/design-system";

const ENRICHMENT_AXIS_LABEL: Record<ClipEnrichmentFactDTOAxis, string> = {
  role: "Type",
  era: "Year",
  audience: "Audience",
  brand: "Advertiser",
  geography: "Location",
  language: "Language",
  product: "Product",
  format: "Format",
  seasonal: "Season",
  "audience-cue": "Audience cue",
  presentation: "Presentation",
};

const ENRICHMENT_EVIDENCE_LABEL: Record<ClipEnrichmentFactDTOEvidence, string> = {
  operator: "You set this",
  item_metadata: "Item details",
  content_observation: "Clip audio or frames",
  trusted_mapping: "A recognized source",
  source_default: "Source settings",
  inference: "Automatic analysis",
};

const displayCodec = (codec: string | undefined) => {
  if (!codec) return undefined;
  const known: Record<string, string> = { h264: "H.264", h265: "H.265", hevc: "HEVC", aac: "AAC" };
  return known[codec.toLowerCase()] ?? codec.toUpperCase();
};

const displayFrameRate = (cadence: string | undefined) => {
  if (!cadence) return undefined;
  const [rawNumerator, rawDenominator] = cadence.split("/");
  const numerator = Number(rawNumerator);
  const denominator = rawDenominator ? Number(rawDenominator) : 1;
  const fps = numerator / denominator;
  return Number.isFinite(fps) && fps > 0
    ? `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(fps)} fps`
    : undefined;
};

const displayAudioChannels = (channels: number | undefined) => {
  if (channels === 1) return "mono";
  if (channels === 2) return "stereo";
  return channels ? `${channels} channels` : undefined;
};

const displayAudioRate = (rate: number | undefined) =>
  rate
    ? `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 }).format(rate / 1000)} kHz`
    : undefined;

interface ClipMoreDetailsProps {
  clip: ClipDTO;
}

// The collapsed "More about this clip" block of the clip panel: the technical facts of the file
// (video, audio, container and size), language, source licence, and how each detail was found.
const ClipMoreDetails = ({ clip }: ClipMoreDetailsProps) => {
  const videoLine = clip.media
    ? [
        clip.media.width && clip.media.height ? `${clip.media.width}×${clip.media.height}` : undefined,
        displayCodec(clip.media.videoCodec),
        displayFrameRate(clip.media.frameRate),
      ]
        .filter(Boolean)
        .join(" · ")
    : undefined;
  const audioLine = clip.media
    ? [
        displayCodec(clip.media.audioCodec),
        displayAudioChannels(clip.media.audioChannels),
        displayAudioRate(clip.media.audioRateHz),
      ]
        .filter(Boolean)
        .join(" · ")
    : undefined;
  const fileLine = clip.media
    ? [clip.media.container?.toUpperCase(), clip.media.bytes ? formatBytes(clip.media.bytes) : undefined]
        .filter(Boolean)
        .join(" · ")
    : undefined;

  return (
    <details className="border-border rounded-lg border p-4">
      <Text as="summary" className="cursor-pointer" textRole="cardLabel">
        More about this clip
      </Text>
      <dl className="mt-3 space-y-2">
        {videoLine ? (
          <div>
            <Text as="dt" textRole="compact" tone="secondary">
              Video
            </Text>
            <Text as="dd" textRole="compact">
              {videoLine}
            </Text>
          </div>
        ) : clip.quality ? (
          <div>
            <Text as="dt" textRole="compact" tone="secondary">
              Resolution
            </Text>
            <Text as="dd" textRole="compact">
              {clip.quality}
            </Text>
          </div>
        ) : null}
        {audioLine ? (
          <div>
            <Text as="dt" textRole="compact" tone="secondary">
              Audio
            </Text>
            <Text as="dd" textRole="compact">
              {audioLine}
            </Text>
          </div>
        ) : null}
        {fileLine ? (
          <div>
            <Text as="dt" textRole="compact" tone="secondary">
              File
            </Text>
            <Text as="dd" textRole="compact">
              {fileLine}
            </Text>
          </div>
        ) : null}
        {clip.language ? (
          <div>
            <Text as="dt" textRole="compact" tone="secondary">
              Language
            </Text>
            <Text as="dd" textRole="compact">
              {clip.language === "none" ? "No speech" : clip.language}
            </Text>
          </div>
        ) : null}
        {clip.license ? (
          <div>
            <Text as="dt" textRole="compact" tone="secondary">
              Source license
            </Text>
            <Text as="dd" className="break-words" textRole="compact">
              {clip.license}
            </Text>
          </div>
        ) : null}
        {clip.enrichment?.facts?.length ? (
          <div>
            <Text as="dt" textRole="compact" tone="secondary">
              How details were found
            </Text>
            <Text as="dd" textRole="compact">
              <ul className="mt-1 space-y-1">
                {clip.enrichment.facts.map((fact) => (
                  <Stack direction="row" gap="4" justify="between" key={fact.axis} render="li">
                    <span>{ENRICHMENT_AXIS_LABEL[fact.axis] ?? fact.axis}</span>
                    <Text as="span" className="text-right" textRole="compact" tone="secondary">
                      {ENRICHMENT_EVIDENCE_LABEL[fact.evidence] ?? fact.evidence}
                    </Text>
                  </Stack>
                ))}
              </ul>
            </Text>
          </div>
        ) : null}
        {!videoLine &&
        !audioLine &&
        !fileLine &&
        !clip.quality &&
        !clip.language &&
        !clip.license &&
        !clip.enrichment?.facts?.length ? (
          <Text as="p" textRole="compact" tone="secondary">
            No additional details recorded yet.
          </Text>
        ) : null}
      </dl>
    </details>
  );
};

export { ClipMoreDetails, displayAudioChannels, displayAudioRate, displayCodec, displayFrameRate };
