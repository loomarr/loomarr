import { LoomarrProvider } from "@loomarr/design-system";
import { Stack } from "expo-router";
import { SafeAreaProvider, useSafeAreaInsets } from "react-native-safe-area-context";
import { startCrashReporting } from "../src/crash-reports";

// Before the first render, so a crash while the app starts is captured too.
startCrashReporting();

const ClientLayout = () => {
  const insets = useSafeAreaInsets();
  return (
    <LoomarrProvider insets={insets}>
      <Stack screenOptions={{ headerShown: false }} />
    </LoomarrProvider>
  );
};

const RootLayout = () => (
  <SafeAreaProvider>
    <ClientLayout />
  </SafeAreaProvider>
);

export default RootLayout;
