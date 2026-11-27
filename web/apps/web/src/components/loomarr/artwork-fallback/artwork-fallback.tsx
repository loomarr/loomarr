import { channelIdentHue, monogramOf } from "@loomarr/core/guide";
import { Stack, Text } from "@loomarr/design-system";
import { onTintVar } from "@loomarr/design-system/web-theme";
import type { ArtworkFallbackProps } from "./artwork-fallback.type";

// ArtworkFallback — generated art for a slot with no picture (#2762): a soft wash in the channel's
// own identity hue with its monogram set large, so a card with no still still looks finished. Keeps
// the caller's own geometry (16:9 channel art, 2:3 posters). No hatching and never a blank box, and
// never AI art: it is only the channel's hue and letters. A title with no channel sets its own
// letters, in a hue picked from `seed`. The letters take the hue's on-tint stop, as ChannelIdent's
// do (#3011), so they hold large-text contrast even where the wash is strongest.
const ArtworkFallback = ({ channel, label, seed = 0 }: ArtworkFallbackProps) => {
  const hue = channelIdentHue(channel?.number ?? seed);
  const wash = `var(--color-${hue})`;
  const words = channel?.name ?? label;
  return (
    <Stack
      align="center"
      backgroundColor="$surfaceRaised"
      height="100%"
      justify="center"
      role="presentation"
      style={{
        backgroundImage: `linear-gradient(155deg, color-mix(in srgb, ${wash} 34%, var(--color-static-900)) 0%, var(--color-static-900) 88%)`,
      }}
      width="100%"
    >
      {words && (
        <Text as="span" style={{ color: onTintVar(hue) }} textRole="monogram">
          {monogramOf(words)}
        </Text>
      )}
    </Stack>
  );
};

export { ArtworkFallback };
