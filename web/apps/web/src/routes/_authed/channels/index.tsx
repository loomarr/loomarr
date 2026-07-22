import { channelsApi } from "@loomarr/api";
import { formatEpgTime } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { channelHealth, channelOnAir } from "@/channels";
import { ChannelCard, EmptyState, ErrorState } from "@/components/loomarr";
import { useLoomarrEventListener } from "@/events";

// Channels (§12) — the "is my TV working" screen. Everything here is Loomarr's own state
// except now/next, which is Tunarr's (§6): one call serves every card, so the list costs
// one upstream request no matter how many channels exist. There is NO manual "refresh" or
// "rebuild" button (§9 self-maintaining): a background reconcile — from an edit, the sweep,
// or a title landing — emits a `channel` SSE frame, and the listener below live-invalidates
// both queries so the page updates on its own.
const ChannelsScreen = () => {
  const queryClient = useQueryClient();
  const channels = channelsApi.useListChannels();
  const nowNext = channelsApi.useChannelsNowNext({ query: { retry: false } });

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
      <header className="border-border border-b px-6 py-4">
        <h1 className="font-semibold text-xl">Channels</h1>
        <p className="mt-1 text-muted-foreground text-sm">
          Every channel Loomarr manages, and what's on right now.
        </p>
      </header>

      <div className="flex-1 overflow-auto p-6">
        {rows.length === 0 && !channels.isLoading ? (
          // Every list gets exactly one next action (§6) — and the only way to make a
          // channel is to describe one.
          <EmptyState
            title="Dead air"
            description="No channels yet. Describe the channel you want and Loomarr builds the lineup."
            action={{ label: "Suggest a channel" }}
          />
        ) : (
          <ul className="flex flex-col gap-3">
            {rows.map((ch) => {
              const nn = byChannel.get(ch.id);
              return (
                <li key={ch.id}>
                  {/* The whole card is the link to the channel's page (where all editing +
                      refine lives) — there is no per-row action button; edits apply
                      seamlessly and this list live-updates. Hover feedback lives on the
                      Link, not ChannelCard (the card is reused non-clickably on the detail
                      page). cursor-pointer because a <Link> gets no pointer by default. */}
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
