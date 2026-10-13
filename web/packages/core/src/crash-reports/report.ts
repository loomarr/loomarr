import type { ClientCrashBreadcrumb } from "@loomarr/api/models/clientCrashBreadcrumb";
import type { ClientCrashReportInput } from "@loomarr/api/models/clientCrashReportInput";
import { ClientCrashReportInputKind } from "@loomarr/api/models/clientCrashReportInputKind";
import { scrubText, truncateUtf8 } from "./scrub";

const MESSAGE_BYTES = 2 << 10;
const STACK_BYTES = 32 << 10;
const LABEL_BYTES = 128;

/** What the app knows about itself and the device it runs on; the same for every report. */
type CrashReportDevice = Pick<
  ClientCrashReportInput,
  "appBuild" | "appVersion" | "deviceModel" | "osVersion" | "platform"
>;

const crashLabel = (value: string) => truncateUtf8(value, LABEL_BYTES);

const crashMessage = (message: string) => {
  const scrubbed = truncateUtf8(scrubText(message), MESSAGE_BYTES);
  return scrubbed.trim() === "" ? "Unknown error" : scrubbed;
};

// Whatever was thrown: an Error, or any other value. Reading it must not throw inside a crash handler.
const describe = (thrown: unknown): { message: string; stack?: string } => {
  try {
    if (thrown instanceof Error) {
      const message = thrown.message ? `${thrown.name}: ${thrown.message}` : thrown.name;
      return { message, stack: typeof thrown.stack === "string" ? thrown.stack : undefined };
    }
    return { message: typeof thrown === "string" ? thrown : (JSON.stringify(thrown) ?? String(thrown)) };
  } catch {
    return { message: "Unreadable thrown value" };
  }
};

const jsCrashReport = (
  thrown: unknown,
  fatal: boolean,
  device: CrashReportDevice,
  breadcrumbs: ClientCrashBreadcrumb[],
  now: number,
): ClientCrashReportInput => {
  const { message, stack } = describe(thrown);
  return {
    appBuild: crashLabel(device.appBuild),
    appVersion: crashLabel(device.appVersion),
    breadcrumbs,
    deviceModel: crashLabel(device.deviceModel),
    kind: fatal ? ClientCrashReportInputKind["js-fatal"] : ClientCrashReportInputKind["js-error"],
    message: crashMessage(message),
    occurredAt: new Date(now).toISOString(),
    osVersion: crashLabel(device.osVersion),
    platform: device.platform,
    ...(stack ? { stack: truncateUtf8(scrubText(stack), STACK_BYTES) } : {}),
  };
};

export type { CrashReportDevice };
export { crashLabel, crashMessage, jsCrashReport };
