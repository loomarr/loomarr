import * as titlesApi from "@loomarr/api/endpoints/titles";
import type { TitleDTO } from "@loomarr/api/models/titleDTO";
import { TitleDTOState } from "@loomarr/api/models/titleDTOState";
import { unwrap } from "@loomarr/api/unwrap";
import type { RequestStatus } from "@loomarr/core/requests";
import { useQueries } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { ListGroup, ListRow, type ListRowProps } from "@/components/ui/list-row";
import { SectionHeader, SectionHeaderAction } from "@/components/ui/section-header";
import type { StatusTone } from "@/components/ui/status-dot";
import { type RequestEntry, useRequests } from "@/queue/use-requests";

const IN_FLIGHT = [TitleDTOState.downloading, TitleDTOState.requested, TitleDTOState.wanted] as const;

// "For Late Night Sci-Fi · 8 of 36 episodes" (the mock). A title not downloading yet is waiting
// for the download client to take it.
const titleRow = (t: TitleDTO): Pick<ListRowProps, "title" | "sub" | "progress"> => {
  const downloading = t.state === TitleDTOState.downloading && t.progress !== undefined;
  const sub = [
    t.channels?.[0] ? `For ${t.channels[0].name}` : undefined,
    t.episodesHave !== undefined && t.episodesWanted
      ? `${t.episodesHave} of ${t.episodesWanted} episodes`
      : undefined,
    downloading ? undefined : "waiting for a download slot",
  ]
    .filter(Boolean)
    .join(" · ");
  return {
    title: t.name ?? t.key,
    sub,
    progress: downloading
      ? { value: Math.round((t.progress ?? 0) * 100), label: "Downloading", eta: t.etaText }
      : { label: "Waiting to download", eta: "queued" },
  };
};

const SectionFrame = ({ title, children }: { title: string; children: React.ReactNode }) => (
  <section aria-labelledby="home-way">
    <SectionHeader id="home-way" title={title}>
      <SectionHeaderAction render={<Link to="/requests/in-progress" />}>All requests</SectionHeaderAction>
    </SectionHeader>
    <ListGroup aria-labelledby="home-way">{children}</ListGroup>
  </section>
);

// OnTheWay — the admin's "On the way" (#1659 web mock): what is downloading for which channel,
// with progress and the download client's own time left. Same per-state fan-out as the Requests
// page, since GET /v1/titles filters by one state.
const OnTheWay = () => {
  const queries = useQueries({
    queries: IN_FLIGHT.map((state) => titlesApi.getListTitlesQueryOptions({ state })),
  });
  const titles = queries.flatMap((q) => unwrap(q.data, (b) => b.titles) ?? []);
  if (titles.length === 0) return null;
  return (
    <SectionFrame title="On the way">
      {titles.map((t) => (
        <ListRow key={t.key} tone="progress" {...titleRow(t)} />
      ))}
    </SectionFrame>
  );
};

// The Requests page's badge tones, as the mock's row dots.
const ROW_TONE: Record<RequestStatus["tone"], StatusTone> = {
  suggest: "attention",
  lock: "ok",
  onair: "error",
  caution: "progress",
};

const requestAction = ({ journey, status }: RequestEntry) => {
  if (status.tab === "needs-you") {
    return (
      <Button size="sm" variant="outline" render={<Link to="/requests/needs-you" />}>
        Edit and retry
      </Button>
    );
  }
  if (status.tone === "lock" && journey.channel) {
    return (
      <Button
        size="sm"
        variant="outline"
        render={<Link to="/channels/$id/watch" params={{ id: journey.channel.id }} />}
      >
        Watch
      </Button>
    );
  }
  return undefined;
};

// YourRequests — the member's "Your requests" (#1659 web mock): each request with the same status
// line the Requests page shows, and its one next step. Finished requests stay only while their
// channel is on the air; the rest live under Requests › Done.
const YourRequests = () => {
  const { entries } = useRequests();
  const shown = entries.filter((e) => e.status.tab !== "done" || e.status.tone === "lock");
  if (shown.length === 0) return null;
  return (
    <SectionFrame title="Your requests">
      {shown.map((entry) => (
        <ListRow
          key={entry.journey.jobId}
          tone={ROW_TONE[entry.status.tone]}
          title={entry.journey.intent.description}
          sub={entry.status.detail ? `${entry.status.line}: ${entry.status.detail}` : entry.status.line}
          action={requestAction(entry)}
        />
      ))}
    </SectionFrame>
  );
};

export { OnTheWay, titleRow, YourRequests };
