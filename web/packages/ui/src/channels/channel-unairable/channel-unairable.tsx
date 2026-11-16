import { Notice } from "@loomarr/design-system";

import type { ChannelUnairableProps, UnairableReason } from "./channel-unairable.type";

// What each reason means, finishing a sentence that starts with the title.
const because: Record<UnairableReason, string> = {
  no_episodes_in_seasons: "has no episodes in the seasons this channel asks for",
  no_episodes_selected: "has no episodes left after the episode choice",
  not_in_library: "isn't in your library",
  out_of_scope: "is outside this channel's years, genres or length",
  over_ceiling: "is rated above this channel's audience limit",
  unrated: "has no rating, and this channel only airs rated titles",
};

/**
 * The lineup titles that never air, and why (#4140): a title missing from the library, asking for
 * seasons the library doesn't hold, or refused by the channel's own limits. Without it a title sits
 * in the lineup and simply never comes on.
 */
const ChannelUnairable = ({ titles }: ChannelUnairableProps) => {
  if (titles.length === 0) return null;
  const count = titles.length === 1 ? "1 title can't air" : `${titles.length} titles can't air`;
  const body = titles.map((t) => `${t.title} ${because[t.reason] ?? "can't air"}.`).join(" ");
  return <Notice body={body} title={count} tone="warn" />;
};

export { ChannelUnairable };
