import { channelsApi } from "@loomarr/api";
import { formatEpgTime } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { Sparkles, X } from "lucide-react";
import { useState } from "react";
import { useAuth } from "@/auth";
import { ChannelRowMenu, channelHealth, channelOnAir } from "@/channels";
import { ChannelCard, EmptyState, ErrorState } from "@/components/loomarr";
import { Button } from "@/components/ui";
import { useLoomarrEventListener } from "@/events";
import { useDocumentTitle } from "@/lib";
import { ChannelSuggestPanel } from "@/suggest";

// Channels (§12) — the "is my TV working" screen. Everything here is Loomarr's own state
// except now/next, which is Tunarr's (§6): one call serves every card, so the list costs
// one upstream request no matter how many channels exist. There is NO manual "refresh" or
// "rebuild" button (§9 self-maintaining): a background reconcile — from an edit, the sweep,
// or a title landing — emits a `channel` SSE frame, and the listener below live-invalidates
// both queries so the page updates on its own.
const ChannelsScreen = () => {
  useDocumentTitle("Channels");
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { isAdmin } = useAuth();
  // The inline "describe a channel" surface. Creating a channel IS describing one (Suggest),
  // so the create path is the ChannelSuggestPanel expanded in place — no separate empty-shell
  // dialog. `adding` toggles it open.
  const [adding, setAdding] = useState(false);
  const channels = channelsApi.useListChannels();
  const nowNext = channelsApi.useChannelsNowNext({ query: { retry: false } });

  // The inline panel approved a proposal, which created the channel — drop the operator on
  // its page, and refresh the list behind them.
  const onCreated = (id: string) => {
    setAdding(false);
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
        {/* Admin: one prominent create action — describe a channel, inline. It toggles the
            ChannelSuggestPanel open below (the create path IS Suggest, integrated here). */}
        {isAdmin && (
          <Button
            variant={adding ? "outline" : "suggest"}
            size="sm"
            onClick={() => setAdding((v) => !v)}
            aria-expanded={adding}
          >
            {adding ? <X aria-hidden /> : <Sparkles aria-hidden />}
            {adding ? "Close" : "Add a channel"}
          </Button>
        )}
      </header>

      <div className="flex-1 overflow-auto p-6">
        {/* The inline create surface — describe a channel, review, approve, land on it. */}
        {isAdmin && adding && (
          <div className="mb-6">
            <ChannelSuggestPanel onCreated={onCreated} />
          </div>
        )}
        {rows.length === 0 && !channels.isLoading ? (
          // Every list gets exactly one next action (§6): describe your first channel, which
          // opens the same inline Suggest surface the header toggles.
          <EmptyState
            title="Dead air"
            description="No channels yet. Describe the channel you want and Loomarr builds the lineup."
            {...(isAdmin
              ? { action: { label: "Describe your first channel", onClick: () => setAdding(true) } }
              : {})}
          />
        ) : (
          <ul className="flex flex-col gap-3">
            {rows.map((ch) => {
              const nn = byChannel.get(ch.id);
              return (
                <li key={ch.id}>
                  {/* The whole card links to the channel's page (where all editing + refine
                      lives). The per-row ⋮ menu (admin) rides the card's `trailing` slot —
                      inside the card's top-right cluster, beside the on-air dot — so it reads
                      as part of the row, not adrift in a margin. It's still a real interactive
                      control inside a Link, so it swallows its own clicks (see ChannelRowMenu)
                      to keep pause/delete from also following the link. Edits apply seamlessly
                      and the list live-updates. cursor-pointer because a <Link> gets none. */}
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
                      trailing={
                        isAdmin ? (
                          <ChannelRowMenu channel={{ id: ch.id, name: ch.name, status: ch.status }} />
                        ) : undefined
                      }
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
