import { isWeb, styled, Text as TamaguiText, useTheme, View } from "@tamagui/core";
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
> & {
  density?: Density;
  /**
   * Chrome docked under the content, edge to edge (a phone's TabBar). It carries the bottom inset
   * itself, so the content stops above it with no bottom gutter.
   */
  footer?: ReactNode;
};

/**
 * Edge-to-edge application frame whose content always stays inside platform insets and the
 * distance-appropriate Loomarr gutter. Insets are supplied once by LoomarrProvider.
 */
const Screen = ({ density = "pointer", footer, ...props }: ScreenProps) => {
  const platformInsets = useViewportInsets();
  const insets = resolveViewportInsets(density, platformInsets);
  if (footer)
    return (
      <ScreenFrame>
        <ScreenFrame
          {...props}
          flex={1}
          minHeight={0}
          paddingBottom={0}
          paddingLeft={insets.left}
          paddingRight={insets.right}
          paddingTop={insets.top}
        />
        {footer}
      </ScreenFrame>
    );
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
  | "signal"
  | "success"
  | "warning";
type TextProps = Omit<
  TamaguiTextProps,
  "color" | "fontFamily" | "fontSize" | "fontWeight" | "letterSpacing" | "lineHeight" | "role"
> & {
  density?: Density;
  /**
   * A readout over snow or a picture (#1627 B4): the canvas as a two-layer halo, tight under the
   * glyphs and wide around them, so the text holds on any ground. React Native takes one text
   * shadow, so the wide layer is a second, hidden copy of the text behind the first.
   */
  halo?: boolean;
  textRole: TextRole;
  tone?: TextTone;
  /** Letter spacing in em, for tracked-out mono readouts ("CH 7" at 0.16, "TUNING IN" at 0.24). Overrides the role's own. */
  tracking?: number;
};

// "#0B0C0E" + 0.9 → "rgba(11,12,14,0.9)": the halo is the theme's canvas, part-transparent.
const withAlpha = (hex: string, alpha: number) => {
  const n = Number.parseInt(hex.slice(1, 7), 16);
  return `rgba(${(n >> 16) & 255},${(n >> 8) & 255},${n & 255},${alpha})`;
};

const dataRoles = new Set<TextRole>([
  "metadata",
  "time",
  "data",
  "channelNumber",
  "code",
  "cardChannelNumber",
  "cardTime",
  "cardInitials",
  "guideMeta",
  "guideLabel",
  "guideNumber",
  "guideIdent",
  "guideNow",
]);
const mutedRoles = new Set<TextRole>([
  "label",
  "section",
  "metadata",
  "time",
  "cardMeta",
  "cardTime",
  "guideMeta",
  "guideLabel",
]);
const amberRoles = new Set<TextRole>(["channelNumber", "code", "cardChannelNumber", "guideNumber"]);
const textTones = {
  danger: "$stateDanger",
  info: "$stateInfo",
  inverse: "$contentInverse",
  live: "$stateLive",
  muted: "$contentMuted",
  primary: "$contentPrimary",
  secondary: "$contentSecondary",
  signal: "$actionPrimary",
  success: "$stateSuccess",
  warning: "$stateWarning",
} as const;

const roleTracking = (textRole: TextRole, size: number) =>
  textRole === "display"
    ? -0.8
    : textRole === "title"
      ? -0.25
      : textRole === "section"
        ? 2
        : textRole === "guideLabel"
          ? size * 0.04
          : 0;

const Text = ({ density = "pointer", halo, textRole, tone, tracking, ...props }: TextProps) => {
  const theme = useTheme();
  const value = typography[density][textRole];
  const face = {
    color: tone
      ? textTones[tone]
      : amberRoles.has(textRole)
        ? "$actionPrimary"
        : mutedRoles.has(textRole)
          ? "$contentSecondary"
          : "$contentPrimary",
    fontFamily: dataRoles.has(textRole) ? "$data" : "$body",
    fontSize: value.size,
    fontWeight: value.weight,
    letterSpacing: tracking === undefined ? roleTracking(textRole, value.size) : value.size * tracking,
    lineHeight: value.lineHeight,
  } as const;
  if (!halo) return <TamaguiText {...props} {...face} />;

  const canvas = theme.surfaceCanvas.val;
  return (
    <View position="relative">
      {/* The wide layer: the same glyphs, hidden, glowing canvas around the text. */}
      <TamaguiText
        {...face}
        aria-hidden
        numberOfLines={props.numberOfLines}
        position="absolute"
        top={0}
        left={0}
        right={0}
        textShadowColor={withAlpha(canvas, 0.85)}
        textShadowOffset={{ height: 0, width: 0 }}
        textShadowRadius={18}
      >
        {props.children}
      </TamaguiText>
      <TamaguiText
        {...props}
        {...face}
        textShadowColor={withAlpha(canvas, 0.9)}
        textShadowOffset={{ height: 1, width: 0 }}
        textShadowRadius={2}
      />
    </View>
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
  // `artwork` is drawn across a still (#1659 web mock cards): a light fill on a translucent track.
  tone?: "artwork" | "live" | "primary";
  // `square` runs edge to edge along a still's foot (the web mock's On now card).
  ends?: "round" | "square";
};

const progressFill = { artwork: "$contentPrimary", live: "$stateLive", primary: "$actionPrimary" } as const;

const ProgressTrack = ({
  ends = "round",
  height = 4,
  percent,
  tone = "primary",
  ...props
}: ProgressTrackProps) => {
  const bounded = Math.max(0, Math.min(100, percent));
  const borderRadius = ends === "round" ? "$round" : 0;
  return (
    <View
      {...props}
      backgroundColor={tone === "artwork" ? "$artworkProgressTrack" : "$surfaceCanvas"}
      borderRadius={borderRadius}
      height={height}
      overflow="hidden"
    >
      <View
        backgroundColor={progressFill[tone]}
        borderRadius={borderRadius}
        height="100%"
        width={`${bounded}%`}
      />
    </View>
  );
};

export type { ArtworkState, BadgeTone, ScreenProps, TextProps, TextTone };
export { ArtworkFrame, Badge, FocusSurface, ProgressTrack, Screen, Surface, Text };
