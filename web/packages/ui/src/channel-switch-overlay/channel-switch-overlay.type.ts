interface ChannelSwitchOverlayProps {
  /** The channel being tuned, for the readout's channel line ("CH 7" then the name). */
  channel: { channelName: string; channelNumber: string };
  /** Overrides the platform's reduced-motion setting (stories and tests). */
  reducedMotion?: boolean;
  /** Signed still of the channel being tuned. Absent or failed: the snow sits on the plain ground. */
  stillUri?: string;
  /** True from the key press until the first frame decodes; false starts the fade-out. */
  visible: boolean;
}

export type { ChannelSwitchOverlayProps };
