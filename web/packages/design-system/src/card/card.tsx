import { styled, View } from "@tamagui/core";

import { Surface } from "../primitives";

/**
 * The legacy Web Card's composable slots (#970 PR B checkpoint 2), built on `Surface` rather than
 * a fresh box: a card IS a raised surface, just one with a conventional header/title/content/
 * footer layout baked in. `Card`'s own ref, like every `styled()` component here, forwards to the
 * host element Tamagui mounts — on web, that is a plain `HTMLDivElement`.
 *
 * The legacy shadcn Card used `rounded-lg` (12px in this app's own Tailwind radius scale), which
 * is `$cardCompact`, not `$card` (16px) — `Surface`'s own default would be visibly larger corners.
 */
const Card = styled(Surface, {
  name: "LoomarrCard",
  borderRadius: "$cardCompact",
});

// 16px vertical rhythm with no gap below the last row — the legacy shadcn CardHeader's
// `flex flex-col gap-1.5 p-4` (gap-1.5 is 6px at this app's base font size).
const CardHeader = styled(View, {
  name: "LoomarrCardHeader",
  flexDirection: "column",
  gap: 6,
  padding: 16,
});

const CardTitle = styled(View, {
  name: "LoomarrCardTitle",
  // The legacy `font-semibold leading-none tracking-tight` is a text treatment, but the slot
  // itself stays a plain container: callers put a `Text` (or any content) inside it, the same
  // way the legacy `<CardTitle>` was a styled `<div>`, not a styled heading element.
});

// `p-4 pt-0`: full padding except the top, which the header above already carries.
const CardContent = styled(View, {
  name: "LoomarrCardContent",
  padding: 16,
  paddingTop: 0,
});

// `flex items-center p-4 pt-0`.
const CardFooter = styled(View, {
  name: "LoomarrCardFooter",
  alignItems: "center",
  flexDirection: "row",
  padding: 16,
  paddingTop: 0,
});

export { Card, CardContent, CardFooter, CardHeader, CardTitle };
