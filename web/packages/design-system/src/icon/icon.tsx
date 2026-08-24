import { useTheme, View } from "@tamagui/core";
import type { LucideIcon } from "lucide-react-native";

import { iconography } from "../tokens";

type IconSize = keyof typeof iconography.size;
type IconTone = "danger" | "disabled" | "info" | "muted" | "primary" | "secondary" | "success" | "warning";
type AccessibleIcon = { accessibilityLabel: string; decorative?: false };
type DecorativeIcon = { accessibilityLabel?: never; decorative: true };

type IconProps = (AccessibleIcon | DecorativeIcon) & {
  glyph: LucideIcon;
  size?: IconSize;
  tone?: IconTone;
};

const Icon = ({
  accessibilityLabel,
  decorative,
  glyph: Glyph,
  size = "default",
  tone = "secondary",
}: IconProps) => {
  const theme = useTheme();
  const tones: Record<IconTone, string> = {
    danger: theme.stateDanger.val,
    disabled: theme.actionDisabled.val,
    info: theme.stateInfo.val,
    muted: theme.contentMuted.val,
    primary: theme.actionPrimary.val,
    secondary: theme.contentSecondary.val,
    success: theme.stateSuccess.val,
    warning: theme.stateWarning.val,
  };

  const glyph = (
    <Glyph
      aria-hidden
      color={tones[tone]}
      size={iconography.size[size]}
      strokeWidth={iconography.strokeWidth}
    />
  );

  if (decorative) return glyph;

  const pixels = iconography.size[size];
  return (
    <View
      aria-label={accessibilityLabel}
      role="img"
      alignItems="center"
      height={pixels}
      justifyContent="center"
      width={pixels}
    >
      {glyph}
    </View>
  );
};

export type { IconProps, IconSize, IconTone };
export { Icon };
