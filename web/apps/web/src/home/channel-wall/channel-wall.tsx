import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideChannelLayout } from "@loomarr/core/guide";
import {
  formatGuideEpisode,
  formatGuideTimeRange,
  formatProgrammeTitle,
  guideBreakProgramme,
  guideChannelState,
  guideWaitingForTitles,
} from "@loomarr/core/guide";
import { Action, SectionHeader, Surface } from "@loomarr/design-system";
import { NeedsLine, surfChannelData, WallTile } from "@loomarr/ui";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { ArtworkFallback } from "@/components/loomarr/artwork-fallback";
import { ServedImage } from "@/components/loomarr/served-image";
import { usePhoneWidth } from "@/lib/use-phone-width";
import type { AiringArtworkProps, ChannelViewer, ChannelWallProps } from "./channel-wall.type";

// A legacy thumbnail URL that 404s must not leave a blank box (#2762): it falls to the generated art.
const ThumbUrl = ({ url, channel }: { url: string; channel: AiringArtworkProps["channel"] }) => {
  const [failedUrl, setFailedUrl] = useState<string | null>(null);
  if (failedUrl === url) return <ArtworkFallback channel={channel} />;
  return (
    <img
      src={url}
      alt=""
      style={{ height: "100%", objectFit: "cover", width: "100%" }}
      onError={() => setFailedUrl(url)}
    />
  );
};

// AiringArtwork — the picture in a Home card's picture slot (the wall, Watching now): the title's
// backdrop, else the media server's still, else its legacy thumbnail URL, else generated channel
// art (#2762). A card always has a picture, never a blank or hatched tile, and the generated art
// also covers an image that fails to load.
const AiringArtwork = ({ airing, channel, sizes }: AiringArtworkProps) => {
  const image = airing.backdropImage ?? airing.thumbImage;
  if (image) {
    return (
      <ServedImage image={image} alt="" sizes={sizes} fallback={<ArtworkFallback channel={channel} />} />
    );
  }
  if (airing.thumbUrl) return <ThumbUrl url={airing.thumbUrl} channel={channel} />;
  return <ArtworkFallback channel={channel} />;
};

// The actual, truthful lifecycle sentence for a channel with no current block, never a
// first-channel prompt, since the channel already exists (#1822 evidence).
const lifecycleLabel = (channel: GuideChannelLayout): string => {
  const state = guideChannelState(channel.source);
  if (state.health === "paused") return "Paused";
  if (state.health === "creating") return "Building schedule";
  if (state.health === "error") return "Couldn’t reach this channel";
  if (state.health === "drift") return "Catching up";
  return "No programme scheduled now";
};

type WallEntry = {
  channel: GuideChannelLayout;
  channelName: string;
  /** The airing whose picture fills the tile; undefined when the channel has nothing to show. */
  airing?: GuideAiring;
  title: string;
  detail?: string;
  /** How far through the programme on air the channel is. */
  progressPercent?: number;
  /** The channel has nothing to air yet, so its tile is dimmed. */
  dim: boolean;
  /** The channel is all breaks while its titles download (#2727). */
  waiting: boolean;
};

const minutesLeft = (stopMs: number, nowMs: number) => Math.max(1, Math.round((stopMs - nowMs) / 60_000));

// What a channel's tile says. Titles mean programmes, never filler: during a break the tile names
// the programme the break leads back to and says it is an ad break (#2761), and a channel that is
// all breaks while its titles download says so (#2727). A channel with nothing on says why.
const wallEntry = (channel: GuideChannelLayout, nowMs: number, timezone?: string): WallEntry => {
  const data = surfChannelData(channel, nowMs, timezone);
  const identity = { channel, channelName: data.channelName };
  const airing = channel.airings.find((a) => a.source.startMs <= nowMs && a.source.stopMs > nowMs)?.source;
  const waiting = guideWaitingForTitles(channel.source);
  if (waiting) {
    // "Commercials until titles arrive · 3 of 12 downloaded": the headline, then the count.
    const [headline = waiting, ...count] = waiting.split(" · ");
    return {
      ...identity,
      title: headline,
      ...(count.length > 0 ? { detail: count.join(" · ") } : {}),
      dim: true,
      waiting: true,
    };
  }
  if (!data.now || !airing) {
    return { ...identity, title: lifecycleLabel(channel), dim: true, waiting: false };
  }
  if (airing.kind === "filler") {
    const programme = guideBreakProgramme(
      channel.airings.map((a) => a.source),
      airing,
    );
    if (!programme) return { ...identity, airing, title: "Ad break", dim: false, waiting: false };
    const episode = formatGuideEpisode(programme.season, programme.episode);
    return {
      ...identity,
      airing: programme,
      title: programme.series ?? formatProgrammeTitle(programme),
      detail: ["Ad break", episode, formatGuideTimeRange(programme.startMs, programme.stopMs, timezone)]
        .filter(Boolean)
        .join(" · "),
      dim: false,
      waiting: false,
    };
  }
  return {
    ...identity,
    airing,
    title: data.now.seriesTitle ?? data.now.title,
    detail: [data.now.episodeLabel, `${minutesLeft(airing.stopMs, nowMs)} min left`]
      .filter(Boolean)
      .join(" · "),
    progressPercent: ((nowMs - airing.startMs) / (airing.stopMs - airing.startMs)) * 100,
    dim: false,
    waiting: false,
  };
};

// The names on one tile, each person once, in the order they started watching.
const watchersOn = (viewers: ChannelViewer[], channelId: string): string[] => [
  ...new Set(viewers.filter((v) => v.channelId === channelId).map((v) => v.name)),
];

// "2 people watching" in the header: people, not devices.
const watchingCount = (viewers: ChannelViewer[]): string => {
  const people = new Set(viewers.map((v) => v.userId)).size;
  if (people === 0) return "Nobody is watching right now";
  return `${people} ${people === 1 ? "person" : "people"} watching`;
};

// ChannelWall — Home's first section (#2764, Direction mock "Channel wall"): every channel as a live
// tile, like a TV multiview, the channel the viewer tuned last first and marked, the rest in channel
// number order. No hero picture and no filled button: the tile is the control. A channel still
// downloading its titles is a dimmer tile that says so instead of airing an ad as its "now". For an
// admin, the "Needs you" counts sit as one quiet line between the header and the tiles (#2785).
const ChannelWall = ({ lastChannelId, layout, needs = [], nowMs, viewers }: ChannelWallProps) => {
  const phone = usePhoneWidth();
  const byNumber = [...layout.channels].sort((a, b) => a.source.number - b.source.number);
  const last = byNumber.find((c) => c.source.channelId === lastChannelId);
  const ordered = last ? [last, ...byNumber.filter((c) => c !== last)] : byNumber;
  const entries = ordered.map((channel) => wallEntry(channel, nowMs, layout.timezone));
  const filling = entries.filter((e) => e.waiting).length;
  const meta =
    filling > 0
      ? `${filling} of ${entries.length} channels are still filling up`
      : `${entries.length} ${entries.length === 1 ? "channel" : "channels"}`;
  const headerMeta = viewers ? `${meta} · ${watchingCount(viewers)}` : meta;

  return (
    <section aria-labelledby="home-wall">
      <SectionHeader id="home-wall" title="On now" meta={headerMeta}>
        <Action accessibilityRole="link" render={<Link to="/guide" />} variant="ghost">
          Full guide
        </Action>
      </SectionHeader>
      {needs.length > 0 && (
        <Surface backgroundColor="transparent" borderWidth={0} marginBottom={16}>
          <NeedsLine compact={phone} items={needs} />
        </Surface>
      )}
      <Surface
        backgroundColor="transparent"
        borderWidth={0}
        style={{
          display: "grid",
          gap: phone ? 12 : 16,
          gridTemplateColumns: phone ? "repeat(2, minmax(0, 1fr))" : "repeat(3, minmax(0, 1fr))",
        }}
      >
        {entries.map((entry) => {
          const { channel, channelName } = entry;
          const art = entry.airing ? (
            <AiringArtwork
              airing={entry.airing}
              channel={{ name: channelName, number: channel.source.number }}
              sizes="(min-width: 1024px) 33vw, 50vw"
            />
          ) : (
            <ArtworkFallback channel={{ name: channelName, number: channel.source.number }} />
          );
          return (
            <WallTile
              key={channel.source.channelId}
              art={art}
              channelName={channelName}
              compact={phone}
              dim={entry.dim}
              last={channel.source.channelId === lastChannelId}
              number={String(channel.source.number).padStart(2, "0")}
              render={<Link to="/watch/$id" params={{ id: channel.source.channelId }} />}
              title={entry.title}
              {...(channel.source.decade === undefined ? {} : { decade: channel.source.decade })}
              {...(viewers ? { watchers: watchersOn(viewers, channel.source.channelId) } : {})}
              {...(entry.detail ? { detail: entry.detail } : {})}
              {...(entry.progressPercent === undefined ? {} : { progressPercent: entry.progressPercent })}
            />
          );
        })}
      </Surface>
    </section>
  );
};

export { AiringArtwork, ChannelWall };
