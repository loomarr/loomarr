import {
  Action,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Surface,
  semanticMotion,
  semanticSpace,
  Text,
  useReducedMotionPreference,
  useResolvedViewportInsets,
} from "@loomarr/design-system";
import { useEffect, useRef, useState } from "react";
import { Animated } from "react-native";

import type { ModalOverlayProps, OverlayAction, TransientOverlayProps } from "./overlay.type";

type OverlayPanelProps = Pick<
  ModalOverlayProps,
  "actions" | "children" | "density" | "description" | "eyebrow" | "title"
> & {
  contentInsets?: { bottom: number; left: number; right: number; top: number };
};

type OverlayDensity = NonNullable<OverlayPanelProps["density"]>;

// A button for each action, in whatever row or column its container draws.
const overlayActions = (actions: readonly OverlayAction[], density: OverlayDensity) =>
  actions.map((action) => (
    <Action
      density={density}
      disabled={action.disabled}
      hasTVPreferredFocus={density === "tv" && action.preferredFocus}
      key={action.label}
      onPress={action.onPress}
      selected={action.selected}
      tone={action.tone ?? "secondary"}
    >
      {action.label}
    </Action>
  ));

const OverlayActions = ({
  actions,
  density,
}: {
  actions?: readonly OverlayAction[];
  density: OverlayDensity;
}) =>
  actions?.length ? (
    <Surface
      alignItems="stretch"
      backgroundColor="transparent"
      borderWidth={0}
      // A phone stacks full-width buttons, as an iOS alert does. Wrapping the column would size every
      // button to the widest label instead.
      flexDirection={density === "touch" ? "column" : "row"}
      flexWrap={density === "touch" ? "nowrap" : "wrap"}
      gap="control"
    >
      {overlayActions(actions, density)}
    </Surface>
  ) : null;

const OverlayPanel = ({
  actions,
  children,
  contentInsets,
  density = "pointer",
  description,
  eyebrow,
  title,
}: OverlayPanelProps) => (
  <Surface
    gap="control"
    padding={density === "tv" ? 32 : "section"}
    paddingBottom={contentInsets?.bottom}
    paddingLeft={contentInsets?.left}
    paddingRight={contentInsets?.right}
    paddingTop={contentInsets?.top}
    // Edge to edge: the panel spans the frame, so it has no corners.
    borderRadius={0}
    width="100%"
    level="overlay"
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
 * Blocking composition on the design system's `Dialog`, which supplies the platform adapters: a
 * native modal on device, and on the web a portal with focus trap, Escape handling, stacking and
 * focus return. `dismissible={false}` makes it one that must be answered.
 */
const ModalOverlay = ({
  actions,
  children,
  density = "pointer",
  description,
  dismissible = true,
  eyebrow,
  onDismiss,
  title,
  visible,
}: ModalOverlayProps) => (
  <Dialog dismissible={dismissible} onOpenChange={(open) => !open && onDismiss()} open={visible}>
    <DialogContent density={density}>
      <DialogHeader>
        {eyebrow ? (
          <Text density={density} textRole="metadata" tone="info">
            {eyebrow}
          </Text>
        ) : null}
        <DialogTitle>{title}</DialogTitle>
        {description ? <DialogDescription>{description}</DialogDescription> : null}
      </DialogHeader>
      {children}
      {actions?.length ? (
        <DialogFooter stacked={density === "touch"}>{overlayActions(actions, density)}</DialogFooter>
      ) : null}
    </DialogContent>
  </Dialog>
);

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
        backgroundColor="transparent"
        borderRadius={0}
        borderWidth={0}
        pointerEvents="box-none"
        onBlur={() => setInteracting(false)}
        onFocus={() => setInteracting(true)}
        onTouchEnd={() => setInteracting(false)}
        onTouchStart={() => setInteracting(true)}
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
        />
      </Surface>
    </Animated.View>
  );
};

export { ModalOverlay, TransientOverlay };
