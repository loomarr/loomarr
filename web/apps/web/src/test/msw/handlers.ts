import {
  getChannelGuideMockHandler,
  getFillerIncomingMockHandler,
  getFillerPoolMockHandler,
  getFillerWatchMockHandler,
  getGetChannelMockHandler,
  getJobsListMockHandler,
  getListActivityMockHandler,
  getListChannelsMockHandler,
  getListDocsMockHandler,
  getListFillerMockHandler,
  getListFillerSourcesMockHandler,
  getListProposalsMockHandler,
  getListTitlesMockHandler,
  getListUsersMockHandler,
  getSettingsListMockHandler,
  getSetupStateMockHandler,
  getSetupStatusMockHandler,
  getSystemLlmDiscoverMockHandler,
  getSystemLlmStatusMockHandler,
} from "@loomarr/api/msw";
import type { RequestHandler } from "msw";
import { channel } from "../fixtures/channels";

// appHandlers — the shared baseline for tests that mount the REAL route tree.
//
// ⚠ This exists because route-level tests fetch whatever the landed screen needs, and that is not
// a list anyone can hold in their head. Before V53e each of them answered the whole surface with a
// catch-all (`json({}, 200)` / `json({})`), so every screen rendered against empty objects — and
// the tests passed, because an empty object is a perfectly good answer to a question nobody
// checked. `test/app-router`, `test/reachability`, `test/settings`, `test/wizard-router`,
// `test/filler`, `test/guide-page`, `test/help` and `test/users` all did this.
//
// ⚠ EVERY response here is EMPTY-BUT-VALID, and that is deliberate: these are the "this screen
// renders at all" reads, not the data any assertion depends on. A test that cares about content
// overrides the one endpoint it asserts on and leaves the rest of the baseline alone.
//
// ⚠⚠ SPREAD IT LAST — `server.use(myOverride, ...appHandlers())`, NOT the other way round.
// MSW resolves the FIRST matching handler in its list, and `server.use()` PREPENDS its arguments
// to that list. So "the most recently registered handler wins" is true across separate `use()`
// CALLS and exactly backwards WITHIN one: spread the baseline first and its empty response sits
// in front of the override, which then never fires.
//
// This is not theoretical and it is not loud. `test/help` overrode `/v1/docs` with two pages,
// got `{ docs: [] }` from the baseline instead, and failed as five 5-SECOND TIMEOUTS with no
// unhandled-request error — the guard cannot help here, because the request WAS handled, just by
// the wrong handler. Silent-empty is the precise failure mode this whole layer exists to remove,
// so the ordering is load-bearing. `test/app-router` never hit it only because everything it adds
// (`/v1/auth/me`, login) is absent from the list below.
//
// ⚠ It is a HAND-MAINTAINED LIST, which is the drift class this repo tracks in three other places.
// It cannot be generated, because "which endpoints does the app fetch on route X" is a property of
// the components, not the spec. What keeps it honest is the unhandled-request guard in `./server`:
// add a screen that fetches something new and the test fails BY NAME rather than silently getting
// an empty object. That is the opposite failure mode from the catch-all this replaces — loud and
// specific, instead of silent and plausible.
const appHandlers = (): RequestHandler[] => [
  // ⚠ `/v1/setup/state` is fetched by the ROOT route on every render — the first thing the app
  // asks and the one the old catch-all most reliably answered with `{}`, which reads as
  // "not bootstrapped, no SSO, no dev login" whether or not that was true.
  getSetupStateMockHandler({ bootstrapped: true, devLogin: false, sso: false }),
  getSetupStatusMockHandler({ checks: [] }),
  getListChannelsMockHandler({ channels: [] }),
  // A single channel read &mdash; the channel-detail routes fetch this by id.
  getGetChannelMockHandler(channel()),
  getListTitlesMockHandler({ titles: [] }),
  getListProposalsMockHandler({ proposals: [] }),
  getListUsersMockHandler({ users: [] }),
  getSettingsListMockHandler({ settings: [], features: {} }),
  // ⚠ `total` is REQUIRED since §10 V51d added paging. This line and that field arrived in two
  // different PRs (#214 added the handler, #203 added the field); each was green against the main
  // it branched from, and together they did not typecheck — main went red on the second merge.
  // The generated client is the coupling, and neither diff mentions the other's file.
  getListFillerMockHandler({ clips: [], total: 0 }),
  getListFillerSourcesMockHandler({ sources: [], total: 0 }),
  getFillerIncomingMockHandler({
    asks: [],
    reels: [],
    recentlyFiled: [],
    pipeline: [],
    rejected: [],
    total: 0,
  }),
  // The guide grid's window read. `fromMs`/`toMs` are required, so an empty grid still has to
  // carry a coherent window rather than `{}`.
  getChannelGuideMockHandler({ channels: [], fromMs: 0, toMs: 0 }),
  getJobsListMockHandler({ jobs: [] }),
  getListActivityMockHandler({ activity: [] }),
  // The AI connection block reads both of these on mount, and it renders on the wizard's
  // connections step AS WELL AS on Settings → AI — so they are common surface, not one screen's
  // detail. ⚠ `local` and `reachable` are REQUIRED on the status body; a stub answering `{}`
  // (which is what every catch-all did) reads as neither, which is a coherent-looking lie.
  getSystemLlmStatusMockHandler({
    local: true,
    reachable: false,
    provider: "ollama",
    model: "",
    catalog: [],
    hosted: [],
  }),
  getSystemLlmDiscoverMockHandler({ models: [], sourceOk: true }),
  getListDocsMockHandler({ docs: [] }),
  getFillerPoolMockHandler({ channels: [], clips: 0, commercials: 0, eligible: 0, untagged: 0 }),
  getFillerWatchMockHandler({ clips: 0, health: "healthy", held: 0, sourcesOn: 0, sourcesTotal: 0 }),
];

export { appHandlers };
