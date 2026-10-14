import { Platform } from "react-native";

// React Native Web implements Animated in JavaScript and warns when asked for the unavailable
// native driver. Native clients keep transform and opacity work off the JS thread.
const useNativeAnimationDriver = Platform.OS !== "web";

export { useNativeAnimationDriver };
