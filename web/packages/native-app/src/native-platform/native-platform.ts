import { ClientBatchPlatform } from "@loomarr/api/models/clientBatchPlatform";
import { ClientBatchSource } from "@loomarr/api/models/clientBatchSource";
import { ClientCrashReportInputPlatform } from "@loomarr/api/models/clientCrashReportInputPlatform";
import type { ClientDiagnosticsIdentity } from "@loomarr/core/client-diagnostics";

/** What React Native reports about the device the app runs on. */
interface NativeRuntime {
  isTV: boolean;
  os: "android" | "ios";
}

/** The platform a crash report names: tvOS is iOS on a TV, Android TV is Android on one. */
const nativeCrashPlatform = ({ isTV, os }: NativeRuntime): ClientCrashReportInputPlatform => {
  if (os === "ios") return isTV ? ClientCrashReportInputPlatform.tvos : ClientCrashReportInputPlatform.ios;
  return isTV ? ClientCrashReportInputPlatform["android-tv"] : ClientCrashReportInputPlatform.android;
};

/** The runtime and platform playback diagnostics name, one pair per native app. */
const nativeDiagnosticsIdentity = (
  { isTV, os }: NativeRuntime,
  clientVersion: string,
): ClientDiagnosticsIdentity => {
  if (os === "ios") {
    return isTV
      ? { clientVersion, platform: ClientBatchPlatform.apple_tv, source: ClientBatchSource.apple_tv }
      : { clientVersion, platform: ClientBatchPlatform.ios, source: ClientBatchSource.ios };
  }
  return isTV
    ? { clientVersion, platform: ClientBatchPlatform.android_tv, source: ClientBatchSource.android_tv }
    : { clientVersion, platform: ClientBatchPlatform.android, source: ClientBatchSource.android };
};

export type { NativeRuntime };
export { nativeCrashPlatform, nativeDiagnosticsIdentity };
