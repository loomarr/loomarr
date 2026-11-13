import * as discoveryApi from "@loomarr/api/endpoints/discovery";
import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import type { ChannelIdeaDTO } from "@loomarr/api/models/channelIdeaDTO";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import { ideaAvailability, ideaCount, ideaReason } from "@loomarr/core/requests";
import {
  Action,
  Badge,
  Card,
  SectionHeader,
  Stack,
  StatusDot,
  Surface,
  Text,
  Tooltip,
} from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
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
        "bg-static-900 flex min-w-0 flex-col overflow-hidden rounded-lg border",
        idea.requested ? "border-suggest-tint-40" : "border-static-700",
      )}
    >
      {/* Poster slots. Titles carry no artwork yet, so they're the mock's empty tiles: at a quarter
          of a card the artwork frame's "No artwork" label doesn't fit, and the text below says
          what the idea is. */}
      <div aria-hidden className="grid grid-cols-4 gap-1.5 px-3 pt-3">
        {idea.keys.slice(0, POSTERS).map((key) => (
          <Stack
            aspectRatio={2 / 3}
            backgroundColor="surfaceElevated"
            borderColor="borderDecorative"
            borderRadius={6}
            borderWidth={1}
            key={key}
          />
        ))}
      </div>
      <Stack gap="1" paddingHorizontal={16} paddingTop={14}>
        <Text as="span" textRole="hint" tone="secondary">
          {ideaReason(idea, empty, nowMs)}
        </Text>
        <Text as="h3" id={nameId} textRole="heading">
          {idea.name}
        </Text>
        <Text as="p" className="text-pretty" textRole="compact" tone="secondary">
          {idea.pitch}
        </Text>
      </Stack>
      <Stack align="center" direction="row" gap="inline" paddingHorizontal={16} paddingTop={10}>
        {/* The availability text beside it says what the dot does. */}
        <StatusDot tone={idea.toDownload > 0 ? "progress" : "ok"} label="" size="compact" />
        <Text as="span" textRole="hint">
          {ideaCount(idea)}
          <Text as="span" textRole="hint" tone="secondary">
            {" "}
            · {ideaAvailability(idea)}
          </Text>
        </Text>
      </Stack>
      <Stack marginTop="auto" padding={16}>
        {idea.requested ? (
          <Stack align="center" direction="row" minHeight={32}>
            <Badge shape="square" tone="suggest">
              Waiting for an admin
            </Badge>
          </Stack>
        ) : (
          <Stack align="center" direction="row" gap="inline" wrap>
            <Action disabled={busy} onPress={onRequest}>
              Request channel
            </Action>
            {/* Opens the manual builder preloaded with this idea's own lineup (#1817) — the
                shipped idea card named this as waiting on the mock; #1872 is the approved one. */}
            <Action
              accessibilityRole="link"
              render={<Link to="/requests/new" search={{ manual: 1, ideaId: idea.id }} />}
              variant="ghost"
            >
              Edit first
            </Action>
            <Stack marginLeft="auto">
              <Tooltip content="Not for me">
                <Action disabled={busy} icon="close" layout="icon" onPress={onHide} variant="ghost">
                  {`Not for me: ${idea.name}`}
                </Action>
              </Tooltip>
            </Stack>
          </Stack>
        )}
      </Stack>
    </article>
  );
};

// ChannelIdeas — the member's "Channel ideas" on Home (#1659 web mock, H4): channels their library
// could make, built without the LLM (#1665, #1720). Three at a time, "Different ideas" pages
// through the rest, Hide keeps an Undo in the header, and Request puts the idea in the approval
// queue. "Edit first" and "Describe your own channel" (#1817, #1872) both open the Guide's
// manual builder — the former preloaded with this idea's own lineup, the latter blank.
const ChannelIdeas = ({ empty, inset = false, nowMs }: ChannelIdeasProps) => {
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
  // Home shows the section only when it has an idea (#2763). The one exception is the idea just
  // hidden: its Undo has to stay reachable until the member is done with it.
  if (ideas.length === 0 && !hidden) return null;
  const start = ideas.length > 0 ? (page * PAGE) % ideas.length : 0;
  const shown = [...ideas.slice(start), ...ideas.slice(0, start)].slice(0, PAGE);

  const section = (
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
            <Text as="span" textRole="hint" tone="secondary">
              Hid {hidden.name}
            </Text>
            <Action
              disabled={unhide.isPending}
              layout="text"
              onPress={() => unhide.mutate({ ideaId: hidden.id })}
              variant="ghost"
            >
              Undo
            </Action>
          </>
        )}
        {ideas.length > PAGE && (
          <Action onPress={() => setPage((p) => p + 1)} variant="ghost">
            Different ideas
          </Action>
        )}
      </SectionHeader>
      {ideas.length === 0 ? (
        <Card paddingHorizontal={16} paddingVertical={20}>
          <Text as="p" textRole="compact" tone="secondary">
            That’s every idea for now. New ones appear as your library grows.
          </Text>
        </Card>
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
      <Stack
        align="center"
        borderColor="borderDecorative"
        borderRadius={12}
        borderStyle="dashed"
        borderWidth={1}
        direction="row"
        justify="center"
        marginTop={12}
        padding={14}
      >
        <Action
          accessibilityRole="link"
          render={<Link to="/requests/new" search={{ manual: 1 }} />}
          variant="ghost"
        >
          Don’t see it? Describe your own channel
        </Action>
      </Stack>
    </section>
  );
  // On a page of its own the section brings its page inset; the inset is part of what renders, so
  // a page with no idea has no empty gap.
  return inset ? (
    <Surface backgroundColor="transparent" borderWidth={0} padding="section">
      {section}
    </Surface>
  ) : (
    section
  );
};

export { ChannelIdeas };
