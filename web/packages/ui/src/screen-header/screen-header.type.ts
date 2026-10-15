import type { IconName } from "@loomarr/design-system";

type ScreenHeaderAction = {
  accessibilityLabel: string;
  icon: IconName;
  onPress: () => void;
};

type ScreenHeaderProps = {
  /**
   * Up to two plain icon buttons at the trailing edge, in reading order, the last one flush with the
   * gutter: Requests' "+" (#1840) and the Device sheet's account button.
   */
  actions?: readonly [ScreenHeaderAction] | readonly [ScreenHeaderAction, ScreenHeaderAction];
  title: string;
};

export type { ScreenHeaderAction, ScreenHeaderProps };
