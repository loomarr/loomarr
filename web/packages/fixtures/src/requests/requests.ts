import type { ChannelIdeaDTO } from "@loomarr/api/models/channelIdeaDTO";
import type { Proposal } from "@loomarr/api/models/proposal";
import type { ProposalDTO } from "@loomarr/api/models/proposalDTO";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import type { ProposalJourneyDTO } from "@loomarr/api/models/proposalJourneyDTO";
import type { PullDTO } from "@loomarr/api/models/pullDTO";
import type { TitleDTO } from "@loomarr/api/models/titleDTO";
import type { RequestEntry, RequestsSnapshot } from "@loomarr/core/requests";
import { requestStatus } from "@loomarr/core/requests";

// Invented requests for Storybook and tests: every title and name here is made up, never copied from
// a design mock or a real library. Dates are fixed so snapshots never drift.

const item = (name: string, tmdbId: number, over: Partial<ProposalItem> = {}): ProposalItem => ({
  inLibrary: false,
  mediaType: "movie",
  name,
  tmdbId,
  year: 1994,
  ...over,
});

// A proposal's scoring and decision trace are server evidence no screen reads; the cast keeps the
// fixtures to the fields the phone actually shows.
const proposalBody = (description: string, lineup: ProposalItem[], acquisitions: ProposalItem[]): Proposal =>
  ({ acquisitions, alternates: [], intent: { description }, lineup }) as unknown as Proposal;

const journey = (
  over: Partial<ProposalJourneyDTO> & Pick<ProposalJourneyDTO, "jobId">,
): ProposalJourneyDTO => ({
  actions: [],
  attempts: [],
  createdAt: "2026-10-01T18:00:00Z",
  intent: { description: "Gentle detective stories for rainy evenings" },
  milestone: "live",
  updatedAt: "2026-10-01T18:30:00Z",
  version: 1,
  ...over,
});

// Provider ids sit above 90,000,000, the demo-library range no real TMDB/TVDB id reaches, so a
// configured TMDB key can never fetch a real poster for an invented title (internal/demolibrary).
const lighthouse = item("The Lighthouse Ledger", 90009001, { year: 1998 });
const marmalade = item("Marmalade Street", 90009002, { year: 2003 });
const harbour = item("Harbour Lights", 90009003, { year: 2011 });

const journeys = {
  generating: journey({
    intent: { description: "Slow-burn space operas with big ships" },
    jobId: "job-generating",
    milestone: "generating",
    progress: {
      picks: [
        {
          inLibrary: true,
          key: "movie:tmdb:90009001",
          mediaType: "movie",
          name: lighthouse.name,
          year: 1998,
        },
        { inLibrary: false, key: "movie:tmdb:90009003", mediaType: "movie", name: harbour.name, year: 2011 },
      ],
      stage: "choosing",
      startedAt: "2026-10-03T09:00:00Z",
      target: 8,
      terms: [],
    },
  }),
  awaitingApproval: journey({
    createdAt: "2026-10-02T12:00:00Z",
    intent: { description: "Cosy baking competitions from around the world" },
    jobId: "job-waiting",
    milestone: "awaiting_approval",
    proposal: {
      id: "proposal-waiting",
      proposal: proposalBody("Cosy baking competitions", [marmalade], [harbour]),
      status: "submitted",
    },
  }),
  downloading: journey({
    createdAt: "2026-09-30T20:00:00Z",
    intent: { description: "Harbour-town dramas with a bit of weather" },
    jobId: "job-downloading",
    milestone: "building",
    proposal: {
      approvedAt: "2026-10-01T08:00:00Z",
      approvedByName: "Ada Lovelace",
      id: "proposal-downloading",
      proposal: proposalBody("Harbour-town dramas", [lighthouse], [marmalade, harbour]),
      status: "approved",
    },
  }),
  live: journey({
    channel: { id: "channel-rainy", status: "live" },
    createdAt: "2026-09-25T19:00:00Z",
    intent: { description: "Gentle detective stories for rainy evenings" },
    jobId: "job-live",
    milestone: "live",
    proposal: {
      approvedAt: "2026-09-26T09:00:00Z",
      approvedByName: "Ada Lovelace",
      id: "proposal-live",
      proposal: proposalBody("Gentle detective stories", [lighthouse, marmalade], []),
      status: "approved",
    },
  }),
  denied: journey({
    createdAt: "2026-09-28T10:00:00Z",
    intent: { description: "Every film ever made about robots" },
    jobId: "job-denied",
    milestone: "denied",
    proposal: {
      denyReason: "That's more than the library can hold. Try one decade at a time.",
      id: "proposal-denied",
      proposal: proposalBody("Every film about robots", [], []),
      status: "denied",
    },
  }),
  failed: journey({
    actions: ["edit", "retry"],
    createdAt: "2026-10-02T21:00:00Z",
    failure: {
      code: "selection_empty",
      guidance: "A channel needs at least 5 titles.",
      message: "Only 2 titles matched that description.",
      reason: "no_catalog_match",
      recoveryAction: "broaden_request",
    },
    intent: { description: "Silent films scored by modern bands" },
    jobId: "job-failed",
    milestone: "failed",
  }),
} satisfies Record<string, ProposalJourneyDTO>;

const titles: TitleDTO[] = [
  { key: "movie:tmdb:90009002", mediaType: "movie", state: "downloading", tmdbId: 90009002 },
  { key: "movie:tmdb:90009003", mediaType: "movie", state: "wanted", tmdbId: 90009003 },
];

const ideas: ChannelIdeaDTO[] = [
  {
    facet: "genre",
    id: "idea-mystery",
    inLibrary: 14,
    keys: ["movie:tmdb:90009001", "movie:tmdb:90009002", "movie:tmdb:90009003", "movie:tmdb:90009004"],
    movies: 14,
    name: "Fireside Mysteries",
    pitch: "Cosy whodunnits for a slow evening.",
    reason: { count: 14, kind: "unaired" },
    requested: false,
    series: 0,
    toDownload: 0,
    value: "Mystery",
  },
  {
    facet: "decade",
    id: "idea-nineties",
    inLibrary: 9,
    keys: ["movie:tmdb:90009005", "movie:tmdb:90009006"],
    movies: 11,
    name: "Back to the Nineties",
    pitch: "Rental-shelf favourites, back to back.",
    reason: { count: 11, kind: "unaired" },
    requested: false,
    series: 0,
    toDownload: 2,
    value: "1990",
  },
  {
    facet: "genre",
    id: "idea-sitcom",
    inLibrary: 0,
    keys: ["tv:tvdb:90009007"],
    movies: 0,
    name: "Couch Comedies",
    pitch: "Half-hour sitcoms with laugh tracks.",
    reason: { count: 6, kind: "unaired" },
    requested: true,
    requestJobId: "job-waiting",
    series: 6,
    toDownload: 6,
    value: "Comedy",
  },
];

const approvals: ProposalDTO[] = [
  {
    createdByName: "Grace Hopper",
    id: "approval-grace",
    jobId: "job-grace",
    note: "For the weekend film club.",
    proposal: proposalBody("Black-and-white thrillers", [lighthouse], [marmalade, harbour]),
    status: "submitted",
  },
  {
    createdByName: "Katherine Johnson",
    id: "approval-katherine",
    jobId: "job-katherine",
    proposal: proposalBody("Mountain documentaries", [harbour], []),
    status: "submitted",
  },
  {
    createdByName: "Mary Jackson",
    id: "approval-mary",
    jobId: "job-mary",
    proposal: proposalBody("Overnight jazz and rain sounds", [marmalade, harbour], [lighthouse]),
    status: "submitted",
  },
];

const pull = (
  id: string,
  title: string,
  estimateClips: number,
  sourceCount: number,
  reason: string,
): PullDTO => ({
  candidateCount: estimateClips,
  createdAt: "2026-10-02T06:00:00Z",
  estimateClips,
  id,
  intent: { catalogReason: reason, count: estimateClips, version: "1" },
  plan: [],
  proposedBy: "Loomarr",
  reason,
  rejected: [],
  sources: Array.from({ length: sourceCount }, (_, index) => ({
    candidateCount: Math.ceil(estimateClips / sourceCount),
    detail: "",
    disposition: "enumerated",
    provider: "invented-archive",
    sourceId: `${id}-source-${index + 1}`,
  })),
  status: "pending",
  title,
});

// Filler downloads Loomarr proposed on its own: an admin approves or dismisses them apart from the
// channel requests, because approving one starts downloads.
const fillerPulls: PullDTO[] = [
  pull("pull-bumpers", "Retro game-show bumpers", 40, 1, "Breaks on the quiz channel repeat too soon."),
  pull(
    "pull-stingers",
    "Static and tuning-card stingers",
    18,
    2,
    "The late-night channel has no short fillers.",
  ),
];

const entry = (source: ProposalJourneyDTO): RequestEntry => ({
  journey: source,
  status: requestStatus(source, titles),
});

/** A ready snapshot a story or test can override one field at a time. */
const requestsSnapshot = (over: Partial<RequestsSnapshot> = {}): RequestsSnapshot => ({
  approvals: [],
  deciding: [],
  entries: [
    journeys.failed,
    journeys.generating,
    journeys.awaitingApproval,
    journeys.downloading,
    journeys.live,
    journeys.denied,
  ].map(entry),
  fillerPulls: [],
  ideas,
  ideasStatus: "ready",
  role: "member",
  status: "ready",
  titles,
  ...over,
});

const requestFixtures = { approvals, fillerPulls, ideas, journeys, titles };

export { requestFixtures, requestsSnapshot };
