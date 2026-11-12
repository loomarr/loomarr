import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import type { GuideHighlightDTO } from "@loomarr/api/models/guideHighlightDTO";
import { GuideHighlightDTOReason } from "@loomarr/api/models/guideHighlightDTOReason";
import { formatGuideTime, formatProgrammeTitle } from "@loomarr/core/guide";
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

interface NextProgramme {
  airing: GuideAiring;
  channelId: string;
  channelNumber: number;
}

// Each channel's next real programme after now (#2763), soonest first. Filler is not a programme,
// so a channel with only filler ahead has no row.
const nextProgrammes = (channels: readonly GuideChannelTimeline[], nowMs: number): NextProgramme[] =>
  channels
    .flatMap((channel) => {
      const next = channel.airings
        .filter((airing) => airing.kind === "program" && airing.startMs > nowMs)
        .sort((a, b) => a.startMs - b.startMs)[0];
      return next ? [{ airing: next, channelId: channel.channelId, channelNumber: channel.number }] : [];
    })
    .sort((a, b) => a.airing.startMs - b.airing.startMs || a.channelNumber - b.channelNumber);

// Tonight — Home's list of what is on next (#1659 web mock): when, which channel, the show and its
// episode, and, when it is a premiere or a marathon, why that is worth knowing. Each row opens the
// guide's grid at that time with the programme selected (#2763): it hasn't started, so Watch would
// show something else.
const Tonight = ({ channels, highlights, nowMs, timeZone }: TonightProps) => {
  const rows = nextProgrammes(channels, nowMs);
  if (rows.length === 0) return null;
  const reasons = new Map(highlights.map((h) => [h.airing.scheduleBlockId, highlightReason(h, timeZone)]));
  return (
    <section aria-labelledby="home-tonight">
      <SectionHeader id="home-tonight" title="Tonight">
        <Action accessibilityRole="link" render={<Link to="/guide" />} variant="ghost">
          Full guide
        </Action>
      </SectionHeader>
      <ListGroup aria-labelledby="home-tonight">
        {rows.map(({ airing, channelId, channelNumber }) => (
          <HighlightRow
            key={airing.scheduleBlockId}
            time={formatGuideTime(airing.startMs, timeZone)}
            channelNumber={String(channelNumber).padStart(2, "0")}
            title={formatProgrammeTitle(airing)}
            reason={reasons.get(airing.scheduleBlockId)}
            render={<Link to="/guide" search={{ view: "grid", at: airing.startMs, channel: channelId }} />}
          />
        ))}
      </ListGroup>
    </section>
  );
};

export { highlightReason, nextProgrammes, Tonight };
