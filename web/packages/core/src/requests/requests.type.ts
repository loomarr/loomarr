import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { ChannelIdeaDTO } from "@loomarr/api/models/channelIdeaDTO";
import type { Intent } from "@loomarr/api/models/intent";
import type { MeBody } from "@loomarr/api/models/meBody";
import type { ProposalDTO } from "@loomarr/api/models/proposalDTO";
import type { ProposalJourneyDTO } from "@loomarr/api/models/proposalJourneyDTO";
import type { PullDTO } from "@loomarr/api/models/pullDTO";
import type { TitleDTO } from "@loomarr/api/models/titleDTO";

import type { RequestStatus } from "./request-status/request-status.type";

type RequestsStatus = "error" | "loading" | "ready";
type RequestsRole = MeBody["role"];

/** One id's outcome in a bulk approve: `BulkApproveResult`'s `{id, ok, error}`, the part a phone reads. */
interface ApprovalResult {
  error?: string;
  id: string;
  ok: boolean;
}

/** What a bulk approve returns, from the proposals endpoint or from filler pulls approved one by one. */
interface BulkOutcome {
  approved: number;
  results: ApprovalResult[];
}

/** One of the caller's own requests with the single status line every surface reads. */
interface RequestEntry {
  journey: ProposalJourneyDTO;
  status: RequestStatus;
}

interface RequestsSnapshot {
  /** Proposals waiting on an admin's decision. Always empty for a member. */
  approvals: readonly ProposalDTO[];
  /** Proposal ids with a decision in flight. */
  deciding: readonly string[];
  entries: readonly RequestEntry[];
  /** The Needs-you list could not load; the sentence is the server's, never invented. */
  errorMessage?: string;
  /** Filler downloads waiting on an admin's decision, a separate group from channel requests. Empty for a member. */
  fillerPulls: readonly PullDTO[];
  /** The idea the person just hid, kept so Undo can bring it back. */
  hiddenIdea?: ChannelIdeaDTO;
  ideas: readonly ChannelIdeaDTO[];
  ideasStatus: RequestsStatus;
  /** The idea whose request is in flight. */
  requestingIdeaId?: string;
  /** A refused action's sentence, shown once until dismissed. */
  notice?: string;
  /** The role from `/v1/auth/me`: it gates every admin control, as on Web. */
  role: RequestsRole;
  status: RequestsStatus;
  titles: readonly TitleDTO[];
}

/** The routes Web's Requests uses, over any authenticated fetch. Every method throws on a refusal. */
interface RequestsPort {
  /** An edit rides the same approve call: it is a parameter to the approval gate, not a separate save. */
  approve: (proposalId: string, edit?: ApprovalEditDTO) => Promise<{ channelId: string }>;
  /**
   * Approves each pull with `POST /v1/filler/pulls/{id}/approve` and reports every id's outcome in
   * `BulkApproveResult`'s shape. There is no bulk endpoint yet; when one lands, this is the one
   * method that changes. A refused pull is a result, not a throw, so the rest still go through.
   */
  approveFillerPulls: (pullIds: readonly string[]) => Promise<BulkOutcome>;
  approveMany: (proposalIds: readonly string[]) => Promise<BulkOutcome>;
  deny: (proposalId: string, reason?: string) => Promise<void>;
  dismissFillerPull: (pullId: string) => Promise<void>;
  hideIdea: (ideaId: string) => Promise<void>;
  loadApprovals: (signal: AbortSignal) => Promise<ProposalDTO[]>;
  loadFillerPulls: (signal: AbortSignal) => Promise<PullDTO[]>;
  loadIdeas: (signal: AbortSignal) => Promise<ChannelIdeaDTO[]>;
  loadJourneys: (signal: AbortSignal) => Promise<ProposalJourneyDTO[]>;
  loadMe: (signal: AbortSignal) => Promise<MeBody>;
  /** `GET /v1/titles` is a single-state filter, so this is every state merged. */
  loadTitles: (signal: AbortSignal) => Promise<TitleDTO[]>;
  requestIdea: (ideaId: string) => Promise<{ jobId: string }>;
  submitBrief: (intent: Intent) => Promise<{ jobId: string }>;
  unhideIdea: (ideaId: string) => Promise<void>;
}

type SubmitBriefResult =
  | { kind: "failed"; message: string }
  | { kind: "started"; jobId: string }
  /** AI or TMDB grounding isn't set up: the brief is kept and only an administrator can fix it. */
  | { kind: "unavailable"; reason: "ai" | "grounding" };

type DecisionResult = { kind: "failed"; message: string } | { kind: "done"; channelId?: string };

/** A bulk approve either ran, with every id's outcome (some may be refused), or could not run at all. */
type BulkDecision = ({ kind: "done" } & BulkOutcome) | { kind: "failed"; message: string };

interface RequestsController {
  approve: (proposalId: string, edit?: ApprovalEditDTO) => Promise<DecisionResult>;
  approveFillerPull: (pullId: string) => Promise<DecisionResult>;
  approveFillerPulls: (pullIds: readonly string[]) => Promise<BulkDecision>;
  approveMany: (proposalIds: readonly string[]) => Promise<BulkDecision>;
  deny: (proposalId: string, reason?: string) => Promise<DecisionResult>;
  dismissFillerPull: (pullId: string) => Promise<DecisionResult>;
  dismissNotice: () => void;
  dispose: () => void;
  getSnapshot: () => RequestsSnapshot;
  hideIdea: (ideaId: string) => Promise<void>;
  /** Reads the requests; while one is generating it re-reads every two seconds until none is. */
  refresh: () => Promise<void>;
  refreshIdeas: () => Promise<void>;
  requestIdea: (ideaId: string) => Promise<void>;
  submitBrief: (intent: Intent) => Promise<SubmitBriefResult>;
  subscribe: (listener: () => void) => () => void;
  undoHideIdea: () => Promise<void>;
}

// The generated wire types the screens read, re-exported so a native package depends on core alone.
export type {
  ApprovalEditDTO,
  ApprovalResult,
  BulkDecision,
  BulkOutcome,
  ChannelIdeaDTO,
  DecisionResult,
  Intent,
  ProposalDTO,
  ProposalJourneyDTO,
  PullDTO,
  RequestEntry,
  RequestsController,
  RequestsPort,
  RequestsRole,
  RequestsSnapshot,
  RequestsStatus,
  SubmitBriefResult,
  TitleDTO,
};
