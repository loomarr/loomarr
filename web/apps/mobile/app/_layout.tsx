import { LoomarrProvider } from "@loomarr/design-system";
import { Stack } from "expo-router";
import { SafeAreaProvider, useSafeAreaInsets } from "react-native-safe-area-context";

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
