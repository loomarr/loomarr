import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { HolidayEpisodesDTO } from "@loomarr/api/models/holidayEpisodesDTO";
import type { Vocabulary } from "@loomarr/api/models/vocabulary";

interface ChannelSeasonalProps {
  policy: ChannelPolicy;
  // Controlled, like ChannelPolicyFields: the parent owns the policy and persists it.
  onChange: (next: ChannelPolicy) => void;
  // The BE-authored rule vocabulary (§6.6). The holiday list is read from its `when` tokens
  // (`holiday:<id>`) rather than hand-mirrored here — `BuildVocabulary` lowers every token
  // through `LowerWhen` → `knownHoliday` → `builtinCalendar`, so a holiday the engine does not
  // know cannot appear in this picker, and one it adds appears without a frontend change.
  vocabulary: Vocabulary;
  // The saved cycle preview's holiday-episode summary (§6 "Holiday episodes"), soonest holiday
  // first. The consequence line under Holiday strength reads the first; absent while it loads.
  holidayEpisodes?: HolidayEpisodesDTO[];
}

export type { ChannelSeasonalProps };
