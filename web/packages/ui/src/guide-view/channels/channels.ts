import { formatProgrammeTitle } from "@loomarr/core/guide";
import type { GuideViewChannel } from "@loomarr/core/guide-view";

import { groupSeriesRuns, type StripAiring } from "../../film-strip";
import type { GuideViewEntry } from "../guide-view.type";

const titleOf = (airing: StripAiring) => airing.series?.trim() || formatProgrammeTitle(airing);

/**
 * Lineup channels with their rows' programmes, from each channel's airings in the guide read. The
 * read carries no episode count per run, so runs are grouped and counted here.
 */
const guideViewChannels = <
  C extends Omit<GuideViewChannel, "programmes"> & { airings: readonly StripAiring[] },
>(
  channels: readonly C[],
): (C & GuideViewEntry)[] =>
  channels.map((channel) => ({
    ...channel,
    programmes: groupSeriesRuns(channel.airings).flatMap((item) =>
      item.type === "programme" ? [{ ...item, title: titleOf(item.airings[0] as StripAiring) }] : [],
    ),
  }));

export { guideViewChannels };
