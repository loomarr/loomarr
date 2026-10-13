import { type ClientCrashReportInput, ClientCrashReportInputKind } from "./contract";
import type { CrashReportDevice } from "./report";
import { crashLabel, crashMessage } from "./report";
import { scrubText, utf8ByteLength } from "./scrub";

const NATIVE_PAYLOAD_BYTES = 64 << 10;

/**
 * One MetricKit diagnostic payload as the native module persisted it: MXDiagnosticPayload's
 * `jsonRepresentation()`, and the end of the window it covers as RFC 3339 (MetricKit reports a
 * crash on the next launch and dates it no closer than that window).
 */
type NativeCrashEnvelope = { endedAt: string; id: string; payload: string };

type MetricKitObject = { [key: string]: unknown };

const asObject = (value: unknown): MetricKitObject | undefined =>
  value && typeof value === "object" && !Array.isArray(value) ? (value as MetricKitObject) : undefined;
const asString = (value: unknown) => (typeof value === "string" && value.trim() !== "" ? value : undefined);
const asNumber = (value: unknown) => (typeof value === "number" ? value : undefined);

// Mach exception types and POSIX signals as <mach/exception_types.h> and <sys/signal.h> number them.
const MACH_EXCEPTIONS: Record<number, string> = {
  1: "EXC_BAD_ACCESS",
  2: "EXC_BAD_INSTRUCTION",
  3: "EXC_ARITHMETIC",
  4: "EXC_EMULATION",
  5: "EXC_SOFTWARE",
  6: "EXC_BREAKPOINT",
  10: "EXC_CRASH",
  11: "EXC_RESOURCE",
  12: "EXC_GUARD",
};
const SIGNALS: Record<number, string> = {
  4: "SIGILL",
  5: "SIGTRAP",
  6: "SIGABRT",
  8: "SIGFPE",
  9: "SIGKILL",
  10: "SIGBUS",
  11: "SIGSEGV",
  13: "SIGPIPE",
};

const symbol = (names: Record<number, string>, value: number | undefined, prefix: string) =>
  value === undefined ? undefined : (names[value] ?? `${prefix} ${value}`);

const nativeCrashMessage = (metadata: MetricKitObject) => {
  const exception = symbol(MACH_EXCEPTIONS, asNumber(metadata.exceptionType), "exception");
  const signal = symbol(SIGNALS, asNumber(metadata.signal), "signal");
  const kind = [exception, signal && (exception ? `(${signal})` : signal)].filter(Boolean).join(" ");
  const reason =
    asString(asObject(metadata.exceptionReason)?.composedMessage) ?? asString(metadata.terminationReason);
  return [kind || "Native crash", reason].filter(Boolean).join(": ");
};

const scrubStrings = (value: unknown): unknown => {
  if (typeof value === "string") return scrubText(value);
  if (Array.isArray(value)) return value.map(scrubStrings);
  const object = asObject(value);
  if (!object) return value;
  return Object.fromEntries(Object.entries(object).map(([key, entry]) => [key, scrubStrings(entry)]));
};

const fits = (payload: MetricKitObject) => utf8ByteLength(JSON.stringify(payload)) <= NATIVE_PAYLOAD_BYTES;

/**
 * The payload for one crash, inside the contract's 64 KiB. A deep stack can exceed it; the crashed
 * thread's stack is what symbolication needs, so the other threads go first, then the stack itself.
 */
const boundedPayload = (
  window: MetricKitObject,
  diagnostic: MetricKitObject,
): MetricKitObject | undefined => {
  const withDiagnostic = (crash: MetricKitObject) => ({ ...window, crashDiagnostics: [crash] });
  const complete = withDiagnostic(diagnostic);
  if (fits(complete)) return complete;
  const tree = asObject(diagnostic.callStackTree);
  const stacks = Array.isArray(tree?.callStacks) ? tree.callStacks : [];
  const crashed = withDiagnostic({
    ...diagnostic,
    callStackTree: {
      ...tree,
      callStacks: stacks.filter((stack) => asObject(stack)?.threadAttributed === true),
    },
  });
  if (fits(crashed)) return crashed;
  const metadataOnly = withDiagnostic({ ...diagnostic, callStackTree: undefined });
  return fits(metadataOnly) ? metadataOnly : undefined;
};

/**
 * A crash report per crash in one MetricKit payload. Its version, build, OS and model are the ones
 * that crashed, which MetricKit records, rather than this launch's. Hangs, CPU and disk-write
 * diagnostics are not crashes and are left out.
 */
const nativeCrashReports = (
  envelope: NativeCrashEnvelope,
  device: CrashReportDevice,
): ClientCrashReportInput[] => {
  let parsed: unknown;
  try {
    parsed = JSON.parse(envelope.payload);
  } catch {
    return [];
  }
  const payload = asObject(scrubStrings(parsed));
  if (!payload) return [];
  const { crashDiagnostics, ...rest } = payload;
  const window = Object.fromEntries(Object.entries(rest).filter(([key]) => key.startsWith("timeStamp")));
  return (Array.isArray(crashDiagnostics) ? crashDiagnostics : []).flatMap((entry) => {
    const diagnostic = asObject(entry);
    if (!diagnostic) return [];
    const metadata = asObject(diagnostic.diagnosticMetaData) ?? {};
    return [
      {
        appBuild: crashLabel(asString(metadata.appBuildVersion) ?? device.appBuild),
        appVersion: crashLabel(asString(metadata.appVersion) ?? device.appVersion),
        breadcrumbs: [],
        deviceModel: crashLabel(asString(metadata.deviceType) ?? device.deviceModel),
        kind: ClientCrashReportInputKind.native,
        message: crashMessage(nativeCrashMessage(metadata)),
        nativePayload: boundedPayload(window, diagnostic),
        occurredAt: envelope.endedAt,
        osVersion: crashLabel(asString(metadata.osVersion) ?? device.osVersion),
        platform: device.platform,
      },
    ];
  });
};

export type { NativeCrashEnvelope };
export { nativeCrashReports };
