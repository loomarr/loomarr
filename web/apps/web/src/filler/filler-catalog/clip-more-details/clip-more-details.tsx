import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import type { ClipEnrichmentFactDTOAxis } from "@loomarr/api/models/clipEnrichmentFactDTOAxis";
import type { ClipEnrichmentFactDTOEvidence } from "@loomarr/api/models/clipEnrichmentFactDTOEvidence";
import { formatBytes } from "@loomarr/core/format";

const ENRICHMENT_AXIS_LABEL: Record<ClipEnrichmentFactDTOAxis, string> = {
  kind: "Type",
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
    <details className="border-border rounded-lg border p-4 text-sm">
      <summary className="cursor-pointer font-medium">More about this clip</summary>
      <dl className="mt-3 space-y-2">
        {videoLine ? (
          <div>
            <dt className="text-muted-foreground">Video</dt>
            <dd>{videoLine}</dd>
          </div>
        ) : clip.quality ? (
          <div>
            <dt className="text-muted-foreground">Resolution</dt>
            <dd>{clip.quality}</dd>
          </div>
        ) : null}
        {audioLine ? (
          <div>
            <dt className="text-muted-foreground">Audio</dt>
            <dd>{audioLine}</dd>
          </div>
        ) : null}
        {fileLine ? (
          <div>
            <dt className="text-muted-foreground">File</dt>
            <dd>{fileLine}</dd>
          </div>
        ) : null}
        {clip.language ? (
          <div>
            <dt className="text-muted-foreground">Language</dt>
            <dd>{clip.language === "none" ? "No speech" : clip.language}</dd>
          </div>
        ) : null}
        {clip.license ? (
          <div>
            <dt className="text-muted-foreground">Source license</dt>
            <dd className="break-words">{clip.license}</dd>
          </div>
        ) : null}
        {clip.enrichment?.facts?.length ? (
          <div>
            <dt className="text-muted-foreground">How details were found</dt>
            <dd>
              <ul className="mt-1 space-y-1">
                {clip.enrichment.facts.map((fact) => (
                  <li key={fact.axis} className="flex justify-between gap-4">
                    <span>{ENRICHMENT_AXIS_LABEL[fact.axis] ?? fact.axis}</span>
                    <span className="text-muted-foreground text-right">
                      {ENRICHMENT_EVIDENCE_LABEL[fact.evidence] ?? fact.evidence}
                    </span>
                  </li>
                ))}
              </ul>
            </dd>
          </div>
        ) : null}
        {!videoLine &&
        !audioLine &&
        !fileLine &&
        !clip.quality &&
        !clip.language &&
        !clip.license &&
        !clip.enrichment?.facts?.length ? (
          <p className="text-muted-foreground">No additional details recorded yet.</p>
        ) : null}
      </dl>
    </details>
  );
};

export { ClipMoreDetails, displayAudioChannels, displayAudioRate, displayCodec, displayFrameRate };
