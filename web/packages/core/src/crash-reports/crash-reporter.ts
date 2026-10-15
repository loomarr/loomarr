import { getReportClientCrashUrl } from "@loomarr/api/endpoints/diagnostics";
import type { ClientCrashReportInput } from "@loomarr/api/models/clientCrashReportInput";
import { BreadcrumbTrail } from "./breadcrumbs";
import { type NativeCrashEnvelope, nativeCrashReports } from "./native";
import { type CrashReportDevice, jsCrashReport } from "./report";

/**
 * Where pending reports live between a crash and the next launch: one entry per report. Every
 * method is synchronous, because a fatal JS error is written while React Native is about to kill
 * the app and nothing awaited would finish.
 */
type CrashReportStorage = {
  list(): string[];
  read(name: string): string | undefined;
  remove(name: string): void;
  write(name: string, contents: string): void;
};

/** Sends one report and answers the HTTP status; a network failure rejects. */
type CrashReportSend = (report: ClientCrashReportInput) => Promise<number>;

type CrashReportFlush = { dropped: number; kept: number; sent: number };

type CrashReporterOptions = {
  device: CrashReportDevice;
  now?: () => number;
  queueLimit?: number;
  storage: CrashReportStorage;
};

const QUEUE_LIMIT = 20;
const REPORT_NAME = /^\d{15}-[a-z0-9]+\.json$/;
// The server will never accept these, so keeping them would resend them on every launch. Anything
// else (401, 429, 5xx, offline) is the server or session, and the report waits for the next flush.
const PERMANENT_REJECTIONS = new Set([400, 413, 422]);

const isReport = (value: unknown): value is ClientCrashReportInput => {
  const candidate = value as Partial<ClientCrashReportInput> | null;
  return (
    typeof candidate === "object" &&
    candidate !== null &&
    typeof candidate.kind === "string" &&
    typeof candidate.message === "string" &&
    Array.isArray(candidate.breadcrumbs)
  );
};

/**
 * Captures crashes into a bounded on-disk queue and sends them to the paired server later. A report
 * leaves the queue only once the server answered 2xx (or refused it for good), so a crash survives
 * being offline, unpaired or killed mid-send.
 */
class CrashReporter {
  readonly breadcrumbs: BreadcrumbTrail;
  private readonly device: CrashReportDevice;
  private readonly now: () => number;
  private readonly queueLimit: number;
  private readonly storage: CrashReportStorage;
  private flushing: Promise<CrashReportFlush> | undefined;
  private sequence = 0;

  constructor({ device, now = Date.now, queueLimit = QUEUE_LIMIT, storage }: CrashReporterOptions) {
    this.device = device;
    this.now = now;
    this.queueLimit = queueLimit;
    this.storage = storage;
    this.breadcrumbs = new BreadcrumbTrail(now);
  }

  /** Persists a thrown value synchronously. Never throws: it runs inside the global error handler. */
  captureError(thrown: unknown, fatal: boolean) {
    try {
      this.enqueue(jsCrashReport(thrown, fatal, this.device, this.breadcrumbs.snapshot(), this.now()));
    } catch {
      // A crash report that cannot be written must not replace the crash it describes.
    }
  }

  /** Queues the crashes in one MetricKit payload; answers how many there were. */
  captureNative(envelope: NativeCrashEnvelope): number {
    const reports = nativeCrashReports(envelope, this.device);
    for (const report of reports) this.enqueue(report);
    return reports.length;
  }

  pending(): ClientCrashReportInput[] {
    return this.entries().map(({ report }) => report);
  }

  /** Sends pending reports oldest first, one flush at a time, stopping at the first failure. */
  flush(send: CrashReportSend): Promise<CrashReportFlush> {
    this.flushing ??= this.sendPending(send).finally(() => {
      this.flushing = undefined;
    });
    return this.flushing;
  }

  private async sendPending(send: CrashReportSend): Promise<CrashReportFlush> {
    const entries = this.entries();
    const result: CrashReportFlush = { dropped: 0, kept: entries.length, sent: 0 };
    for (const { name, report } of entries) {
      let status: number;
      try {
        status = await send(report);
      } catch {
        break;
      }
      const sent = status >= 200 && status < 300;
      if (!sent && !PERMANENT_REJECTIONS.has(status)) break;
      this.storage.remove(name);
      result.kept -= 1;
      if (sent) result.sent += 1;
      else result.dropped += 1;
    }
    return result;
  }

  private enqueue(report: ClientCrashReportInput) {
    const name = `${String(this.now()).padStart(15, "0")}-${(this.sequence++).toString(36)}${Math.random()
      .toString(36)
      .slice(2, 8)}.json`;
    this.storage.write(name, JSON.stringify(report));
    const names = this.names();
    for (const oldest of names.slice(0, Math.max(0, names.length - this.queueLimit)))
      this.storage.remove(oldest);
  }

  private names() {
    return this.storage
      .list()
      .filter((name) => REPORT_NAME.test(name))
      .sort();
  }

  private entries() {
    return this.names().flatMap((name) => {
      try {
        const report: unknown = JSON.parse(this.storage.read(name) ?? "");
        if (isReport(report)) return [{ name, report }];
      } catch {
        // A report torn by the crash it was describing is unreadable; it is removed below.
      }
      this.storage.remove(name);
      return [];
    });
  }
}

type GlobalErrorHandler = (error: unknown, isFatal?: boolean) => void;
/** React Native's `ErrorUtils` global, the last handler before a fatal JS error kills the app. */
type GlobalErrorUtils = {
  getGlobalHandler(): GlobalErrorHandler;
  setGlobalHandler(handler: GlobalErrorHandler): void;
};

/**
 * Captures every error that reaches React Native's global handler, then hands it to the previous
 * handler, which still reports it (and, for a fatal one, ends the app). Returns the uninstaller.
 */
const installCrashHandler = (errorUtils: GlobalErrorUtils, reporter: Pick<CrashReporter, "captureError">) => {
  const previous = errorUtils.getGlobalHandler();
  const handler: GlobalErrorHandler = (error, isFatal) => {
    reporter.captureError(error, isFatal === true);
    previous(error, isFatal);
  };
  errorUtils.setGlobalHandler(handler);
  return () => {
    if (errorUtils.getGlobalHandler() === handler) errorUtils.setGlobalHandler(previous);
  };
};

/**
 * Sends reports through a fetch that already speaks for the paired device (the authenticated
 * fetch adds its token and resolves the path against the server). The server reads who sent it from
 * that authentication, so the report carries no identity of its own.
 */
const createCrashReportSender =
  (fetcher: typeof globalThis.fetch): CrashReportSend =>
  async (report) => {
    const response = await fetcher(getReportClientCrashUrl(), {
      body: JSON.stringify(report),
      headers: { "Content-Type": "application/json" },
      method: "POST",
    });
    return response.status;
  };

export type { CrashReportFlush, CrashReportSend, CrashReportStorage, GlobalErrorUtils };
export { CrashReporter, createCrashReportSender, installCrashHandler };
