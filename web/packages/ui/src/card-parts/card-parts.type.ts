import type { ActionProps, StatusTone } from "@loomarr/design-system";

interface CardActionProps extends Omit<ActionProps, "variant"> {
  /** The item needs the viewer: the action is drawn as a secondary button instead of a quiet one. */
  attention?: boolean;
}

interface CardStatusProps {
  label: string;
  tone: StatusTone;
}

interface ChannelChipProps {
  name: string;
  number: string;
}

export type { CardActionProps, CardStatusProps, ChannelChipProps };
