// Interim mirror of the server's ClientCrashReportInput (POST /v1/diagnostics/client-crashes,
// Refs #1816). The server lane generates the orval model and enum symbols; this file is deleted for
// imports from @loomarr/api before the app PR merges.

type ClientCrashReportInputKind =
  (typeof ClientCrashReportInputKind)[keyof typeof ClientCrashReportInputKind];
const ClientCrashReportInputKind = {
  "js-fatal": "js-fatal",
  "js-error": "js-error",
  native: "native",
} as const;

type ClientCrashReportInputPlatform =
  (typeof ClientCrashReportInputPlatform)[keyof typeof ClientCrashReportInputPlatform];
const ClientCrashReportInputPlatform = {
  ios: "ios",
  android: "android",
  tvos: "tvos",
  "android-tv": "android-tv",
} as const;

type ClientCrashBreadcrumbCategory =
  (typeof ClientCrashBreadcrumbCategory)[keyof typeof ClientCrashBreadcrumbCategory];
const ClientCrashBreadcrumbCategory = {
  navigation: "navigation",
  http: "http",
  lifecycle: "lifecycle",
  log: "log",
} as const;

type ClientCrashBreadcrumb = {
  at: string;
  category: ClientCrashBreadcrumbCategory;
  message: string;
};

type ClientCrashReportInput = {
  appBuild: string;
  appVersion: string;
  breadcrumbs: ClientCrashBreadcrumb[];
  deviceModel: string;
  kind: ClientCrashReportInputKind;
  message: string;
  nativePayload?: { [key: string]: unknown };
  occurredAt: string;
  osVersion: string;
  platform: ClientCrashReportInputPlatform;
  stack?: string;
};

const getReportClientCrashUrl = () => "/v1/diagnostics/client-crashes";

export type { ClientCrashBreadcrumb, ClientCrashReportInput };
export {
  ClientCrashBreadcrumbCategory,
  ClientCrashReportInputKind,
  ClientCrashReportInputPlatform,
  getReportClientCrashUrl,
};
