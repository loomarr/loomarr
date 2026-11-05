import { NativeAppRoot } from "@loomarr/native-app";
import { Stack } from "expo-router";
import { nativeApp } from "../src/native-app";

const RootLayout = () => (
  <NativeAppRoot app={nativeApp}>
    <Stack screenOptions={{ headerShown: false }} />
  </NativeAppRoot>
);

export default RootLayout;
