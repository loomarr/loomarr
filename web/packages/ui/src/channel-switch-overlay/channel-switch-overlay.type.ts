import type { SurfChannelCardData } from "../surf-rail/surf-channel-card";

interface ChannelSwitchOverlayProps {
  /** Distance from the bottom edge to the card's underside; lifts it clear of the bottom chrome bar. */
  bottomInset?: number;
  channel: SurfChannelCardData;
  /** Signed still of the channel being tuned. Absent or failed: the card sits on the plain background. */
  stillUri?: string;
  /** True from the key press until the first frame decodes; false starts the fade-out. */
  visible: boolean;
}

export type { ChannelSwitchOverlayProps };
