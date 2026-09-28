import { View } from "@tamagui/core";
import { type ReactNode, useMemo, useRef } from "react";
import { Animated, PanResponder } from "react-native";

import { useReducedMotionPreference } from "../motion/use-reduced-motion";
import { semanticMotion, semanticRadius } from "../tokens";

type BottomSheetProps = {
  accessibilityLabel?: string;
  children: ReactNode;
  /**
   * The iPhone detail sheet (#1659 mock 5d) wears a grabber and drags down to close. Android's
   * docked strip (5f) is the same raised surface with neither.
   */
  handle?: boolean;
  /** Called when the sheet is dragged down past its threshold, or on the screen reader's escape gesture. */
  onDismiss?: () => void;
};

// A downward drag this far, or a flick this fast, closes the sheet; anything less springs back.
const DISMISS_DISTANCE = 64;
const DISMISS_VELOCITY = 0.5;

/**
 * A raised surface docked to the bottom edge, holding one selection and its primary action. The
 * host places it above its navigation; the sheet doesn't cover the screen or trap focus.
 */
const BottomSheet = ({ accessibilityLabel, children, handle = true, onDismiss }: BottomSheetProps) => {
  const reducedMotion = useReducedMotionPreference() === true;
  const offset = useRef(new Animated.Value(0)).current;
  const draggable = handle && onDismiss !== undefined;

  const responder = useMemo(
    () =>
      PanResponder.create({
        onMoveShouldSetPanResponder: (_, gesture) =>
          draggable && gesture.dy > 4 && Math.abs(gesture.dy) > Math.abs(gesture.dx),
        onPanResponderMove: (_, gesture) => offset.setValue(Math.max(0, gesture.dy)),
        onPanResponderRelease: (_, gesture) => {
          if (gesture.dy > DISMISS_DISTANCE || gesture.vy > DISMISS_VELOCITY) {
            onDismiss?.();
            return;
          }
          Animated.timing(offset, {
            duration: reducedMotion ? 0 : semanticMotion.overlay,
            toValue: 0,
            useNativeDriver: false,
          }).start();
        },
      }),
    [draggable, offset, onDismiss, reducedMotion],
  );

  return (
    <Animated.View
      {...(draggable ? responder.panHandlers : {})}
      accessibilityLabel={accessibilityLabel}
      onAccessibilityEscape={onDismiss}
      style={{ transform: [{ translateY: offset }] }}
    >
      <View
        backgroundColor="$surfaceRaised"
        borderTopLeftRadius={semanticRadius.cardCompact}
        borderTopRightRadius={semanticRadius.cardCompact}
        gap={14}
        paddingBottom={16}
        paddingHorizontal={16}
        paddingTop={handle ? 10 : 16}
      >
        {handle ? (
          <View
            alignSelf="center"
            aria-hidden
            backgroundColor="$borderDecorative"
            borderRadius={4}
            height={4}
            width={36}
          />
        ) : null}
        {children}
      </View>
    </Animated.View>
  );
};

export type { BottomSheetProps };
export { BottomSheet };
