import type { RequestsController } from "@loomarr/core/requests";
import type { ReactNode } from "react";

import type { RequestChannelName } from "../request-done-row";

type RequestsJourneyProps = {
  /** The guide's channels, so a finished request's row names its channel by number and name. */
  channels?: readonly RequestChannelName[];
  controller: RequestsController;
  /** The host's tab bar, docked under the screen. */
  footer?: ReactNode;
  /** Wall-clock now, fixed in stories so holiday ideas read the same every run. */
  nowMs?: number;
  /** A finished request's channel, or the one an approval just built. The host owns navigation. */
  onOpenChannel: (channelId: string) => void;
};

export type { RequestsJourneyProps };
