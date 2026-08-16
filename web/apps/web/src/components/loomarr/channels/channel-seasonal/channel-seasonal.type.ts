import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
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
  className?: string;
}

export type { ChannelSeasonalProps };
