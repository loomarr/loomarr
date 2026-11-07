import type { PairingCredential, PairingSession } from "@loomarr/core/pairing";
import { LoomarrProvider } from "@loomarr/design-system";
import Constants from "expo-constants";
import { createContext, type ReactNode, useContext, useEffect } from "react";
import { Platform } from "react-native";
import { SafeAreaProvider, useSafeAreaInsets } from "react-native-safe-area-context";

import { type CrashReporting, createCrashReporting } from "../crash-reporting";
import { type NativeRuntime, nativeDiagnosticsIdentity } from "../native-platform";

interface NativeAppOptions {
  /** The version playback diagnostics report; defaults to the app's own configured version. */
  clientVersion?: string;
}

interface NativeApp {
  crashReports: CrashReporting;
  /** Who this app is to the diagnostics API; pass it to `usePairedClient`. */
  diagnostics: ReturnType<typeof nativeDiagnosticsIdentity>;
}

const NativeAppContext = createContext<NativeApp | undefined>(undefined);

const Providers = ({ children }: { children: ReactNode }) => {
  const insets = useSafeAreaInsets();
  return <LoomarrProvider insets={insets}>{children}</LoomarrProvider>;
};

/** Draws the app's providers (safe area, Loomarr theme) around its screens. */
const NativeAppRoot = ({ app, children }: { app: NativeApp; children: ReactNode }) => (
  <NativeAppContext value={app}>
    <SafeAreaProvider>
      <Providers>{children}</Providers>
    </SafeAreaProvider>
  </NativeAppContext>
);

/**
 * While paired, sends this app's pending crash reports to the server (see `CrashReporting`).
 * Call it once from the paired shell, under `NativeAppRoot`.
 */
const useCrashReportUpload = (credential: PairingCredential, session: PairingSession) => {
  const app = useContext(NativeAppContext);
  useEffect(() => app?.crashReports.startUpload(credential, session), [app, credential, session]);
};

/**
 * The bootstrap every native app calls once, at module load and before the first render, so a crash
 * while the app starts is captured too: it installs crash reporting and names the app for playback
 * diagnostics from the platform it runs on.
 */
const createNativeApp = ({ clientVersion }: NativeAppOptions = {}): NativeApp => {
  const runtime: NativeRuntime = { isTV: Platform.isTV, os: Platform.OS === "ios" ? "ios" : "android" };
  return {
    crashReports: createCrashReporting(runtime),
    diagnostics: nativeDiagnosticsIdentity(
      runtime,
      clientVersion ?? Constants.expoConfig?.version ?? "unknown",
    ),
  };
};

export type { NativeApp, NativeAppOptions };
export { createNativeApp, NativeAppRoot, useCrashReportUpload };
