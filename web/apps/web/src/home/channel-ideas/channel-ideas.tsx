import * as discoveryApi from "@loomarr/api/endpoints/discovery";
import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import type { ChannelIdeaDTO } from "@loomarr/api/models/channelIdeaDTO";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { ideaAvailability, ideaCount, ideaReason } from "@loomarr/core/requests";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { RefreshCcw, X } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { SectionHeader, SectionHeaderAction } from "@/components/ui/section-header";
import { StatusDot } from "@/components/ui/status-dot";
import { cn } from "@/lib/utils";
import type { ChannelIdeasProps, IdeaCardProps } from "./channel-ideas.type";

const PAGE = 3;
const POSTERS = 4;
// IdeaCard — one channel idea (#1659 web mock): four posters, the reason, name and pitch, what it
// holds, then Request and Hide. A requested idea keeps its card and says it's waiting for an admin.
// There's no Undo there yet: withdrawing a member's request has no API.
const IdeaCard = ({ idea, empty, nowMs, busy, onRequest, onHide }: IdeaCardProps) => {
  const nameId = `idea-${idea.id}`;
  return (
    <article
      aria-labelledby={nameId}
      className={cn(
        "flex min-w-0 flex-col overflow-hidden rounded-lg border bg-static-900",
        idea.requested ? "border-suggest-tint-40" : "border-static-700",
      )}
    >
      {/* Poster slots. Titles carry no artwork yet, so they're the mock's empty tiles: at a quarter
          of a card the artwork frame's "No artwork" label doesn't fit, and the text below says
          what the idea is. */}
      <div aria-hidden className="grid grid-cols-4 gap-1.5 px-3 pt-3">
        {idea.keys.slice(0, POSTERS).map((key) => (
          <div key={key} className="aspect-[2/3] rounded-[6px] border border-static-700 bg-static-800" />
        ))}
      </div>
      <div className="flex flex-col gap-1 px-4 pt-3.5">
        <span className="text-static-400 text-xs">{ideaReason(idea, empty, nowMs)}</span>
        <h3 id={nameId} className="m-0 font-semibold text-base leading-[1.3]">
          {idea.name}
        </h3>
        <p className="m-0 text-pretty text-[13px] text-static-400">{idea.pitch}</p>
      </div>
      <div className="flex items-center gap-2 px-4 pt-2.5 text-xs">
        {/* The availability text beside it says what the dot does. */}
        <StatusDot tone={idea.toDownload > 0 ? "progress" : "ok"} label="" className="size-1.5" />
        <span>
          {ideaCount(idea)}
          <span className="text-static-400"> · {ideaAvailability(idea)}</span>
        </span>
      </div>
      <div className="mt-auto p-4">
        {idea.requested ? (
          <div className="flex min-h-8 items-center">
            <Badge variant="suggest">Waiting for an admin</Badge>
          </div>
        ) : (
          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm" disabled={busy} onClick={onRequest}>
              Request channel
            </Button>
            {/* Opens the manual builder preloaded with this idea's own lineup (#1817) — the
                shipped idea card named this as waiting on the mock; #1872 is the approved one. */}
            <Link
              to="/guide"
              search={{ manual: "1", ideaId: idea.id }}
              className={cn(buttonVariants({ size: "sm", variant: "ghost" }))}
            >
              Edit first
            </Link>
            <Button
              size="sm"
              variant="ghost"
              aria-label={`Not for me: ${idea.name}`}
              title="Not for me"
              disabled={busy}
              onClick={onHide}
              className="ml-auto w-8 px-0 text-static-400 hover:bg-static-800 hover:text-foreground"
            >
              <X aria-hidden />
            </Button>
          </div>
        )}
      </div>
    </article>
  );
};

// ChannelIdeas — the member's "Channel ideas" on Home (#1659 web mock, H4): channels their library
// could make, built without the LLM (#1665, #1720). Three at a time, "Different ideas" pages
// through the rest, Hide keeps an Undo in the header, and Request puts the idea in the approval
// queue. "Edit first" and "Describe your own channel" (#1817, #1872) both open the Guide's
// manual builder — the former preloaded with this idea's own lineup, the latter blank.
const ChannelIdeas = ({ empty, nowMs }: ChannelIdeasProps) => {
  const queryClient = useQueryClient();
  const list = discoveryApi.useListChannelIdeas();
  const [page, setPage] = useState(0);
  const [hidden, setHidden] = useState<ChannelIdeaDTO>();
  const [busyId, setBusyId] = useState<string>();

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: discoveryApi.getListChannelIdeasQueryKey() });
  const settle = { onSettled: () => setBusyId(undefined) };
  const request = discoveryApi.useRequestChannelIdea({
    mutation: {
      ...settle,
      onSuccess: () => {
        void refresh();
        // The request joins Your requests and the Requests page.
        void queryClient.invalidateQueries({ queryKey: proposalJobsApi.getListProposalJobsQueryKey() });
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't request that channel"),
    },
  });
  const hide = discoveryApi.useHideChannelIdea({
    mutation: { ...settle, onError: (e) => toast.error(toProblem(e).title ?? "Couldn't hide that idea") },
  });
  const unhide = discoveryApi.useUnhideChannelIdea({
    mutation: {
      onSuccess: () => {
        setHidden(undefined);
        void refresh();
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't bring that idea back"),
    },
  });

  if (!list.isSuccess) return null;
  const ideas = unwrap(list.data, (b) => b.ideas) ?? [];
  const start = ideas.length > 0 ? (page * PAGE) % ideas.length : 0;
  const shown = [...ideas.slice(start), ...ideas.slice(0, start)].slice(0, PAGE);

  return (
    <section aria-labelledby="home-ideas">
      <SectionHeader
        id="home-ideas"
        title="Channel ideas"
        meta={
          empty
            ? "Nothing’s on yet. Start with one of these, built from your library."
            : "Picked from your library"
        }
      >
        {hidden && (
          <>
            <span className="text-static-400 text-xs">Hid {hidden.name}</span>
            <button
              type="button"
              disabled={unhide.isPending}
              onClick={() => unhide.mutate({ ideaId: hidden.id })}
              className="cursor-pointer border-none bg-transparent p-0 text-foreground text-xs underline underline-offset-[3px] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              Undo
            </button>
          </>
        )}
        {ideas.length > PAGE && (
          <SectionHeaderAction
            icon={<RefreshCcw aria-hidden className="size-3.5" />}
            onClick={() => setPage((p) => p + 1)}
          >
            Different ideas
          </SectionHeaderAction>
        )}
      </SectionHeader>
      {ideas.length === 0 ? (
        <p className="m-0 rounded-lg border border-static-700 bg-static-900 px-4 py-5 text-[13px] text-static-400">
          That’s every idea for now. New ones appear as your library grows.
        </p>
      ) : (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(272px,1fr))] gap-3">
          {shown.map((idea) => (
            <IdeaCard
              key={idea.id}
              idea={idea}
              empty={empty}
              nowMs={nowMs}
              busy={busyId === idea.id}
              onRequest={() => {
                setBusyId(idea.id);
                request.mutate({ ideaId: idea.id });
              }}
              onHide={() => {
                setBusyId(idea.id);
                hide.mutate(
                  { ideaId: idea.id },
                  {
                    onSuccess: () => {
                      setHidden(idea);
                      void refresh();
                    },
                  },
                );
              }}
            />
          ))}
        </div>
      )}
      {/* Describe your own channel (#1817): the same manual builder "Edit first" opens, started
          blank. The shipped grid above already names this as waiting on the mock; #1872 is the
          approved one. */}
      <div className="mt-3 flex items-center justify-center rounded-lg border border-static-700 border-dashed p-3.5">
        <Link to="/guide" search={{ manual: "1" }} className={cn(buttonVariants({ variant: "ghost" }))}>
          Don’t see it? Describe your own channel
        </Link>
      </div>
    </section>
  );
};

export { ChannelIdeas };
