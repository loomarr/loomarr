/**
 * The sentence shown when a channel refused to start and no more specific reason is available.
 *
 * Web reaches this only when a 5xx problem body can't be parsed (see `parseStartFailureReason`
 * below). Native (Expo) reaches it for EVERY 5xx start failure, by design (§1455 checkpoint 2):
 * the server's manifest endpoint IS the tune (internal/api/playout.go: tuneRaw → playout.Tune), so
 * the native transport cannot safely re-fetch it from JS to read the problem body — that would
 * start a second tune of a channel that just failed. Showing the server's specific `detail` on
 * native would need a read-only API surface the client can poll AFTER a failed tune without
 * triggering another one (e.g. a last-start-failure field scoped to the channel); none exists
 * today — see #1455 for that follow-up.
 */
const GENERIC_START_FAILURE = "Couldn't start this channel. Try again in a moment.";

const decodeResponseBody = (body: unknown): string => {
  if (typeof body === "string") return body;
  if (body instanceof ArrayBuffer) return new TextDecoder().decode(body);
  return "";
};

/**
 * The reason a channel's start request was refused, when the server decided it (a 5xx problem
 * response). The server answers a tune that could not start with a problem body whose `detail`
 * says why (program source unreachable, encoder stopped, all tuners busy), after its start
 * deadline. Retrying that request only reproduces the same wait, so the viewer is told instead.
 * A status below 500 is the ordinary warm-up race (a 404 or an empty playlist) and is left to the
 * caller's own retry, not reported as a start failure.
 */
const parseStartFailureReason = (status: number, body: unknown): string | undefined => {
  if (status < 500) return undefined;
  try {
    const problem = JSON.parse(decodeResponseBody(body)) as {
      detail?: unknown;
      title?: unknown;
    };
    for (const text of [problem.detail, problem.title]) {
      if (typeof text === "string" && text.trim() !== "") return text;
    }
  } catch {
    /* not a problem body — fall through to the generic sentence */
  }
  return GENERIC_START_FAILURE;
};

export { GENERIC_START_FAILURE, parseStartFailureReason };
