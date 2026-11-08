import type { GuideHighlightDTO } from "@loomarr/api/models/guideHighlightDTO";
import { GuideHighlightDTOReason } from "@loomarr/api/models/guideHighlightDTOReason";
import { formatGuideTime } from "@loomarr/core/guide";
import { Action, SectionHeader } from "@loomarr/design-system";
import { HighlightRow, ListGroup } from "@loomarr/ui";
import { Link } from "@tanstack/react-router";
import type { TonightProps } from "./tonight.type";

// Why a highlight is one, worded here from the server's typed reason (#1664): the server never
// sends a sentence, so the words can change without an API change. Q-H3 limits the reasons to
// premieres and marathons.
const highlightReason = (h: GuideHighlightDTO, timeZone?: string): string => {
  switch (h.reason) {
    case GuideHighlightDTOReason.series_premiere:
      return "Series premiere";
    case GuideHighlightDTOReason.season_premiere:
      return h.airing.season ? `Season ${h.airing.season} premiere` : "Season premiere";
    case GuideHighlightDTOReason.marathon:
      return h.untilMs ? `Back-to-back until ${formatGuideTime(h.untilMs, timeZone)}` : "Back-to-back";
  }
};

// Tonight — Home's highlights list (#1659 web mock): when, which channel, the show and its
// episode, and why it is worth knowing about. Each row opens that channel's Watch.
const Tonight = ({ highlights, timeZone }: TonightProps) => {
  if (highlights.length === 0) return null;
  return (
    <section aria-labelledby="home-tonight">
      <SectionHeader id="home-tonight" title="Tonight">
        <Action accessibilityRole="link" render={<Link to="/guide" />} variant="link">
          Full guide
        </Action>
      </SectionHeader>
      <ListGroup aria-labelledby="home-tonight">
        {highlights.map((h) => (
          <HighlightRow
            key={h.airing.scheduleBlockId}
            time={formatGuideTime(h.airing.startMs, timeZone)}
            channelNumber={String(h.channelNumber).padStart(2, "0")}
            title={h.airing.series ?? h.airing.title}
            detail={h.airing.series ? ` · ${h.airing.title}` : undefined}
            reason={highlightReason(h, timeZone)}
            render={<Link to="/watch/$id" params={{ id: h.channelId }} />}
          />
        ))}
      </ListGroup>
    </section>
  );
};

export { highlightReason, Tonight };
