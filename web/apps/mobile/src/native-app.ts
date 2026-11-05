import { createNativeApp } from "@loomarr/native-app";

// Created at module load, before the first render, so a crash while the app starts is captured too.
const nativeApp = createNativeApp();

export { nativeApp };
