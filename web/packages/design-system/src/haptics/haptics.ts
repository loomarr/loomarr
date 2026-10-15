import * as Haptics from "expo-haptics";
import { useMemo } from "react";
import { Platform } from "react-native";

// The three taps the app uses, named for what they say rather than for the motor: a selection tick
// for a step the viewer chose, success for a decision that went through, warning for one that didn't.
interface HapticFeedback {
  selection: () => void;
  success: () => void;
  warning: () => void;
}

// Haptics belong to a phone in a hand. The web has no motor, a TV remote has none to speak of, and
// every call here is for a viewer's own press: nothing fires on an automatic event.
const hasHaptics = () => Platform.OS !== "web" && !Platform.isTV;

// A haptic is garnish: a device that refuses one (low power, no taptic engine) must not break the
// press that asked for it.
const fire = (play: () => Promise<void>) => {
  if (hasHaptics()) void play().catch(() => undefined);
};

const feedback: HapticFeedback = {
  selection: () => fire(() => Haptics.selectionAsync()),
  success: () => fire(() => Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success)),
  warning: () => fire(() => Haptics.notificationAsync(Haptics.NotificationFeedbackType.Warning)),
};

const useHaptic = (): HapticFeedback => useMemo(() => feedback, []);

export type { HapticFeedback };
export { useHaptic };
