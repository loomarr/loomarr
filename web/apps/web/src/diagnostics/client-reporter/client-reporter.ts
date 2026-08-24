import { ingestClientDiagnostics } from "@loomarr/api/endpoints/diagnostics";
import type { ClientBatch } from "@loomarr/api/models/clientBatch";
import type { ClientObservation as GeneratedClientObservation } from "@loomarr/api/models/clientObservation";
import { observeApiFailures } from "@loomarr/api/mutator";

type WebPlatform = "chromium" | "firefox" | "webkit" | "unknown_web";
type ClientEvent = GeneratedClientObservation["event"];
type ClientObservation = Omit<GeneratedClientObservation, "occurredAt"> & { occurredAt?: number };
type AcceptedObservation = GeneratedClientObservation;
type SendBatch = (events: AcceptedObservation[]) => Promise<void>;

const QUEUE_LIMIT = 100;
const BATCH_LIMIT = 20;
const FLUSH_MS = 2_000;
const errorEvents = new Set<ClientEvent>([
  "client.error_boundary",
  "client.unhandled_error",
  "client.api_failed",
  "player.media_error",
]);

const webPlatform = (): WebPlatform => {
  const ua = navigator.userAgent.toLowerCase();
  if (ua.includes("firefox")) return "firefox";
  if (ua.includes("applewebkit") && !ua.includes("chrome") && !ua.includes("chromium")) return "webkit";
  if (ua.includes("chrome") || ua.includes("chromium")) return "chromium";
  return "unknown_web";
};

class ClientDiagnosticsReporter {
  private readonly queue: AcceptedObservation[] = [];
  private timer?: number;
  private sending = false;
  private version = "embedded";

  constructor(private readonly sendBatch: SendBatch) {}

  setVersion(version: string | undefined) {
    if (version) this.version = version.slice(0, 64);
  }

  record(observation: ClientObservation) {
    const accepted = { ...observation, occurredAt: observation.occurredAt ?? Date.now() };
    if (this.queue.length >= QUEUE_LIMIT) {
      const routine = this.queue.findIndex((queued) => !errorEvents.has(queued.event));
      this.queue.splice(routine >= 0 ? routine : 0, 1);
    }
    this.queue.push(accepted);
    this.schedule();
  }

  async flush() {
    if (this.sending || this.queue.length === 0) return;
    this.sending = true;
    const batch = this.queue.splice(0, BATCH_LIMIT);
    try {
      await this.sendBatch(batch);
    } catch {
      // Diagnostics are best-effort and must never hold playback. Restore only what still fits;
      // a later observation or timer retries it without growing memory without bound.
      this.queue.unshift(...batch.slice(-(QUEUE_LIMIT - this.queue.length)));
    } finally {
      this.sending = false;
      if (this.queue.length > 0) this.schedule();
    }
  }

  private schedule() {
    if (this.timer !== undefined) return;
    this.timer = window.setTimeout(() => {
      this.timer = undefined;
      void this.flush();
    }, FLUSH_MS);
  }

  wireBatch(events: AcceptedObservation[]): ClientBatch {
    return { source: "web", clientVersion: this.version, platform: webPlatform(), events };
  }
}

let reporter: ClientDiagnosticsReporter;
reporter = new ClientDiagnosticsReporter(async (events) => {
  await ingestClientDiagnostics(reporter.wireBatch(events), { keepalive: true });
});

const errorClassOf = (error: unknown): GeneratedClientObservation["errorClass"] => {
  if (error instanceof TypeError) return "type_error";
  if (error instanceof RangeError) return "range_error";
  if (error instanceof Error) return "error";
  return "unknown";
};

const installGlobalClientDiagnostics = () => {
  observeApiFailures(({ requestId, status }) => {
    reporter.record({ event: "client.api_failed", requestId, httpStatus: status });
  });
  window.addEventListener("error", (event) => {
    reporter.record({
      event: "client.unhandled_error",
      surface: "root",
      errorClass: errorClassOf(event.error),
    });
  });
  window.addEventListener("unhandledrejection", () => {
    reporter.record({ event: "client.unhandled_error", surface: "root", errorClass: "promise_rejection" });
  });
};

export type { ClientObservation, SendBatch };
export { ClientDiagnosticsReporter, installGlobalClientDiagnostics, reporter as clientDiagnostics };
