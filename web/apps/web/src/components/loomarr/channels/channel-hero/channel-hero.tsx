import * as channelsApi from "@loomarr/api/endpoints/channels";
import { GuideAiringKind } from "@loomarr/api/models/guideAiringKind";
import { unwrap } from "@loomarr/api/unwrap";
import { Surface } from "@loomarr/design-system";
import { ArtworkFallback } from "@/components/loomarr/artwork-fallback";
import { ServedImage } from "@/components/loomarr/served-image";
import type { ChannelHeroProps } from "./channel-hero.type";

// ChannelHero — the picture channel detail opens on (#2753): the backdrop of what is on now (else
// what is on next), else its still. Decorative (the programme's name sits right below it in What's
// on), so it carries no alt text.
//
// A channel with no picture has no hero (#3413 decision 9): the page starts at the "On air" card
// and the header's channel icon is the only identity. A tall panel holding two letters is not a
// reason to watch. While the timeline loads the box holds its place, so a channel that has a
// picture does not push the page down when it arrives.
//
// The box takes the artwork's own shape: backdrops and stills are 16:9, so a 16:9 box up to the
// 48rem the `sizes` hint asks for shows the whole picture, and `cover` only trims an odd-shaped
// still. The timeline refetches on the `channel` SSE frame (its key sits under `/v1/channels`).
const HERO_MAX_WIDTH = 768;

const ChannelHero = ({ channelId, channel }: ChannelHeroProps) => {
  const timeline = channelsApi.useChannelTimeline(channelId, undefined, {
    query: { retry: false },
  });
  const programmes = (unwrap(timeline.data, (b) => b.airings) ?? []).filter(
    (a) => a.kind === GuideAiringKind.program,
  );
  const nowMs = Date.now();
  const airing = programmes.find((a) => a.startMs <= nowMs && nowMs < a.stopMs) ?? programmes[0];
  const image = airing?.backdropImage ?? airing?.thumbImage;
  if (!image && !timeline.isPending) return null;

  return (
    <Surface
      aspectRatio={16 / 9}
      data-channel-hero=""
      maxWidth={HERO_MAX_WIDTH}
      overflow="hidden"
      width="100%"
    >
      {image ? (
        <ServedImage
          image={image}
          alt=""
          sizes="(min-width: 1024px) 48rem, 100vw"
          priority
          // A picture that fails to load falls to the channel's generated art, not a broken image.
          fallback={<ArtworkFallback channel={channel} />}
        />
      ) : null}
    </Surface>
  );
};

export { ChannelHero };
