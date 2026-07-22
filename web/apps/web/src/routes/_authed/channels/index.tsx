import { channelsApi } from "@loomarr/api";
import { formatEpgTime } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { Plus, Sparkles } from "lucide-react";
import { useState } from "react";
import { useAuth } from "@/auth";
import { ChannelCreateDialog, ChannelRowMenu, channelHealth, channelOnAir } from "@/channels";
import { ChannelCard, EmptyState, ErrorState } from "@/components/loomarr";
import { Button } from "@/components/ui";
import { useLoomarrEventListener } from "@/events";

// Channels (§12) — the "is my TV working" screen. Everything here is Loomarr's own state
// except now/next, which is Tunarr's (§6): one call serves every card, so the list costs
// one upstream request no matter how many channels exist. There is NO manual "refresh" or
// "rebuild" button (§9 self-maintaining): a background reconcile — from an edit, the sweep,
// or a title landing — emits a `channel` SSE frame, and the listener below live-invalidates
// both queries so the page updates on its own.
const ChannelsScreen = () => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { isAdmin } = useAuth();
  const [creating, setCreating] = useState(false);
  const channels = channelsApi.useListChannels();
  const nowNext = channelsApi.useChannelsNowNext({ query: { retry: false } });

  const goSuggest = () => navigate({ to: "/suggest" });
  // A freshly-created channel is empty — drop the user on its page, where Refine-with-AI and
  // the manual lineup editor fill it (the "create shell → then Suggest" flow).
  const onCreated = (id: string) => {
    setCreating(false);
    void queryClient.invalidateQueries({ queryKey: channelsApi.getListChannelsQueryKey() });
    void navigate({ to: "/channels/$id", params: { id } });
  };

  // Live update: any `channel` frame refreshes the list + guide (no refresh button).
  useLoomarrEventListener({
    onChannel: () => {
      void queryClient.invalidateQueries({ queryKey: channelsApi.getListChannelsQueryKey() });
      void queryClient.invalidateQueries({ queryKey: channelsApi.getChannelsNowNextQueryKey() });
    },
  });

  const rows = channels.data?.status === 200 ? (channels.data.data.channels ?? []) : [];
  // Absent now/next is normal (no Tunarr yet, guide not generated) — the card just omits
  // the strip rather than the page failing.
  const byChannel = new Map(
    (nowNext.data?.status === 200 ? (nowNext.data.data.channels ?? []) : []).map((n) => [n.channelId, n]),
  );

  if (channels.error) {
    return (
      <div className="p-6">
        <ErrorState error={channels.error} onRetry={() => channels.refetch()} />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      <header className="flex flex-wrap items-start justify-between gap-3 border-border border-b px-6 py-4">
        <div>
          <h1 className="font-semibold text-xl">Channels</h1>
          <p className="mt-1 text-muted-foreground text-sm">
            Every channel Loomarr manages, and what's on right now.
          </p>
        </div>
        {/* Admin actions. Suggest is the prominent, headline path (describe a channel →
            build it); New channel is the direct hand-made alternative. Both always visible,
            not just when the list is empty. */}
        {isAdmin && (
          <div className="flex gap-2">
            <Button variant="suggest" size="sm" onClick={goSuggest}>
              <Sparkles aria-hidden />
              Suggest a channel
            </Button>
            <Button variant="outline" size="sm" onClick={() => setCreating(true)}>
              <Plus aria-hidden />
              New channel
            </Button>
          </div>
        )}
      </header>

      <div className="flex-1 overflow-auto p-6">
        {creating && (
          <div className="mb-4">
            <ChannelCreateDialog onCreated={onCreated} onClose={() => setCreating(false)} />
          </div>
        )}
        {rows.length === 0 && !channels.isLoading ? (
          // Every list gets exactly one next action (§6). Describing a channel is the
          // headline path, so the empty state's hero action is Suggest (now wired) — the
          // "New channel" hand-made path lives in the header for when you'd rather build
          // one directly.
          <EmptyState
            title="Dead air"
            description="No channels yet. Describe the channel you want and Loomarr builds the lineup."
            {...(isAdmin ? { action: { label: "Suggest a channel", onClick: goSuggest } } : {})}
          />
        ) : (
          <ul className="flex flex-col gap-3">
            {rows.map((ch) => {
              const nn = byChannel.get(ch.id);
              return (
                <li key={ch.id} className="relative">
                  {/* The whole card is the link to the channel's page (where all editing +
                      refine lives). The per-row ⋮ menu (admin) overlays the top-right corner
                      as a SIBLING, not a child, and swallows its own clicks so pause/delete
                      don't also follow the link. Edits still apply seamlessly + the list
                      live-updates. cursor-pointer because a <Link> gets no pointer by default. */}
                  <Link
                    to="/channels/$id"
                    params={{ id: ch.id }}
                    className="block cursor-pointer rounded-lg outline-none ring-signal/0 transition-[box-shadow] hover:ring-2 hover:ring-static-600 focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <ChannelCard
                      number={ch.number}
                      name={ch.name}
                      onAir={channelOnAir(ch)}
                      health={channelHealth(ch)}
                      managed
                      nowNext={
                        nn && {
                          // `until` is what makes the strip read like a guide ("Cheers
                          // ·til 8:00 PM"). NowNextEntry has carried stopMs all along;
                          // omitting it left the affordance rendering nothing.
                          now: nn.now
                            ? { title: nn.now.title, until: formatEpgTime(nn.now.stopMs) }
                            : undefined,
                          next: nn.next ? { title: nn.next.title } : undefined,
                          gap: nn.now?.gap,
                        }
                      }
                    />
                  </Link>
                  {isAdmin && (
                    <div className="absolute top-3 right-3">
                      <ChannelRowMenu channel={{ id: ch.id, name: ch.name, status: ch.status }} />
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
};

const Route = createFileRoute("/_authed/channels/")({
  component: ChannelsScreen,
});

export { Route };
