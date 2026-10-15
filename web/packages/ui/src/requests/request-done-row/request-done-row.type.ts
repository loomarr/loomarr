import type { RequestEntry } from "@loomarr/core/requests";

/** The part of a guide channel a Done row names: the number and name the viewer knows it by. */
type RequestChannelName = { id: string; name: string; number: number };

type RequestDoneRowProps = {
  /** The finished request's channel, when the guide has it; without it the row names the request instead. */
  channel?: RequestChannelName;
  entry: RequestEntry;
  /** Wall-clock now for the relative date; fixed in stories and tests. */
  nowMs?: number;
  /** Tunes the channel and opens Watching. */
  onOpen: () => void;
};

export type { RequestChannelName, RequestDoneRowProps };
