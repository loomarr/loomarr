import { styled, Text as TamaguiText, useTheme, View } from "@tamagui/core";
import { type ComponentProps, type ReactNode, useState } from "react";
import { Pressable, TextInput } from "react-native";

import { type Density, type TextRole, typography } from "../tokens";

const Screen = styled(View, {
  name: "LoomarrScreen",
  flex: 1,
  width: "100%",
  minHeight: "100%",
  backgroundColor: "$surfaceCanvas",
  padding: "$screen",
  variants: {
    density: {
      pointer: { padding: "$screen" },
      touch: { padding: "$section" },
      tv: { padding: 48 },
    },
  } as const,
  defaultVariants: {
    density: "pointer",
  },
});

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
type TextProps = Omit<
  TamaguiTextProps,
  "color" | "fontFamily" | "fontSize" | "fontWeight" | "letterSpacing" | "lineHeight" | "role"
> & {
  density?: Density;
  textRole: TextRole;
};

const dataRoles = new Set<TextRole>(["metadata", "time", "channelNumber"]);
const mutedRoles = new Set<TextRole>(["label", "metadata", "time"]);

const Text = ({ density = "pointer", textRole, ...props }: TextProps) => {
  const value = typography[density][textRole];
  return (
    <TamaguiText
      {...props}
      color={
        textRole === "channelNumber"
          ? "$actionPrimary"
          : mutedRoles.has(textRole)
            ? "$contentSecondary"
            : "$contentPrimary"
      }
      fontFamily={dataRoles.has(textRole) ? "$data" : "$body"}
      fontSize={value.size}
      fontWeight={value.weight}
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

const ActionFrame = styled(Pressable, {
  name: "LoomarrAction",
  alignItems: "center",
  borderRadius: "$control",
  borderWidth: 2,
  justifyContent: "center",
  paddingHorizontal: "$control",
  focusStyle: {
    borderColor: "$actionFocus",
    outlineColor: "$actionFocus",
    outlineStyle: "solid",
    outlineWidth: 3,
  },
  pressStyle: { opacity: 0.82, scale: 0.98 },
  variants: {
    tone: {
      primary: { backgroundColor: "$actionPrimary", borderColor: "$actionPrimary" },
      secondary: { backgroundColor: "$surfaceElevated", borderColor: "$borderDecorative" },
    },
  } as const,
  defaultVariants: { tone: "primary" },
});

type ActionProps = Omit<ComponentProps<typeof ActionFrame>, "children" | "tone"> & {
  children: ReactNode;
  density?: Density;
  tone?: "primary" | "secondary";
};

const Action = ({
  children,
  density = "pointer",
  onBlur,
  onFocus,
  tone = "primary",
  ...props
}: ActionProps) => {
  const [focused, setFocused] = useState(false);
  return (
    <ActionFrame
      {...props}
      borderColor={focused ? "$actionFocus" : undefined}
      borderWidth={focused ? 4 : 2}
      minHeight={density === "tv" ? 64 : density === "touch" ? 48 : 40}
      onBlur={(event) => {
        setFocused(false);
        onBlur?.(event);
      }}
      onFocus={(event) => {
        setFocused(true);
        onFocus?.(event);
      }}
      scale={focused ? 1.025 : 1}
      tone={tone}
    >
      <TamaguiText
        color={tone === "primary" ? "$contentInverse" : "$contentPrimary"}
        fontFamily="$body"
        fontSize={typography[density].label.size}
        fontWeight="700"
      >
        {children}
      </TamaguiText>
    </ActionFrame>
  );
};

type FieldProps = ComponentProps<typeof TextInput> & { density?: Density };
const Field = ({ density = "pointer", onBlur, onFocus, style, ...props }: FieldProps) => {
  const theme = useTheme();
  const [focused, setFocused] = useState(false);
  return (
    <TextInput
      {...props}
      onBlur={(event) => {
        setFocused(false);
        onBlur?.(event);
      }}
      onFocus={(event) => {
        setFocused(true);
        onFocus?.(event);
      }}
      placeholderTextColor={theme.contentMuted.val}
      style={[
        {
          backgroundColor: theme.surfaceCanvas.val,
          borderColor: focused ? theme.actionFocus.val : theme.borderDecorative.val,
          borderRadius: 8,
          borderWidth: focused ? 3 : 2,
          color: theme.contentPrimary.val,
          fontFamily: typography.family.data.native,
          fontSize: typography[density].body.size,
          minHeight: density === "tv" ? 64 : density === "touch" ? 48 : 40,
          paddingHorizontal: 16,
        },
        style,
      ]}
    />
  );
};

const ProgressTrack = ({ percent, tone = "primary", ...props }: ProgressTrackProps) => {
  const bounded = Math.max(0, Math.min(100, percent));
  return (
    <View {...props} backgroundColor="$surfaceCanvas" borderRadius="$round" height={4} overflow="hidden">
      <View
        backgroundColor={tone === "live" ? "$stateLive" : "$actionPrimary"}
        borderRadius="$round"
        height="100%"
        width={`${bounded}%`}
      />
    </View>
  );
};

export type { ActionProps, ArtworkState, BadgeTone, FieldProps, TextProps };
export { Action, ArtworkFrame, Badge, Field, FocusSurface, ProgressTrack, Screen, Surface, Text };
