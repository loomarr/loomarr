import type { GuideLayout } from "@loomarr/core/guide";
import type { MyChannels } from "@loomarr/core/my-channels";

import type { GuideFilter, GuideFilterOption } from "../guide.type";

/**
 * The three filters with their counts, as the native mock writes them ("All · 54", "★ Favorites · 6",
 * "Recent · 4"). A count covers only channels the served guide holds. Without the person's lists the
 * two personal filters stay disabled; with them, a filter is disabled while it would be empty.
 */
const guideFilterOptions = (layout: GuideLayout, mine: MyChannels | undefined): GuideFilterOption[] => {
  const served = new Set(layout.source.channels.map((channel) => channel.channelId));
  const count = (ids: readonly string[]) => ids.filter((id) => served.has(id)).length;
  const favourites = mine ? count(mine.favouriteIds) : undefined;
  const recent = mine ? count(mine.recentIds) : undefined;
  return [
    { count: served.size, label: "All", value: "all" },
    { count: favourites, disabled: !favourites, label: "Favorites", value: "favourites" },
    { count: recent, disabled: !recent, label: "Recent", value: "recent" },
  ];
};

/** The channels a filter keeps, or undefined for all of them. */
const guideFilterChannelIds = (filter: GuideFilter, mine: MyChannels | undefined) =>
  filter === "all" || !mine ? undefined : filter === "favourites" ? mine.favouriteIds : mine.recentIds;

const guideFilterText = (option: GuideFilterOption) => ({
  accessibilityLabel:
    option.count === undefined ? `${option.label} channels` : `${option.label}, ${option.count} channels`,
  text: `${option.value === "favourites" ? "★ " : ""}${option.label}${option.count === undefined ? "" : ` · ${option.count}`}`,
});

export { guideFilterChannelIds, guideFilterOptions, guideFilterText };
