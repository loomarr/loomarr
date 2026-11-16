import { useReducedMotionPreference } from "@loomarr/design-system";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { View } from "react-native";

// An answer locks the card in (static → clear) and it leaves once that has been seen.
const LOCK_CLEAR_MS = 200;
const LOCK_HOLD_MS = 1400;

// The answered card, held a moment: it starts greyed and clears, then leaves. The clearing is a
// CSS transition (react-native-web passes it through); Animated needs a global the web build lacks.
const LockIn = ({ children, onDone }: { children: ReactNode; onDone: () => void }) => {
  const reduced = useReducedMotionPreference();
  const [clear, setClear] = useState(false);
  // The latest onDone, so a re-render (a refetch) doesn't restart the hold.
  const done = useRef(onDone);
  done.current = onDone;
  useEffect(() => {
    const start = setTimeout(() => setClear(true), 30);
    const leave = setTimeout(() => done.current(), LOCK_HOLD_MS);
    return () => {
      clearTimeout(start);
      clearTimeout(leave);
    };
  }, []);
  const transition =
    reduced === false ? { transitionDuration: `${LOCK_CLEAR_MS}ms`, transitionProperty: "opacity" } : {};
  return <View style={[{ opacity: clear ? 1 : 0.6 }, transition as object]}>{children}</View>;
};

export { LockIn };
