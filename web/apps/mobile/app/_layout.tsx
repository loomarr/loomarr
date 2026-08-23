import { LoomarrProvider } from "@loomarr/design-system";
import { Stack } from "expo-router";

const RootLayout = () => (
  <LoomarrProvider>
    <Stack screenOptions={{ headerShown: false }} />
  </LoomarrProvider>
);

export default RootLayout;
