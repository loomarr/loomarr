import { channelIdentHue, monogramOf } from "@loomarr/core/guide";
import { CHANNEL_IDENT_WASH_PERCENT, Stack, Text } from "@loomarr/design-system";
import { onTintVar } from "@loomarr/design-system/web-theme";
import type { ChannelIdentProps } from "./channel-ident.type";

// ChannelIdent — a channel's mark in the guide rail (§12, the v2 mock's rail icon).
//
// A logo when the channel has one, otherwise a derived monogram. Deriving rather than leaving
// a blank is the point: a rail of unlabelled rows is much harder to scan than one with even a
// crude mark, and most channels never get a logo uploaded.
//
// The mock hardcoded per-channel monograms and colours. Those are DERIVED here, because a
// fixed table cannot cover channels the operator creates — and a channel that fell off the
// table would render blank, which is exactly the case the fallback exists for.

// The letters and the hue come from `@loomarr/core/guide` (#1659, N8), so the shared guide grid
// and native draw the same mark. Colour is keyed on the channel NUMBER, so renaming a channel
// does not change how the rail looks.

const ChannelIdent = ({ name, number, logo, size = 30, className }: ChannelIdentProps) => {
  if (logo) {
    return (
      <Stack
        aria-label={`${name} logo`}
        backgroundColor="$surfaceCanvas"
        borderColor="borderDecorative"
        borderRadius={5}
        borderWidth={1}
        className={className}
        flexShrink={0}
        role="img"
        style={{
          backgroundImage: `url(${JSON.stringify(logo)})`,
          backgroundPosition: "center",
          backgroundSize: "cover",
          height: size,
          width: size,
        }}
      />
    );
  }

  const hue = channelIdentHue(number);
  const color = `var(--color-${hue})`;
  return (
    <Stack
      align="center"
      aria-hidden
      borderRadius={5}
      borderWidth={1}
      className={className}
      flexShrink={0}
      justify="center"
      style={{
        width: size,
        height: size,
        // The 14% wash of the hue (#3011). The letters take the hue's on-tint stop (its `-300`
        // where the full hue fails AA on its own wash), which is what lets the wash be this strong:
        // the full pink and red fell below 4.5:1 here (#2953). The design system's contrast test
        // proves every hue on the canvas and a raised card.
        backgroundColor: `color-mix(in srgb, ${color} ${CHANNEL_IDENT_WASH_PERCENT}%, transparent)`,
        borderColor: `color-mix(in srgb, ${color} 30%, transparent)`,
      }}
    >
      <Text
        as="span"
        strong
        style={{ color: onTintVar(hue), fontSize: Math.round(size * 0.37) }}
        textRole="mono"
        tracking={0.02}
      >
        {monogramOf(name)}
      </Text>
    </Stack>
  );
};

export { ChannelIdent, monogramOf };
