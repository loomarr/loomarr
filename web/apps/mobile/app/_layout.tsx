import { NativeAppRoot } from "@loomarr/native-app";
import { Stack } from "expo-router";
import { lockAsync, OrientationLock } from "expo-screen-orientation";
import { nativeApp } from "../src/native-app";

// The app is portrait; only Watching follows the phone round (#2127).
void lockAsync(OrientationLock.PORTRAIT_UP);

const RootLayout = () => (
  <NativeAppRoot app={nativeApp}>
    <Stack screenOptions={{ headerShown: false }} />
  </NativeAppRoot>
);

export default RootLayout;
