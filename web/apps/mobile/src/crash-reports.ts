import {
  ClientCrashBreadcrumbCategory,
  ClientCrashReportInputPlatform,
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
import { useEffect } from "react";
import { AppState, Linking, Platform } from "react-native";

// Crash reports go to the paired Loomarr server and nowhere else (Refs #1816). The queue logic lives
// in @loomarr/core; this module adapts it to the phone: files, React Native's global error handler,
// MetricKit on iOS, and uploading once the phone is paired.

// One file per report. Every call is synchronous: a fatal error is written while React Native is
// about to end the app.
const directory = new Directory(Paths.document, "crash-reports");
const fileStorage: CrashReportStorage = {
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
    platform:
      Platform.OS === "ios" ? ClientCrashReportInputPlatform.ios : ClientCrashReportInputPlatform.android,
  },
  storage: fileStorage,
});

// Requests crash reports themselves make are not breadcrumbs, so uploads use the unwrapped fetch.
const unrecordedFetch = globalThis.fetch;
let uploadSoon: (() => void) | undefined;

/**
 * Installs crash capture: React Native's global error handler, request and lifecycle breadcrumbs,
 * and the MetricKit subscription (started by creating the native source). Runs once, at startup.
 */
const startCrashReporting = () => {
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
  // error when opened with `?loomarrCrashTest=js-fatal`. A release bundle inlines the unset variable
  // as undefined, so the minifier removes this block, and the TestFlight config refuses the variable.
  if (process.env.EXPO_PUBLIC_LOOMARR_CRASH_TEST === "1") {
    const crashOnTestLink = (url: string | null) => {
      if (!url?.includes("loomarrCrashTest=js-fatal")) return;
      setTimeout(() => {
        throw new Error("Loomarr crash report test: a fatal JS error");
      }, 0);
    };
    void Linking.getInitialURL().then(crashOnTestLink);
    Linking.addEventListener("url", ({ url }) => crashOnTestLink(url));
  }
};

const recordCrashBreadcrumb = (category: ClientCrashBreadcrumbCategory, message: string) =>
  reporter.breadcrumbs.add(category, message);

/**
 * While paired, sends pending reports to the server: at once (the crash that ended the last launch),
 * whenever the app returns to the foreground, after a non-fatal error, and when MetricKit delivers.
 */
const useCrashReportUpload = (credential: PairingCredential, session: PairingSession) => {
  useEffect(() => {
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
  }, [credential, session]);
};

export { recordCrashBreadcrumb, startCrashReporting, useCrashReportUpload };
