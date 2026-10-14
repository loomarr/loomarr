import type { ArtworkState, Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

import type { ChannelIdentityData, ProgrammeIdentityData } from "../identity";

interface ProgrammeCardData extends ChannelIdentityData, ProgrammeIdentityData {
  artworkState: ArtworkState;
  progressPercent?: number;
}

// Who is watching, for the overlay layout (Home's Watching now). Whether a name may be shown at
// all is the server's call (#1662, Q-H2); the card only draws what it is given.
interface ProgrammeCardViewer {
  name: string;
  initials: string;
  device?: string;
}

// `default` — the native card (TV, phone): padded, identity rows under the artwork.
// `stacked` — the web mock's On now card (#1659): the still with progress along its foot, then
//   channel, title and time left.
// `overlay` — the web mock's Watching now card: the text laid over the still's gradient.
type ProgrammeCardLayout = "default" | "stacked" | "overlay";

interface ProgrammeCardProps {
  artwork?: ReactNode;
  channelLogo?: ReactNode;
  density?: Density;
  focused?: boolean;
  // Outlines the card in the focus amber: the viewer's own card in Watching now.
  highlighted?: boolean;
  // Outlines the card in the focus amber under the pointer, for a card that is a link (the web
  // mock's hover on Watching now). Keyboard focus keeps the link's own ring.
  hoverHighlight?: boolean;
  layout?: ProgrammeCardLayout;
  programme: ProgrammeCardData;
  viewer?: ProgrammeCardViewer;
}

export type { ProgrammeCardData, ProgrammeCardLayout, ProgrammeCardProps, ProgrammeCardViewer };
