import * as channelsApi from "@loomarr/api/endpoints/channels";
import { GuideAiringKind } from "@loomarr/api/models/guideAiringKind";
import { unwrap } from "@loomarr/api/unwrap";
import { Surface } from "@loomarr/design-system";
import { ArtworkFallback } from "@/components/loomarr/artwork-fallback";
import { ServedImage } from "@/components/loomarr/served-image";
import type { ChannelHeroProps } from "./channel-hero.type";

// ChannelHero — the picture channel detail opens on (#2753): the backdrop of what is on now (else
// what is on next), else its still, else generated channel art in the channel's own colour. The
// page used to open on text; this is the one that should make someone want to watch. Decorative
// (the programme's name sits right below it in What's on), so it carries no alt text.
const ChannelHero = ({ channelId, channel }: ChannelHeroProps) => {
  const timeline = channelsApi.useChannelTimeline(channelId, undefined, {
    query: { retry: false, refetchInterval: 60_000 },
  });
  const programmes = (unwrap(timeline.data, (b) => b.airings) ?? []).filter(
    (a) => a.kind === GuideAiringKind.program,
  );
  const nowMs = Date.now();
  const airing = programmes.find((a) => a.startMs <= nowMs && nowMs < a.stopMs) ?? programmes[0];
  const image = airing?.backdropImage ?? airing?.thumbImage;
  const fallback = <ArtworkFallback channel={channel} />;

  return (
    <Surface aspectRatio={21 / 9} data-channel-hero="" maxHeight={288} overflow="hidden" width="100%">
      {image ? (
        <ServedImage
          image={image}
          alt=""
          sizes="(min-width: 1024px) 48rem, 100vw"
          priority
          fallback={fallback}
        />
      ) : (
        fallback
      )}
    </Surface>
  );
};

export { ChannelHero };
