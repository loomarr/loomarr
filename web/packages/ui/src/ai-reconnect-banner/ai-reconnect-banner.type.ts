type AiReconnectBannerProps = {
  /** How many titles the person has already picked by hand; the copy promises they stay. */
  pickCount: number;
  /** Resume into AI suggestions. The caller keeps the picks; this banner only asks. */
  onTryAi: () => void;
  /** Hide the banner and carry on by hand. */
  onKeepManual: () => void;
  /**
   * Why "Try AI suggestions" can't run yet (e.g. the channel has no name). When set, the action is
   * disabled and this reason is shown beneath it, so the control never fails silently.
   */
  tryAiBlockedReason?: string;
  /** True while the hand-off is in flight. */
  busy?: boolean;
};

export type { AiReconnectBannerProps };
