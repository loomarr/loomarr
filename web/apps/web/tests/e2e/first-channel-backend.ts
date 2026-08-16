import type {
  ChannelDTO,
  ChannelTracksOutputBody,
  GuideOutputBody,
  Intent,
  NowNextOutputBody,
  PoolDTO,
  ProposalJobDTO,
  TimelineOutputBody,
} from "@loomarr/api";
import type { Page, Route } from "@playwright/test";
import { installMockBackend, type MockOptions } from "./mock-backend";

interface FirstChannelOptions extends Pick<MockOptions, "role"> {
  fillerEligible?: number;
  initialJobs?: Record<string, ProposalJobDTO[]>;
  submittedJobs?: ProposalJobDTO[][];
}

interface FirstChannelBackend {
  readonly state: {
    approvals: string[];
    channelStatus: ChannelDTO["status"];
    eventConnections: number;
    hlsRequests: Record<PlaybackPhase, string[]>;
    jobReads: Record<string, number>;
    playURLRequests: Record<PlaybackPhase, number>;
    submissions: Intent[];
  };
  goLive: () => void;
}

type PlaybackPhase = "building" | "live";

const CHANNEL = {
  id: "ch-new",
  name: "Saturday Morning Cartoons",
  number: 42,
  strategy: "sequential",
  revision: 1,
  lineup: [{ key: "series:tmdb:2429", name: "Animaniacs", year: 1993, state: "available" }],
  policy: {},
  programCount: 1,
  pendingCount: 1,
  breakCount: 0,
  slotCount: 2,
} satisfies Omit<ChannelDTO, "inAppPlayable" | "status">;

const json = (route: Route, body: unknown, status = 200) =>
  route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });

// Contract-typed stateful backend for the first-channel acceptance journey. It layers only the
// relevant generated endpoint shapes over the repository's shared authenticated app backend.
// A job sequence advances on each authoritative GET, which lets Playwright prove polling and
// reload recovery while the SSE endpoint stays deliberately unavailable.
const installFirstChannelBackend = async (
  page: Page,
  opts: FirstChannelOptions = {},
): Promise<FirstChannelBackend> => {
  await installMockBackend(page, { authed: true, role: opts.role ?? "admin" });

  const jobSequences = new Map<string, ProposalJobDTO[]>();
  for (const [id, sequence] of Object.entries(opts.initialJobs ?? {})) {
    jobSequences.set(id, [...sequence]);
  }
  const submittedJobs = [...(opts.submittedJobs ?? [])];
  const state = {
    approvals: [] as string[],
    channelStatus: "building" as ChannelDTO["status"],
    eventConnections: 0,
    hlsRequests: { building: [], live: [] } as Record<PlaybackPhase, string[]>,
    jobReads: {} as Record<string, number>,
    playURLRequests: { building: 0, live: 0 } as Record<PlaybackPhase, number>,
    submissions: [] as Intent[],
  };
  const phase = (): PlaybackPhase => (state.channelStatus === "live" ? "live" : "building");
  const channel = (): ChannelDTO => ({
    ...CHANNEL,
    status: state.channelStatus,
    inAppPlayable: state.channelStatus === "live",
  });

  page.on("request", (request) => {
    const pathname = new URL(request.url()).pathname;
    if (pathname.endsWith(".m3u8") || pathname.includes("/hls/")) {
      state.hlsRequests[phase()].push(pathname);
    }
  });

  await page.route("**/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const method = request.method();

    if (path === "/v1/events") {
      state.eventConnections++;
      return route.abort();
    }

    if (path === "/v1/guide" && method === "GET") {
      const now = Date.now();
      const body = { channels: [], fromMs: now, toMs: now + 14_400_000 } satisfies GuideOutputBody;
      return json(route, body);
    }

    if (path === "/v1/proposals" && method === "POST") {
      const intent = JSON.parse(request.postData() ?? "{}") as Intent;
      state.submissions.push(intent);
      const sequence = submittedJobs.shift();
      if (!sequence?.[0]) return json(route, { title: "No submitted job fixture" }, 500);
      jobSequences.set(sequence[0].jobId, [...sequence]);
      return json(route, { jobId: sequence[0].jobId });
    }

    const jobMatch = /^\/v1\/proposal-jobs\/([^/]+)$/.exec(path);
    if (jobMatch && method === "GET") {
      const id = decodeURIComponent(jobMatch[1] ?? "");
      const sequence = jobSequences.get(id);
      if (!sequence?.[0]) return json(route, { title: "Proposal Job not found" }, 404);
      const read = state.jobReads[id] ?? 0;
      state.jobReads[id] = read + 1;
      return json(route, sequence[Math.min(read, sequence.length - 1)]);
    }

    const approveMatch = /^\/v1\/proposals\/([^/]+)\/approve$/.exec(path);
    if (approveMatch && method === "POST") {
      const id = decodeURIComponent(approveMatch[1] ?? "");
      state.approvals.push(id);
      return json(route, { channelId: CHANNEL.id, enqueued: 1, status: "approved" });
    }

    if (path === "/v1/filler/pool" && method === "GET") {
      const eligible = opts.fillerEligible ?? 0;
      const body = {
        channels: [],
        clips: eligible,
        commercials: eligible,
        eligible,
        untagged: 0,
      } satisfies PoolDTO;
      return json(route, body);
    }

    if (path === "/v1/channels" && method === "GET") {
      return json(route, { channels: state.approvals.length > 0 ? [channel()] : [] });
    }
    if (path === `/v1/channels/${CHANNEL.id}` && method === "GET") {
      return json(route, channel());
    }
    if (path === "/v1/channels/now-next" && method === "GET") {
      return json(route, { channels: [] } satisfies NowNextOutputBody);
    }
    if (path === `/v1/channels/${CHANNEL.id}/tracks` && method === "GET") {
      return json(route, { audio: [], subtitles: [] } satisfies ChannelTracksOutputBody);
    }
    if (path === `/v1/channels/${CHANNEL.id}/timeline` && method === "GET") {
      return json(route, { airings: [] } satisfies TimelineOutputBody);
    }
    if (path === `/v1/channels/${CHANNEL.id}/play-url` && method === "POST") {
      state.playURLRequests[phase()]++;
      return json(route, {
        url: "https://stream.invalid/hls/ch-new/index.m3u8",
        relativeUrl: "/hls/ch-new/index.m3u8",
        expiresAt: "2026-08-15T13:00:00Z",
      });
    }

    return route.fallback();
  });

  return {
    state,
    goLive: () => {
      state.channelStatus = "live";
    },
  };
};

export type { FirstChannelBackend, FirstChannelOptions };
export { installFirstChannelBackend };
