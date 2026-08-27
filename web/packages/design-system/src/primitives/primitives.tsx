import { isWeb, styled, Text as TamaguiText, View } from "@tamagui/core";
import type { ComponentProps, ReactNode } from "react";

import { type Density, type TextRole, typography } from "../tokens";
import { resolveViewportInsets, useViewportInsets } from "../viewport";

const ScreenFrame = styled(View, {
  name: "LoomarrScreen",
  flex: 1,
  width: "100%",
  minHeight: isWeb ? "100vh" : "100%",
  backgroundColor: "$surfaceCanvas",
});

type ScreenFrameProps = ComponentProps<typeof ScreenFrame>;
type ScreenProps = Omit<
  ScreenFrameProps,
  | "padding"
  | "paddingBlock"
  | "paddingBlockEnd"
  | "paddingBlockStart"
  | "paddingBottom"
  | "paddingEnd"
  | "paddingHorizontal"
  | "paddingInline"
  | "paddingInlineEnd"
  | "paddingInlineStart"
  | "paddingLeft"
  | "paddingRight"
  | "paddingStart"
  | "paddingTop"
  | "paddingVertical"
> & { density?: Density };

/**
 * Edge-to-edge application frame whose content always stays inside platform insets and the
 * distance-appropriate Loomarr gutter. Insets are supplied once by LoomarrProvider.
 */
const Screen = ({ density = "pointer", ...props }: ScreenProps) => {
  const platformInsets = useViewportInsets();
  const insets = resolveViewportInsets(density, platformInsets);
  return (
    <ScreenFrame
      {...props}
      paddingBottom={insets.bottom}
      paddingLeft={insets.left}
      paddingRight={insets.right}
      paddingTop={insets.top}
    />
  );
};

const Surface = styled(View, {
  name: "LoomarrSurface",
  borderColor: "$borderDecorative",
  borderWidth: 1,
  borderRadius: "$card",
  variants: {
    level: {
      canvas: { backgroundColor: "$surfaceCanvas" },
      raised: { backgroundColor: "$surfaceRaised" },
      elevated: { backgroundColor: "$surfaceElevated" },
      overlay: { backgroundColor: "$surfaceOverlay", borderRadius: "$overlay" },
      focus: { backgroundColor: "$surfaceFocus", borderColor: "$actionFocus" },
    },
  } as const,
  defaultVariants: {
    level: "raised",
  },
});

const FocusSurface = styled(Surface, {
  name: "LoomarrFocusSurface",
  borderWidth: 2,
  borderColor: "$transparent",
  variants: {
    focused: {
      false: {
        backgroundColor: "$surfaceRaised",
        borderColor: "$transparent",
      },
      true: {
        backgroundColor: "$surfaceFocus",
        borderColor: "$actionFocus",
      },
    },
  } as const,
  defaultVariants: {
    focused: false,
  },
});

type TamaguiTextProps = ComponentProps<typeof TamaguiText>;
type TextTone =
  | "danger"
  | "info"
  | "inverse"
  | "live"
  | "muted"
  | "primary"
  | "secondary"
  | "success"
  | "warning";
type TextProps = Omit<
  TamaguiTextProps,
  "color" | "fontFamily" | "fontSize" | "fontWeight" | "letterSpacing" | "lineHeight" | "role"
> & {
  density?: Density;
  textRole: TextRole;
  tone?: TextTone;
};

const dataRoles = new Set<TextRole>(["metadata", "time", "channelNumber"]);
const mutedRoles = new Set<TextRole>(["label", "metadata", "time"]);
const textTones = {
  danger: "$stateDanger",
  info: "$stateInfo",
  inverse: "$contentInverse",
  live: "$stateLive",
  muted: "$contentMuted",
  primary: "$contentPrimary",
  secondary: "$contentSecondary",
  success: "$stateSuccess",
  warning: "$stateWarning",
} as const;

type TypographyWeight = (typeof typography)[Density][TextRole]["weight"];

/** Keep variable-font weights on web while using weights native system fonts accept. */
const resolveFontWeight = (weight: TypographyWeight, web = isWeb) => {
  if (web) return weight;
  if (weight === "550") return "600";
  if (weight === "650") return "700";
  return weight;
};

const Text = ({ density = "pointer", textRole, tone, ...props }: TextProps) => {
  const value = typography[density][textRole];
  return (
    <TamaguiText
      {...props}
      color={
        tone
          ? textTones[tone]
          : textRole === "channelNumber"
            ? "$actionPrimary"
            : mutedRoles.has(textRole)
              ? "$contentSecondary"
              : "$contentPrimary"
      }
      fontFamily={dataRoles.has(textRole) ? "$data" : "$body"}
      fontSize={value.size}
      fontWeight={resolveFontWeight(value.weight)}
      letterSpacing={textRole === "display" ? -0.8 : textRole === "title" ? -0.25 : 0}
      lineHeight={value.lineHeight}
    />
  );
};

type BadgeTone = "danger" | "info" | "live" | "neutral" | "success" | "warning";
type BadgeProps = Omit<ComponentProps<typeof View>, "children"> & {
  children: ReactNode;
  density?: Density;
  tone?: BadgeTone;
};

const badgeTone = {
  danger: { backgroundColor: "$stateDangerSurface", borderColor: "$stateDanger", color: "$stateDanger" },
  info: { backgroundColor: "$stateInfoSurface", borderColor: "$stateInfo", color: "$stateInfo" },
  live: { backgroundColor: "$stateLiveSurface", borderColor: "$stateLive", color: "$stateLive" },
  neutral: {
    backgroundColor: "$surfaceElevated",
    borderColor: "$actionDisabled",
    color: "$contentSecondary",
  },
  success: {
    backgroundColor: "$stateSuccessSurface",
    borderColor: "$stateSuccess",
    color: "$stateSuccess",
  },
  warning: {
    backgroundColor: "$stateWarningSurface",
    borderColor: "$stateWarning",
    color: "$stateWarning",
  },
} as const;

const Badge = ({ children, density = "pointer", tone = "neutral", ...props }: BadgeProps) => {
  const colors = badgeTone[tone];
  return (
    <View
      {...props}
      alignItems="center"
      alignSelf="flex-start"
      backgroundColor={colors.backgroundColor}
      borderColor={colors.borderColor}
      borderRadius="$round"
      borderWidth={1}
      paddingHorizontal="$inline"
      paddingVertical={4}
    >
      <TamaguiText
        color={colors.color}
        fontFamily="$data"
        fontSize={typography[density].metadata.size}
        fontWeight="600"
        lineHeight={typography[density].metadata.lineHeight}
      >
        {children}
      </TamaguiText>
    </View>
  );
};

type ArtworkState = "error" | "loading" | "missing" | "ready";
type ArtworkFrameProps = Omit<ComponentProps<typeof Surface>, "children"> & {
  children?: ReactNode;
  density?: Density;
  state: ArtworkState;
};

const artworkCopy: Record<Exclude<ArtworkState, "ready">, string> = {
  error: "Artwork unavailable",
  loading: "Loading artwork",
  missing: "No artwork",
};

const ArtworkFrame = ({ children, density = "pointer", state, ...props }: ArtworkFrameProps) => (
  <Surface
    {...props}
    alignItems="center"
    aspectRatio={16 / 9}
    backgroundColor="$artworkPlaceholder"
    justifyContent="center"
    overflow="hidden"
  >
    {state === "ready" && children ? (
      children
    ) : (
      <Text density={density} textRole="metadata">
        {artworkCopy[state === "ready" ? "missing" : state]}
      </Text>
    )}
  </Surface>
);

type ProgressTrackProps = Omit<ComponentProps<typeof View>, "children"> & {
  percent: number;
  tone?: "live" | "primary";
};

const ProgressTrack = ({ percent, tone = "primary", ...props }: ProgressTrackProps) => {
  const bounded = Math.max(0, Math.min(100, percent));
  const accessibilityProps = isWeb
    ? { "aria-valuemax": 100, "aria-valuemin": 0, "aria-valuenow": bounded, role: "progressbar" as const }
    : { accessibilityRole: "progressbar" as const, accessibilityValue: { max: 100, min: 0, now: bounded } };
  return (
    <View
      {...props}
      {...accessibilityProps}
      backgroundColor="$surfaceCanvas"
      borderRadius="$round"
      height={4}
      overflow="hidden"
    >
      <View
        backgroundColor={tone === "live" ? "$stateLive" : "$actionPrimary"}
        borderRadius="$round"
        height="100%"
        width={`${bounded}%`}
      />
    </View>
  );
};

export type { ArtworkState, BadgeTone, ScreenProps, TextProps, TextTone };
export { ArtworkFrame, Badge, FocusSurface, ProgressTrack, resolveFontWeight, Screen, Surface, Text };
