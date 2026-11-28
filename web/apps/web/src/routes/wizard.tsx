import * as libraryApi from "@loomarr/api/endpoints/library";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import * as setupApi from "@loomarr/api/endpoints/setup";
import { createFileRoute } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth/me-query";
import { isConnectionId, PLAYOUT_INTERNAL, PLAYOUT_TUNARR, wizardSteps } from "@/wizard/steps";
import { WizardPage, type WizardSearch } from "@/wizard/wizard-page";

const Route = createFileRoute("/wizard")({
  // `?step=` and `?conn=` are deep-linkable (§13). Both NARROW rather than validate-and-throw,
  // the same shape `?tab=` uses: a stale link from an older release lands on a real screen
  // instead of an error. The step is only sanity-checked for shape here — whether it is
  // REACHABLE depends on server truth the router does not have, so that clamp lives in
  // resolveStep, where the checks are.
  //
  // ⚠ Checked against EVERY step and connection either backend can have, not the chosen
  // one's: validateSearch runs before the component, so the settings query has not resolved
  // and the backend is unknown here. Narrowing to the internal path would silently drop
  // `?step=library` and `?conn=tunarr` on a Tunarr install, turning a valid deep link into
  // a fall-through. resolveStep does the backend-aware clamp once the answer is in hand.
  validateSearch: (search: Record<string, unknown>): WizardSearch => {
    const known = (id: unknown) =>
      [...wizardSteps(PLAYOUT_INTERNAL), ...wizardSteps(PLAYOUT_TUNARR)].some((s) => s.id === id);
    const knownConn = (id: string | undefined) =>
      isConnectionId(id, PLAYOUT_INTERNAL) || isConnectionId(id, PLAYOUT_TUNARR);
    return {
      step: known(search.step) ? (search.step as string) : undefined,
      conn: knownConn(search.conn as string | undefined) ? (search.conn as string) : undefined,
    };
  },
  // The server truth the resume point is derived from, so the wizard opens on the right step rather
  // than holding the paint. Not signed in yet (the bootstrap step) there is nothing to read, and the
  // reads are the signed-in operator's: a member would only be refused. Each answers for itself
  // when it fails (the page's own retry), so they are warmed, not awaited to success.
  loader: async ({ context: { queryClient } }) => {
    const me = await queryClient.ensureQueryData(meQueryOptions()).catch(() => undefined);
    if (me?.status !== 200 || me.data.role !== "admin") return;
    await Promise.all([
      queryClient.prefetchQuery(setupApi.getSetupStatusQueryOptions({ query: { retry: false } })),
      queryClient.prefetchQuery(settingsApi.getSettingsListQueryOptions({ query: { retry: false } })),
      queryClient.prefetchQuery(libraryApi.getLibraryStatusQueryOptions({ query: { retry: false } })),
    ]);
  },
  component: WizardPage,
});

export { Route };
