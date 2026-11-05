import { ClientCrashBreadcrumbCategory } from "@loomarr/api/models/clientCrashBreadcrumbCategory";
import {
  CrashReporter,
  type CrashReportStorage,
  createBreadcrumbFetch,
  createCrashReportSender,
  installCrashHandler,
} from "@loomarr/core/crash-reports";
import { createAuthenticatedFetch, type PairingCredential, type PairingSession } from "@loomarr/core/pairing";
import { createNativeCrashSource, forwardNativeCrashes } from "@loomarr/crash-reports-native";
import Constants from "expo-constants";
import { Directory, File, Paths } from "expo-file-system";
import { AppState, Platform, Settings } from "react-native";

import { type NativeRuntime, nativeCrashPlatform } from "../native-platform";

// Crash reports go to the paired Loomarr server and nowhere else (Refs #1816). The queue logic lives
// in @loomarr/core; this module adapts it to a native app: files, React Native's global error
// handler, MetricKit where the native module is linked, and uploading once the device is paired.

/** One file per report. Every call is synchronous: a fatal error is written while React Native ends the app. */
const createFileStorage = (): CrashReportStorage => {
  const directory = new Directory(Paths.document, "crash-reports");
  return {
    list: () => (directory.exists ? directory.list().map(({ name }) => name) : []),
    read: (name) => {
      const file = new File(directory, name);
      return file.exists ? file.textSync() : undefined;
    },
    remove: (name) => {
      const file = new File(directory, name);
      if (file.exists) file.delete();
    },
    write: (name, contents) => {
      if (!directory.exists) directory.create({ idempotent: true, intermediates: true });
      new File(directory, name).writeSync(contents);
    },
  };
};

interface CrashReporting {
  /** Adds a navigation, lifecycle, http or log breadcrumb to the next report. */
  recordBreadcrumb: (category: ClientCrashBreadcrumbCategory, message: string) => void;
  /**
   * Sends pending reports to the paired server: at once (the crash that ended the last launch),
   * whenever the app returns to the foreground, after a non-fatal error, and when MetricKit
   * delivers. Answers the function that stops it.
   */
  startUpload: (credential: PairingCredential, session: PairingSession) => () => void;
}

/**
 * Installs crash capture (React Native's global error handler, request and lifecycle breadcrumbs,
 * and the MetricKit subscription, started by creating the native source) and answers the two
 * calls the paired shell makes. Runs once, at startup, before the first render.
 */
const createCrashReporting = (runtime: NativeRuntime): CrashReporting => {
  const nativeCrashes = createNativeCrashSource();
  const platformConstants = Platform.constants as { Model?: string; Release?: string };
  const reporter = new CrashReporter({
    device: {
      appBuild:
        nativeCrashes?.device.appBuild ??
        Constants.expoConfig?.ios?.buildNumber ??
        String(Constants.expoConfig?.android?.versionCode ?? "unknown"),
      appVersion: nativeCrashes?.device.appVersion ?? Constants.expoConfig?.version ?? "unknown",
      deviceModel: nativeCrashes?.device.deviceModel ?? platformConstants.Model ?? "unknown",
      osVersion: nativeCrashes?.device.osVersion ?? platformConstants.Release ?? String(Platform.Version),
      platform: nativeCrashPlatform(runtime),
    },
    storage: createFileStorage(),
  });

  // Requests crash reports themselves make are not breadcrumbs, so uploads use the unwrapped fetch.
  const unrecordedFetch = globalThis.fetch;
  let uploadSoon: (() => void) | undefined;

  installCrashHandler(ErrorUtils, {
    captureError(error, fatal) {
      reporter.captureError(error, fatal);
      // The app survives a non-fatal error, so its report need not wait for the next launch.
      if (!fatal) uploadSoon?.();
    },
  });
  globalThis.fetch = createBreadcrumbFetch(unrecordedFetch, reporter.breadcrumbs);
  reporter.breadcrumbs.add(ClientCrashBreadcrumbCategory.lifecycle, "launch");
  AppState.addEventListener("change", (state) =>
    reporter.breadcrumbs.add(ClientCrashBreadcrumbCategory.lifecycle, `app ${state}`),
  );

  // Simulator proof only: a build bundled with EXPO_PUBLIC_LOOMARR_CRASH_TEST=1 throws a fatal JS
  // error five seconds after `xcrun simctl launch <device> <app> -loomarrCrashTest js-fatal` (iOS
  // puts launch arguments in NSUserDefaults, which Settings reads). A release bundle inlines the unset
  // variable as undefined, so the minifier removes this block, and the TestFlight config refuses it.
  if (process.env.EXPO_PUBLIC_LOOMARR_CRASH_TEST === "1" && Settings.get("loomarrCrashTest") === "js-fatal") {
    setTimeout(() => {
      throw new Error("Loomarr crash report test: a fatal JS error");
    }, 5_000);
  }

  const startUpload = (credential: PairingCredential, session: PairingSession) => {
    const send = createCrashReportSender(
      createAuthenticatedFetch(credential, () => session.revoked(), unrecordedFetch),
    );
    let active = true;
    const upload = () => {
      if (!active) return;
      void (nativeCrashes ? forwardNativeCrashes(nativeCrashes, reporter) : Promise.resolve(0))
        .catch(() => 0)
        .then(() => reporter.flush(send));
    };
    uploadSoon = upload;
    upload();
    const unsubscribeNative = nativeCrashes?.subscribe(upload);
    const foreground = AppState.addEventListener("change", (state) => {
      if (state === "active") upload();
    });
    return () => {
      active = false;
      if (uploadSoon === upload) uploadSoon = undefined;
      unsubscribeNative?.();
      foreground.remove();
    };
  };

  return {
    recordBreadcrumb: (category, message) => reporter.breadcrumbs.add(category, message),
    startUpload,
  };
};

export type { CrashReporting };
export { createCrashReporting };
