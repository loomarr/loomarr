import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideChannelLayout, GuideChannelState } from "@loomarr/core/guide";
import {
  formatGuideEpisode,
  formatGuideTimeRange,
  guideAiringLabel,
  guideBreakProgramme,
  guideChannelState,
  guideWaitingForTitles,
} from "@loomarr/core/guide";
import type { StatusTone } from "@loomarr/design-system";
import { Action, SectionHeader } from "@loomarr/design-system";
import {
  CardAction,
  CardStatus,
  ChannelChip,
  ListGroup,
  ListRow,
  PictureCard,
  surfChannelData,
} from "@loomarr/ui";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { ArtworkFallback } from "@/components/loomarr/artwork-fallback";
import { ServedImage } from "@/components/loomarr/served-image";
import type { AiringArtworkProps, OnNowProps } from "./on-now.type";

const MAX_CHANNELS = 6;

// A legacy thumbnail URL that 404s must not leave a blank box (#2762): it falls to the generated art.
const ThumbUrl = ({ url, channel }: { url: string; channel: AiringArtworkProps["channel"] }) => {
  const [failedUrl, setFailedUrl] = useState<string | null>(null);
  if (failedUrl === url) return <ArtworkFallback channel={channel} />;
  return <img src={url} alt="" className="size-full object-cover" onError={() => setFailedUrl(url)} />;
};

// AiringArtwork — the picture in a Home card's picture slot (On now, Watching now, Recently
// tuned): the title's backdrop, else the media server's still, else its legacy thumbnail URL, else
// generated channel art (#2762). A card always has a picture, never a blank or hatched tile, and
// the generated art also covers an image that fails to load.
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

// The actual, truthful lifecycle sentence for a channel with no current block — never a
// first-channel prompt, since the channel already exists (#1822 evidence).
const lifecycle = (state: GuideChannelState): { label: string; tone: StatusTone } => {
  if (state.health === "paused") return { label: "Paused", tone: "off" };
  if (state.health === "creating") return { label: "Building schedule", tone: "progress" };
  if (state.health === "error") return { label: "Couldn’t reach this channel", tone: "error" };
  if (state.health === "drift") return { label: "Catching up", tone: "warn" };
  return { label: "No programme scheduled now", tone: "off" };
};

type OnNowCard = {
  channel: GuideChannelLayout;
  channelName: string;
  channelLogoUri?: string;
  /** The airing whose picture fills the card. */
  airing: GuideAiring;
  /** The card's title: always a programme, never filler (#2761). */
  title?: string;
  episodeLabel?: string;
  timeLabel?: string;
  /** The channel is in a break: the title is the programme it leads back to. */
  inBreak?: boolean;
  /** The channel is all breaks while its titles download: one plain sentence instead (#2727). */
  waiting?: string;
};

// What a channel's On now card says. Titles mean programmes, never filler: during a break the card
// names the programme the break leads back to with an Ad break badge (#2761), and a channel that
// is all breaks while its titles download says so in one sentence (#2727). A break with no
// programme after it has nothing to title, so that channel gets no card.
const onNowCard = (channel: GuideChannelLayout, nowMs: number, timezone?: string): OnNowCard | undefined => {
  const data = surfChannelData(channel, nowMs, timezone);
  const airing = channel.airings.find((a) => a.source.startMs <= nowMs && a.source.stopMs > nowMs)?.source;
  if (!data.now || !airing) return undefined;
  const identity = { channel, channelName: data.channelName, channelLogoUri: data.channelLogoUri };
  const waiting = guideWaitingForTitles(channel.source);
  if (waiting) return { ...identity, airing, waiting };
  if (airing.kind === "filler") {
    const programme = guideBreakProgramme(
      channel.airings.map((a) => a.source),
      airing,
    );
    if (!programme) return undefined;
    return {
      ...identity,
      airing: programme,
      title: programme.series ?? guideAiringLabel(programme),
      episodeLabel: formatGuideEpisode(programme.season, programme.episode),
      timeLabel: formatGuideTimeRange(programme.startMs, programme.stopMs, timezone),
      inBreak: true,
    };
  }
  return {
    ...identity,
    airing,
    title: data.now.seriesTitle ?? data.now.title,
    episodeLabel: data.now.episodeLabel,
    timeLabel: data.now.timeLabel,
  };
};

// OnNow — Home's "On now" (#1822 evidence): up to six currently scheduled live channels in
// channel-number order, from the same guide facts the Guide reads. A household with channels but
// nothing playing right now gets "Your channels" instead: actual lifecycle or schedule-gap
// wording and View channel, never a first-channel prompt — the channels already exist.
const OnNow = ({ layout, nowMs }: OnNowProps) => {
  const ordered = [...layout.channels].sort((a, b) => a.source.number - b.source.number);
  const onNow = ordered
    .map((channel) => onNowCard(channel, nowMs, layout.timezone))
    .filter((card): card is OnNowCard => card !== undefined)
    .slice(0, MAX_CHANNELS);

  if (onNow.length === 0) {
    return (
      <section aria-labelledby="home-on-now">
        <SectionHeader id="home-on-now" title="Your channels" />
        <ListGroup aria-labelledby="home-on-now">
          {ordered.map(({ source }) => {
            const { label, tone } = lifecycle(guideChannelState(source));
            return (
              <ListRow
                key={source.channelId}
                tone={tone}
                title={
                  <span className="flex items-center gap-2">
                    <span className="text-foreground font-mono">
                      {String(source.number).padStart(2, "0")}
                    </span>
                    {source.name}
                  </span>
                }
                sub={label}
                action={
                  <Link
                    to="/channels/$id"
                    params={{ id: source.channelId }}
                    className="text-static-400 hover:text-foreground text-sm"
                  >
                    View channel <span aria-hidden="true">→</span>
                  </Link>
                }
              />
            );
          })}
        </ListGroup>
      </section>
    );
  }

  return (
    <section aria-labelledby="home-on-now">
      <SectionHeader id="home-on-now" title="On now">
        <Action accessibilityRole="link" render={<Link to="/guide" />} variant="link">
          Full guide
        </Action>
      </SectionHeader>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {onNow.map(({ channel, channelName, airing, title, episodeLabel, timeLabel, inBreak, waiting }) => (
          <PictureCard
            key={channel.source.channelId}
            action={
              // Named for what it plays: six cards each say "Watch live", and the name still starts
              // with the visible label (WCAG 2.5.3).
              <CardAction
                accessibilityLabel={`Watch live: ${title ?? channelName} on ${channelName}`}
                accessibilityRole="link"
                render={<Link to="/watch/$id" params={{ id: channel.source.channelId }} />}
              >
                Watch live
              </CardAction>
            }
            chip={<ChannelChip name={channelName} number={String(channel.source.number).padStart(2, "0")} />}
            media={
              waiting ? (
                <ArtworkFallback channel={{ name: channelName, number: channel.source.number }} />
              ) : (
                <AiringArtwork
                  airing={airing}
                  channel={{ name: channelName, number: channel.source.number }}
                  sizes="(min-width: 1024px) 33vw, (min-width: 640px) 50vw, 100vw"
                />
              )
            }
            meta={
              waiting
                ? [waiting]
                : [
                    ...(inBreak ? [<CardStatus key="break" label="Ad break" tone="notice" />] : []),
                    ...(episodeLabel ? [episodeLabel] : []),
                    ...(timeLabel ? [timeLabel] : []),
                  ]
            }
            orientation="top"
            ratio="16/9"
            // A channel waiting for its titles has no programme to name, so the card is named for the
            // channel and says why in its one line (#2727).
            title={waiting ? channelName : (title ?? channelName)}
          />
        ))}
      </div>
    </section>
  );
};

export { AiringArtwork, OnNow };
