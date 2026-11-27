// First: the Intl APIs Hermes lacks, before any screen formats a language name.
import "./src/intl-polyfills";

export type { AppStateSource } from "./src/app-foreground";
export { useAppForeground } from "./src/app-foreground";
export type { LaunchGateOptions, LaunchStageProps } from "./src/launch-stage";
export { LaunchStage, useLaunchGate } from "./src/launch-stage";
export type { NativeApp, NativeAppOptions } from "./src/native-app";
export {
  createNativeApp,
  NativeAppRoot,
  useCrashReportUpload,
  usePairingBreadcrumbs,
  useShellCrashReporting,
} from "./src/native-app";
export type { NativeRuntime } from "./src/native-platform";
export { nativeCrashPlatform, nativeDiagnosticsIdentity } from "./src/native-platform";
