import { getIngestClientDiagnosticsUrl } from "@loomarr/api/endpoints/diagnostics";
import type { ClientBatch } from "@loomarr/api/models/clientBatch";
import type { ClientObservation as GeneratedClientObservation } from "@loomarr/api/models/clientObservation";

type ClientObservation = Omit<GeneratedClientObservation, "occurredAt"> & { occurredAt?: number };
type AcceptedObservation = GeneratedClientObservation;
/**
 * Sends one batch and resolves with the server's client-event vocabulary from its answer. A server
 * from before vocabularies reports none: resolve `undefined`, which counts as version 0.
 */
type SendBatch = (events: AcceptedObservation[]) => Promise<number | undefined>;
type ClientDiagnosticsIdentity = Pick<ClientBatch, "clientVersion" | "platform" | "source">;

const QUEUE_LIMIT = 100;
const BATCH_LIMIT = 20;
const FLUSH_MS = 2_000;

// The vocabulary version that introduced each event and field (docs/design/observability.md, "Client
// diagnostics"). A server refuses a whole batch for one event or field it doesn't know, and an app is
// often newer than the server it's paired with, so an observation goes only to a server whose
// vocabulary covers it. The fourteen original events are version 0. A new event or field is listed
// here with the version that added it, in the same change that raises the server's ClientVocabulary.
const EVENT_SINCE: Record<GeneratedClientObservation["event"], number> = {
  "client.api_failed": 0,
  "client.error_boundary": 0,
  "client.unhandled_error": 0,
  "player.attached": 0,
  "player.buffering_ended": 0,
  "player.buffering_started": 0,
  "player.detached": 0,
  "player.media_error": 0,
  "player.playhead_drift": 0,
  "player.ready": 0,
  "player.schedule_block_changed": 0,
  "player.seeked": 0,
  "player.seeking": 0,
  "player.source_replaced": 0,
};
const FIELD_SINCE: Partial<Record<keyof GeneratedClientObservation, number>> = {};

/** The lowest server vocabulary that accepts this observation. */
const vocabularyOf = (observation: AcceptedObservation): number =>
  Math.max(
    EVENT_SINCE[observation.event],
    ...Object.keys(observation).map((field) => FIELD_SINCE[field as keyof GeneratedClientObservation] ?? 0),
  );
const errorEvents = new Set<GeneratedClientObservation["event"]>([
  "client.error_boundary",
  "client.unhandled_error",
  "client.api_failed",
  "player.media_error",
]);

const defaultIdentity: ClientDiagnosticsIdentity = {
  clientVersion: "embedded",
  platform: "unknown_web",
  source: "web",
};

const boundObservations = (observations: AcceptedObservation[]): AcceptedObservation[] => {
  const bounded = [...observations];
  while (bounded.length > QUEUE_LIMIT) {
    const routine = bounded.findIndex(({ event }) => !errorEvents.has(event));
    bounded.splice(Math.max(routine, 0), 1);
  }
  return bounded;
};

class ClientDiagnosticsReporter {
  private readonly queue: AcceptedObservation[] = [];
  private timer?: ReturnType<typeof setTimeout>;
  private sending = false;
  private disposed = false;
  // The diagnostics endpoint requires a session. A logged-out page that kept flushing got a 401
  // every interval — thousands in bursts — so sending is gated on this and a 401 closes it.
  private authenticated = true;
  private identity: ClientDiagnosticsIdentity;
  // The server's vocabulary, learned from its first accepted batch. Until then only version-0
  // observations go, which every server accepts; newer ones wait in the bounded queue, and are
  // dropped once the server turns out to be too old to ever take them.
  private serverVocabulary: number | undefined;

  constructor(
    private readonly sendBatch: SendBatch,
    identity: ClientDiagnosticsIdentity = defaultIdentity,
  ) {
    this.identity = { ...identity, clientVersion: identity.clientVersion.slice(0, 64) };
  }

  setVersion(version: string | undefined) {
    if (version) this.identity = { ...this.identity, clientVersion: version.slice(0, 64) };
  }

  // Observations still queue (bounded) while unauthenticated so early errors survive login,
  // but nothing is sent until the session is known good.
  setAuthenticated(authenticated: boolean) {
    this.authenticated = authenticated;
    if (authenticated) this.schedule();
  }

  record(observation: ClientObservation) {
    if (this.disposed) return;
    const accepted = { ...observation, occurredAt: observation.occurredAt ?? Date.now() };
    this.replaceQueue([...this.queue, accepted]);
    this.schedule();
  }

  async flush() {
    if (this.sending || !this.authenticated || this.queue.length === 0) return;
    const ceiling = this.serverVocabulary ?? 0;
    const batch: AcceptedObservation[] = [];
    const held: AcceptedObservation[] = [];
    for (const observation of this.queue) {
      if (batch.length < BATCH_LIMIT && vocabularyOf(observation) <= ceiling) batch.push(observation);
      else held.push(observation);
    }
    if (batch.length === 0) return;
    this.sending = true;
    this.replaceQueue(held);
    try {
      this.learnVocabulary((await this.sendBatch(batch)) ?? 0);
    } catch (error) {
      if ((error as { status?: unknown } | null)?.status === 401) this.authenticated = false;
      this.replaceQueue([...batch, ...this.queue]);
    } finally {
      this.sending = false;
      if (this.queue.length > 0 && this.authenticated && !this.disposed) this.schedule();
    }
  }

  dispose() {
    this.disposed = true;
    if (this.timer !== undefined) clearTimeout(this.timer);
    this.timer = undefined;
  }

  wireBatch(events: AcceptedObservation[]): ClientBatch {
    return { ...this.identity, events };
  }

  private learnVocabulary(vocabulary: number) {
    this.serverVocabulary = vocabulary;
    this.replaceQueue(this.queue.filter((observation) => vocabularyOf(observation) <= vocabulary));
  }

  private replaceQueue(observations: AcceptedObservation[]) {
    this.queue.splice(0, this.queue.length, ...boundObservations(observations));
  }

  private schedule() {
    if (this.disposed || this.timer !== undefined) return;
    this.timer = setTimeout(() => {
      this.timer = undefined;
      void this.flush();
    }, FLUSH_MS);
  }
}

/**
 * Builds the batch sender for clients that authenticate with a bearer token rather than the
 * browser session (Android TV). A non-2xx answer throws so the reporter requeues the batch.
 */
const createAuthenticatedBatchSender =
  (request: typeof globalThis.fetch, wire: (events: AcceptedObservation[]) => ClientBatch): SendBatch =>
  async (events) => {
    const response = await request(getIngestClientDiagnosticsUrl(), {
      body: JSON.stringify(wire(events)),
      headers: { "Content-Type": "application/json" },
      method: "POST",
    });
    if (!response.ok) throw new Error(`Client diagnostics rejected (${response.status}).`);
    return vocabularyIn(await response.json().catch(() => undefined));
  };

/** The vocabulary in an accepted batch's answer; a server from before vocabularies has none. */
const vocabularyIn = (body: unknown): number | undefined => {
  const vocabulary = (body as { vocabulary?: unknown } | null | undefined)?.vocabulary;
  return typeof vocabulary === "number" && Number.isInteger(vocabulary) ? vocabulary : undefined;
};

export type { AcceptedObservation, ClientDiagnosticsIdentity, ClientObservation, SendBatch };
export { ClientDiagnosticsReporter, createAuthenticatedBatchSender, vocabularyIn };
