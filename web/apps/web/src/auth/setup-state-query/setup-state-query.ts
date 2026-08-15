import * as setupApi from "@loomarr/api/endpoints/setup";
import type { SetupStateOutputBody } from "@loomarr/api/models/setupStateOutputBody";

// The single definition of the GET /v1/setup/state query — the UNAUTHENTICATED
// "does this install have an owner yet?" signal the router guards branch on (§7).
//
// Why it exists: the app is a static bundle, so without a fact the browser can read
// before it holds a session, every entry point resolves to /login — and a brand-new
// install has no account to log in with. That dead end is what the maintainer smoke
// walked into; only an operator who guessed the /wizard URL escaped it.
//
// retry:false for the same reason as meQueryOptions: the guard runs on the critical
// path of the very first paint, and a retry storm against a server that is up (just
// unclaimed) would stall the redirect rather than fix anything.
const setupStateQueryOptions = () => setupApi.getSetupStateQueryOptions({ query: { retry: false } });

type SetupStateClient = {
  ensureQueryData: (opts: ReturnType<typeof setupStateQueryOptions>) => Promise<unknown>;
};

// readSetupState is the one interpretation of the status-bearing orval response.
// Guards need all three advertised facts, while a transport/error response must
// never be mistaken for a real bootstrap or development-login decision.
const readSetupState = async (queryClient: SetupStateClient): Promise<SetupStateOutputBody | undefined> => {
  try {
    const res = (await queryClient.ensureQueryData(setupStateQueryOptions())) as {
      status?: number;
      data?: SetupStateOutputBody;
    };
    return res?.status === 200 ? res.data : undefined;
  } catch {
    return undefined;
  }
};

// needsBootstrap resolves the guard's question, and FAILS CLOSED: if the probe itself
// errors we report false (no bootstrap needed), which routes to the ordinary login.
// The inverse would send every visitor of a healthy, claimed install into the wizard
// the moment /v1/setup/state hiccups — turning a transient blip into a first-run
// screen for people who already have accounts.
const needsBootstrap = async (queryClient: SetupStateClient): Promise<boolean> =>
  (await readSetupState(queryClient))?.bootstrapped === false;

export { needsBootstrap, readSetupState, setupStateQueryOptions };
