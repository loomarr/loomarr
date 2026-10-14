/**
 * The sentence shown when a channel refused to start and no more specific reason is available:
 * a 5xx answer whose problem body can't be parsed (see `parseStartFailureReason` below).
 */
const GENERIC_START_FAILURE = "Couldn't start this channel. Try again in a moment.";

/** The sentence shown when the paired server could not be reached at all while starting a channel. */
const SERVER_UNREACHABLE = "Couldn't reach Loomarr. Check your connection and try again.";

/**
 * A channel start the server refused and explained (a 5xx problem response to the manifest that
 * starts the stream). It is final for this tune: the server already waited out its own start
 * deadline, so an automatic retry only makes the viewer wait the same time again. The viewer sees
 * the message and decides whether to Retry.
 */
class StartFailure extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "StartFailure";
    this.status = status;
  }
}

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

export { GENERIC_START_FAILURE, parseStartFailureReason, SERVER_UNREACHABLE, StartFailure };
