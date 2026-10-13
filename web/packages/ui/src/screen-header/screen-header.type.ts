import type { IconName } from "@loomarr/design-system";

type ScreenHeaderAction = {
  accessibilityLabel: string;
  icon: IconName;
  onPress: () => void;
};

type ScreenHeaderProps = {
  /** At most one plain icon button at the trailing edge (#1840's Requests "+"). */
  action?: ScreenHeaderAction;
  title: string;
};

export type { ScreenHeaderAction, ScreenHeaderProps };
