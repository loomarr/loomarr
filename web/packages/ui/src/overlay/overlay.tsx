import {
  Action,
  Surface,
  semanticMotion,
  semanticSpace,
  Text,
  useReducedMotionPreference,
  useResolvedViewportInsets,
} from "@loomarr/design-system";
import { useEffect, useRef, useState } from "react";
import { Animated, Modal, View } from "react-native";

import type { ModalOverlayProps, OverlayAction, TransientOverlayProps } from "./overlay.type";

type OverlayPanelProps = Pick<
  ModalOverlayProps,
  "actions" | "children" | "density" | "description" | "eyebrow" | "title"
> & {
  contentInsets?: { bottom: number; left: number; right: number; top: number };
  edgeToEdge?: boolean;
  modal?: boolean;
};

const OverlayActions = ({
  actions,
  density,
}: {
  actions?: readonly OverlayAction[];
  density: NonNullable<OverlayPanelProps["density"]>;
}) =>
  actions?.length ? (
    <Surface
      alignItems="stretch"
      backgroundColor="$transparent"
      borderWidth={0}
      flexDirection={density === "touch" ? "column" : "row"}
      flexWrap="wrap"
      gap="$control"
    >
      {actions.map((action) => (
        <Action
          density={density}
          disabled={action.disabled}
          hasTVPreferredFocus={density === "tv" && action.preferredFocus}
          key={action.label}
          onPress={action.onPress}
          tone={action.tone ?? "secondary"}
        >
          {action.label}
        </Action>
      ))}
    </Surface>
  ) : null;

const OverlayPanel = ({
  actions,
  children,
  contentInsets,
  density = "pointer",
  description,
  eyebrow,
  edgeToEdge = false,
  modal = false,
  title,
}: OverlayPanelProps) => (
  <Surface
    gap="$control"
    level="overlay"
    maxWidth={modal ? (density === "tv" ? 880 : 600) : undefined}
    padding={density === "tv" ? 32 : "$section"}
    paddingBottom={contentInsets?.bottom}
    paddingLeft={contentInsets?.left}
    paddingRight={contentInsets?.right}
    paddingTop={contentInsets?.top}
    borderRadius={edgeToEdge ? 0 : undefined}
    width="100%"
    zIndex={modal ? 1 : undefined}
  >
    {eyebrow ? (
      <Text density={density} textRole="metadata" tone="info">
        {eyebrow}
      </Text>
    ) : null}
    <Text density={density} textRole="title">
      {title}
    </Text>
    {description ? (
      <Text density={density} textRole="body">
        {description}
      </Text>
    ) : null}
    {children}
    <OverlayActions actions={actions} density={density} />
  </Surface>
);

/**
 * Blocking composition. React Native supplies the two real platform adapters: a native modal on
 * device and a portal with focus trap, Escape handling, stacking, and focus return on the web.
 */
const ModalOverlay = ({
  dismissible = true,
  onDismiss,
  reducedMotion,
  visible,
  ...content
}: ModalOverlayProps) => {
  const prefersReducedMotion = useReducedMotionPreference(reducedMotion);
  return (
    <Modal
      accessibilityLabel={content.title}
      accessibilityViewIsModal
      animationType={prefersReducedMotion === false ? "fade" : "none"}
      hardwareAccelerated
      navigationBarTranslucent
      onRequestClose={dismissible ? onDismiss : () => undefined}
      presentationStyle="overFullScreen"
      statusBarTranslucent
      transparent
      visible={visible}
    >
      <Surface
        alignItems="center"
        backgroundColor="$transparent"
        borderRadius={0}
        borderWidth={0}
        flex={1}
        justifyContent="center"
        padding="$section"
      >
        <View
          accessibilityElementsHidden
          accessible={false}
          importantForAccessibility="no-hide-descendants"
          onResponderRelease={dismissible ? onDismiss : undefined}
          onStartShouldSetResponder={() => dismissible}
          style={{
            bottom: 0,
            left: 0,
            position: "absolute",
            right: 0,
            top: 0,
          }}
        >
          <Surface backgroundColor="$artworkScrim" borderRadius={0} borderWidth={0} flex={1} />
        </View>
        <OverlayPanel {...content} modal />
      </Surface>
    </Modal>
  );
};

const useTransientPresence = (visible: boolean, reducedMotion?: boolean) => {
  const prefersReducedMotion = useReducedMotionPreference(reducedMotion);
  const [rendered, setRendered] = useState(visible);
  const progress = useRef(new Animated.Value(visible ? 1 : 0)).current;

  useEffect(() => {
    if (visible) setRendered(true);
    const animation = Animated.timing(progress, {
      duration: prefersReducedMotion === false ? semanticMotion.overlay : 0,
      toValue: visible ? 1 : 0,
      useNativeDriver: false,
    });
    animation.start(({ finished }) => {
      if (finished && !visible) setRendered(false);
    });
    return () => animation.stop();
  }, [prefersReducedMotion, progress, visible]);

  return { progress, rendered };
};

/** Edge-to-edge, non-modal composition for playback and tuning feedback. */
const TransientOverlay = ({
  autoDismissMs,
  density = "pointer",
  onDismiss,
  placement = "bottom",
  reducedMotion,
  visible,
  ...content
}: TransientOverlayProps) => {
  const insets = useResolvedViewportInsets(density);
  const [interacting, setInteracting] = useState(false);
  const { progress, rendered } = useTransientPresence(visible, reducedMotion);

  useEffect(() => {
    if (!visible || interacting || !autoDismissMs || autoDismissMs <= 0) return undefined;
    const timeout = setTimeout(onDismiss, autoDismissMs);
    return () => clearTimeout(timeout);
  }, [autoDismissMs, interacting, onDismiss, visible]);

  if (!rendered) return null;

  return (
    <Animated.View
      pointerEvents="box-none"
      style={{
        left: 0,
        opacity: progress,
        position: "absolute",
        right: 0,
        [placement]: 0,
        transform: [
          {
            translateY: progress.interpolate({
              inputRange: [0, 1],
              outputRange: [placement === "bottom" ? 24 : -24, 0],
            }),
          },
        ],
        zIndex: 100,
      }}
    >
      <Surface
        backgroundColor="$transparent"
        borderRadius={0}
        borderWidth={0}
        onBlur={() => setInteracting(false)}
        onFocus={() => setInteracting(true)}
        onTouchEnd={() => setInteracting(false)}
        onTouchStart={() => setInteracting(true)}
        pointerEvents="box-none"
      >
        <OverlayPanel
          {...content}
          contentInsets={{
            bottom: placement === "bottom" ? insets.bottom : semanticSpace.section,
            left: insets.left,
            right: insets.right,
            top: placement === "top" ? insets.top : semanticSpace.section,
          }}
          density={density}
          edgeToEdge
        />
      </Surface>
    </Animated.View>
  );
};

export { ModalOverlay, TransientOverlay };
