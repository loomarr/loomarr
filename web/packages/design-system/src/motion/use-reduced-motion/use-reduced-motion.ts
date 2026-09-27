import { useEffect, useState } from "react";
import { AccessibilityInfo } from "react-native";

const useReducedMotionPreference = (override?: boolean): boolean | null => {
  const [systemPreference, setSystemPreference] = useState<boolean | null>(override ?? null);

  useEffect(() => {
    if (override !== undefined) {
      setSystemPreference(override);
      return undefined;
    }

    let mounted = true;
    void AccessibilityInfo.isReduceMotionEnabled().then((enabled) => {
      if (mounted) setSystemPreference(enabled);
    });
    const subscription = AccessibilityInfo.addEventListener("reduceMotionChanged", setSystemPreference);
    return () => {
      mounted = false;
      // react-native-web returns a subscription without `remove` where matchMedia is missing
      // (jsdom), so an unmount must not assume one.
      subscription?.remove?.();
    };
  }, [override]);

  return systemPreference;
};

export { useReducedMotionPreference };
