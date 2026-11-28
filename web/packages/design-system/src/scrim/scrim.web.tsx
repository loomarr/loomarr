import { withAlpha } from "../text";
import { playerChrome } from "../tokens";
import type { ScrimProps } from "./scrim.type";

// The web adapter: a CSS gradient, masked at its top and bottom where it softens.
const Scrim = ({ feather, from, soften = 0, strength = 0.95, testID }: ScrimProps) => {
  const mask = `linear-gradient(transparent, black ${soften}px, black calc(100% - ${soften}px), transparent)`;
  return (
    <div
      aria-hidden
      data-testid={testID}
      style={{
        backgroundImage: `linear-gradient(to ${from}, ${withAlpha(playerChrome.surface, strength)} calc(100% - ${feather}px), transparent)`,
        bottom: -soften,
        left: 0,
        maskImage: soften > 0 ? mask : undefined,
        pointerEvents: "none",
        position: "absolute",
        right: 0,
        top: -soften,
      }}
    />
  );
};

export { Scrim };
