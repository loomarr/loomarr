import { Stack, Text, useReducedMotionPreference, VisuallyHidden } from "@loomarr/design-system";
import type { WatchHealth, WatchLineProps } from "./watch-line.type";

// WatchLine — the web mock's `watchLine` (#1659): a plain line under the Filler page's
// description, with a 7px status dot. The mock set it in mono; it is a sentence, and mono is for
// channel numbers and clocks (#2778), so it is in the body face (#3412). It is a line, not a bordered pill at the header's top
// right (§10 V38c): the web mock draws it unboxed under the description.
//
// ⚠ **The dot's colour follows the server's health verdict.** The web mock does the same (green and
// pulsing when healthy, caution and still when not). Before it did, a static mock drew the dot
// unconditionally green, and shipping that would have shown a healthy pulse on an install with every
// source switched off. The text says WHAT ("3 of 4 sources on"); the dot answers whether it is
// working right now. (Maintainer decision, 2026-08-02.)
const DOT: Record<WatchHealth, string> = {
  healthy: "var(--color-lock)",
  attention: "var(--color-caution)",
  unconfigured: "var(--color-static-500)",
};

// ⚠ Only `healthy` pulses. Motion here means "running", so animating the other two would say the
// opposite of what they mean — and a page with a permanently throbbing amber dot trains an
// operator to stop seeing it.
//
// ⚠ The reduced-motion preference gates the animation. Those readers get the colour and the
// hidden sentence, which carry the whole meaning without the movement.
const SR: Record<WatchHealth, string> = {
  healthy: "Watching your sources",
  attention: "Sources need attention",
  unconfigured: "No sources set up yet",
};

const WatchLine = ({ status, health, className }: WatchLineProps) => {
  const reducedMotion = useReducedMotionPreference();
  const pulse = health === "healthy" && reducedMotion === false;
  return (
    <Stack align="center" className={className} direction="row" gap="inline" marginTop={8} render="p">
      <span
        aria-hidden
        style={{
          backgroundColor: DOT[health],
          borderRadius: 999,
          flexShrink: 0,
          height: 7,
          width: 7,
          ...(pulse ? { animation: "onair-pulse 2s ease-in-out infinite" } : {}),
        }}
      />
      <Text as="span" minWidth={0} textRole="hint" tone="secondary">
        {status}
      </Text>
      {/* ⚠ The dot is `aria-hidden`, so its meaning has to be said in words or it is invisible to a
          screen reader — a colour with no text equivalent is exactly the failure axe cannot catch
          (it sees a decorated span, not a missing sentence). */}
      <VisuallyHidden as="span">{SR[health]}</VisuallyHidden>
    </Stack>
  );
};

export { WatchLine };
