import { useEffect, useState } from "react";
import { AppState } from "react-native";

/** The part of React Native's `AppState` the hook reads. */
interface AppStateSource {
  addEventListener: (type: "change", listener: (state: string) => void) => { remove: () => void };
  currentState: string | null | undefined;
}

/**
 * Whether the app is in the foreground, following the system. LAN discovery browses only while the
 * connect screen is showing, so it reads this rather than the paired shell's own lifecycle.
 */
const useAppForeground = (appState: AppStateSource = AppState): boolean => {
  const [foreground, setForeground] = useState(appState.currentState === "active");
  useEffect(() => {
    const subscription = appState.addEventListener("change", (state) => setForeground(state === "active"));
    return () => subscription.remove();
  }, [appState]);
  return foreground;
};

export type { AppStateSource };
export { useAppForeground };
