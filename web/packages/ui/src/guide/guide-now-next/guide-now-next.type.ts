import type { GuideLayout } from "@loomarr/core/guide";
import type { ReactNode } from "react";

interface GuideNowNextProps {
  layout: GuideLayout;
  /** The clock the layout was computed against: what is on now, and how far through it is. */
  nowMs: number;
  /** A row was tapped: tune its channel. */
  onWatch: (channelId: string) => void;
}

interface GuidePhoneHeaderProps {
  /** The view switch, right of the title. */
  children?: ReactNode;
  subtitle: string;
  title: string;
}

export type { GuideNowNextProps, GuidePhoneHeaderProps };
