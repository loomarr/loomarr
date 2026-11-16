import { isWeb, useTheme, View } from "@tamagui/core";
import { useEffect, useRef } from "react";
import { Animated, Easing } from "react-native";

import { useNativeAnimationDriver } from "../../motion/animation-driver";
import { useReducedMotionPreference } from "../../motion/use-reduced-motion";
import { Text, type TextTone } from "../../text";
import { Tooltip } from "../../tooltip";

type StatusTone = "attention" | "error" | "live" | "notice" | "off" | "ok" | "pending" | "progress" | "warn";
type StatusDotProps = {
  /**
   * What the dot MEANS, for assistive tech — required, because a bare coloured circle conveys
   * nothing without it. Pass "" to opt out explicitly, for a dot beside text that already says it.
   */
  label: string;
  reducedMotion?: boolean;
  /** 8px by default; `compact` is 6px (a tighter row of dots). */
  size?: "compact" | "default";
  tone: StatusTone;
};

const statusDotTone = (theme: ReturnType<typeof useTheme>) =>
  ({
    live: theme.guideOnAir.val,
    error: theme.guideOnAir.val,
    pending: theme.guideReconciling.val,
    ok: theme.stateSuccess.val,
    warn: theme.stateWarning.val,
    off: theme.actionDisabled.val,
    attention: theme.accentSuggest.val,
    progress: theme.stateInfo.val,
    notice: theme.actionPrimary.val,
  }) as const;

/**
 * The pulse is reserved for `live` alone (§970 PR B, carried over from the legacy StatusDot):
 * motion means "happening right now", so `error` shares `live`'s colour but never animates — a
 * job that failed hours ago has nothing happening, and pulsing would say otherwise.
 */
const StatusDot = ({ label, reducedMotion, size = "default", tone }: StatusDotProps) => {
  const theme = useTheme();
  const prefersReducedMotion = useReducedMotionPreference(reducedMotion);
  const pulse = useRef(new Animated.Value(1)).current;
  const pulses = tone === "live";

  useEffect(() => {
    if (!pulses || prefersReducedMotion !== false) {
      pulse.stopAnimation();
      pulse.setValue(1);
      return undefined;
    }
    pulse.setValue(1);
    const animation = Animated.loop(
      Animated.sequence([
        Animated.timing(pulse, {
          duration: 1000,
          easing: Easing.inOut(Easing.quad),
          toValue: 0.5,
          useNativeDriver: useNativeAnimationDriver,
        }),
        Animated.timing(pulse, {
          duration: 1000,
          easing: Easing.inOut(Easing.quad),
          toValue: 1,
          useNativeDriver: useNativeAnimationDriver,
        }),
      ]),
    );
    animation.start();
    return () => animation.stop();
  }, [pulses, prefersReducedMotion, pulse]);

  const dimension = size === "compact" ? 6 : 8;
  return (
    <Animated.View
      {...(label ? { role: "img", "aria-label": label } : { "aria-hidden": true })}
      style={{
        backgroundColor: statusDotTone(theme)[tone],
        borderRadius: 999,
        flexShrink: 0,
        height: dimension,
        opacity: pulse,
        width: dimension,
      }}
    />
  );
};

/**
 * The five states a Settings row can be in, and nothing else. Every surface that reports a service,
 * a task or a credential picks one of these rather than writing its own word, so the same state
 * reads the same on the index, Connections, Diagnostics and Tasks.
 */
type StatusWordKind = "attention" | "expired" | "notSetUp" | "ok" | "overdue";

const statusWords: Record<
  StatusWordKind,
  { dot: StatusTone; label: string; text: TextTone; explanation?: string }
> = {
  ok: { dot: "ok", label: "OK", text: "success" },
  attention: { dot: "error", label: "Needs attention", text: "danger" },
  notSetUp: { dot: "off", label: "Not set up", text: "muted" },
  overdue: { dot: "warn", label: "Overdue", text: "warning" },
  expired: {
    dot: "progress",
    label: "Expired",
    text: "info",
    explanation: "This is no longer valid. Sign in or enter it again to keep it working.",
  },
};

type StatusWordProps = {
  kind: StatusWordKind;
};

/**
 * A status as a coloured dot and a plain word. The word is the meaning and the colour backs it up,
 * so it reads without colour vision. `expired` is the one word that needs explaining, so it carries
 * a tooltip.
 */
const StatusWord = ({ kind }: StatusWordProps) => {
  const { dot, explanation, label, text } = statusWords[kind];
  const word = (
    <View
      alignItems="center"
      flexDirection="row"
      gap={6}
      {...(isWeb ? ({ display: "inline-flex", render: "span" } as const) : {})}
      {...(explanation ? { focusable: true } : {})}
    >
      <StatusDot label="" tone={dot} />
      <Text textRole="compact" tone={text} strong>
        {label}
      </Text>
    </View>
  );
  return explanation ? <Tooltip content={explanation}>{word}</Tooltip> : word;
};

export type { StatusTone, StatusWordKind, StatusWordProps };
export { StatusDot, StatusWord };
