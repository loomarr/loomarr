import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import type { ProposalJourneyDTO } from "@loomarr/api/models/proposalJourneyDTO";
import type { TitleDTO } from "@loomarr/api/models/titleDTO";
import { pluralize } from "@loomarr/core/format";
import type { RequestAcquisition, RequestStatus } from "./request-status.type";

// One request, read the way its requester asks about it: does it need me, is it still moving,
// or is it finished? This is the ONLY place a Journey milestone plus the acquisition states of its
// titles collapse into a Requests tab and a single status line, so the tab, the card and the
// detail can never disagree about where a request stands.
//
// ⚠ There is no "Ready" outcome. A title that has landed is part of its channel; the request is
// simply Done once the channel is live and nothing it asked for is still on its way or given up.

// The API keys titles by their own id scheme, so a proposal item is joined to its provisioning
// row by media type plus the TMDB (or TVDB) id — the same identity the enqueue contract takes.
const sameTitle = (item: ProposalItem, title: TitleDTO): boolean =>
  item.mediaType === title.mediaType &&
  ((Boolean(item.tmdbId) && item.tmdbId === title.tmdbId) ||
    (Boolean(item.tvdbId) && item.tvdbId === title.tvdbId));

/** Each title the request asked Loomarr to acquire, with its provisioning row when there is one. */
const requestAcquisitions = (journey: ProposalJourneyDTO, titles: TitleDTO[]): RequestAcquisition[] =>
  (journey.proposal?.proposal.acquisitions ?? []).map((item) => ({
    item,
    title: titles.find((title) => sameTitle(item, title)),
  }));

const requestStatus = (journey: ProposalJourneyDTO, titles: TitleDTO[]): RequestStatus => {
  switch (journey.milestone) {
    case "failed":
      return {
        tab: "needs-you",
        line: journey.failure?.message ?? "Couldn't generate this channel",
        tone: "onair",
      };
    case "denied":
      return { tab: "done", line: "Not approved", tone: "onair" };
    case "generating":
      return { tab: "in-progress", line: "Generating", tone: "suggest" };
    case "awaiting_approval":
      return { tab: "in-progress", line: "Waiting for approval", tone: "suggest" };
    default:
      break;
  }

  const states = requestAcquisitions(journey, titles).flatMap(({ title }) => (title ? [title.state] : []));
  const downloading = states.filter((s) => s === "requested" || s === "downloading").length;
  const waiting = states.filter((s) => s === "wanted").length;
  const givenUp = states.filter((s) => s === "unavailable").length;

  if (downloading + waiting > 0) {
    const parts = [
      downloading > 0 ? `${downloading} downloading` : null,
      waiting > 0 ? `${waiting} waiting` : null,
    ].filter(Boolean);
    return {
      tab: "in-progress",
      line: `Getting ${pluralize(downloading + waiting, "title")} (${parts.join(", ")})`,
      tone: "caution",
    };
  }
  if (givenUp > 0) {
    return { tab: "in-progress", line: `Couldn't get ${pluralize(givenUp, "title")}`, tone: "onair" };
  }
  if (journey.milestone === "building") {
    return { tab: "in-progress", line: "Building your channel", tone: "caution" };
  }
  return { tab: "done", line: "On your channel", tone: "lock" };
};

// The ONE thing a requester can do about a request that failed. The server decides what is allowed
// (`actions`); this only names it. Editing wins over a plain retry because an edit is what fixes a
// request that would fail the same way again. Both resume the same Guide flow.
const requestFixLabel = (journey: ProposalJourneyDTO): string | undefined => {
  if (journey.actions.includes("edit")) {
    return journey.failure?.recoveryAction === "edit_reference" ? "Edit reference" : "Edit and try again";
  }
  if (journey.actions.includes("retry")) return "Try again";
  return undefined;
};

export { requestAcquisitions, requestFixLabel, requestStatus };
