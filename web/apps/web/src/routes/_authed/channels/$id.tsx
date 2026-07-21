import { type ChannelDTO, channelsApi } from "@loomarr/api";
import { formatEpgTime, pluralize } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowLeft, ChevronDown } from "lucide-react";
import { useState } from "react";
import { useAuth } from "@/auth";
import { useTunarrReady } from "@/channels";
import type { OnAirState } from "@/components/loomarr";
import { ErrorState, OnAirIndicator } from "@/components/loomarr";
import { Button, Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui";
import { ChannelAdvanced } from "./channel-advanced";

// Channel detail (§12). TWO AUDIENCES: the top answers a viewer's questions — is it on,
// what's playing, what's next, how full is it — in plain words. The scheduler internals
// (which programming rules were eased, the commercial-break breakdown, the Tunarr id) are
// real and useful for debugging, but they are the operator's concern, so they live in a
// collapsed admin-only "Advanced" section — never leaking codes like "reconcile" or a
// relaxation ladder into the viewer surface.

// airState is the ONE honest status, derived from ground truth rather than the stored
// `status` alone: a channel is only genuinely "on air" once it has been pushed to Tunarr
// (a TunarrID). `live` with no TunarrID is the impossible state that used to render as the
// contradictory "On air" + "Nothing scheduled"; here it reads as "building" instead.
// dot reuses the existing OnAirIndicator states (live=red pulse, reconciling=amber,
// off=muted); `label`/`detail` are the plain-language status a viewer reads.
type AirState = { dot: OnAirState; label: string; detail: string };

const airStateOf = (ch: ChannelDTO): AirState => {
  const pushed = Boolean(ch.tunarrId);
  if (ch.status === "detached") {
    return { dot: "off", label: "Off air", detail: "Loomarr no longer manages this channel." };
  }
  if (ch.status === "drifted") {
    return {
      dot: "reconciling",
      label: "On air (needs a rebuild)",
      detail: "Something changed since the last build — rebuild to bring it back in line.",
    };
  }
  if (ch.status === "live" && pushed) {
    return { dot: "live", label: "On air", detail: "Playing now in your TV guide." };
  }
  // building, or live-without-a-push (not yet broadcasting).
  return {
    dot: "reconciling",
    label: "Not on air yet",
    detail: pushed ? "Getting ready to broadcast." : "Rebuild to push it to your TV guide and go live.",
  };
};

const ChannelDetailScreen = () => {
  const { id } = Route.useParams();
  const { isAdmin } = useAuth();
  const queryClient = useQueryClient();
  const [showAdvanced, setShowAdvanced] = useState(false);

  const channel = channelsApi.useGetChannel(id);
  // Now/next is Tunarr's guide (§6) — absent until the channel is pushed, which is normal,
  // not an error, so it retries-false and simply shows an honest "nothing yet".
  const nowNext = channelsApi.useChannelsNowNext({ query: { retry: false } });
  // Whether a rebuild can even work: it pushes to Tunarr, so it needs tunarr.url set.
  const tunarrReady = useTunarrReady();

  const reconcile = channelsApi.useReconcileChannel({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: channelsApi.getGetChannelQueryKey(id) }),
    },
  });

  if (channel.error) {
    return (
      <div className="p-6">
        <ErrorState error={channel.error} onRetry={() => channel.refetch()} />
      </div>
    );
  }
  const ch = channel.data?.status === 200 ? channel.data.data : undefined;
  if (!ch) return <p className="p-6 text-muted-foreground text-sm">Loading channel…</p>;

  const air = airStateOf(ch);
  const guide =
    nowNext.data?.status === 200 ? nowNext.data.data.channels?.find((c) => c.channelId === id) : undefined;

  const ready = ch.programCount; // shows with a real program
  const total = ch.slotCount;

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center gap-3 border-border border-b px-6 py-4">
        <Tooltip>
          <TooltipTrigger asChild>
            <Link
              to="/channels"
              aria-label="Back to channels"
              className="cursor-pointer text-muted-foreground hover:text-foreground"
            >
              <ArrowLeft className="size-4" aria-hidden />
            </Link>
          </TooltipTrigger>
          <TooltipContent>Back to channels</TooltipContent>
        </Tooltip>
        <h1 className="font-semibold text-xl">{ch.name}</h1>
        <span className="ml-auto font-mono text-muted-foreground text-sm">Channel {ch.number}</span>
      </header>

      <div className="mx-auto flex w-full max-w-2xl flex-1 flex-col gap-6 overflow-auto p-6">
        {/* Status — the honest one-liner. */}
        <section className="flex items-start gap-3">
          <OnAirIndicator state={air.dot} className="mt-1.5" />
          <div>
            <p className="font-semibold text-lg">{air.label}</p>
            <p className="text-muted-foreground text-sm">{air.detail}</p>
          </div>
        </section>

        {/* What's on — a viewer's first question. */}
        <section className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4">
          <NowNextRow label="Now playing" entry={guide?.now} live={air.dot === "live"} />
          <div className="border-border border-t" />
          <NowNextRow label="Up next" entry={guide?.next} live={air.dot === "live"} />
        </section>

        {/* How full — plain words, no "slots/flex/filler" jargon. */}
        <section className="flex flex-col gap-1">
          <p className="text-sm">
            <span className="font-medium">{`${ready} of ${pluralize(total, "show")} ready`}</span>
            {ready < total && (
              <span className="text-muted-foreground">
                {" "}
                — the rest fill in with clips as they arrive, so the channel never goes dark.
              </span>
            )}
          </p>
        </section>

        {/* Rebuild — gated on Tunarr being connected AND admin, so it never invites a click
            that only 501s. A member sees no controls; an admin without Tunarr sees WHY. */}
        {isAdmin && (
          <section className="flex flex-col gap-2">
            <div className="flex items-center gap-3">
              {tunarrReady ? (
                <Button onClick={() => reconcile.mutate({ id })} disabled={reconcile.isPending}>
                  {reconcile.isPending ? "Rebuilding…" : "Rebuild"}
                </Button>
              ) : (
                <Tooltip>
                  <TooltipTrigger asChild>
                    {/* aria-disabled (not `disabled`) keeps the button focusable so the
                        tooltip reaches keyboard + hover users; the no-op onClick makes it inert. */}
                    <Button aria-disabled className="opacity-50" onClick={(e) => e.preventDefault()}>
                      Rebuild
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Connect Tunarr in Settings first</TooltipContent>
                </Tooltip>
              )}
              {!tunarrReady && (
                <p className="text-muted-foreground text-sm">
                  <Link to="/settings/connections" className="cursor-pointer text-tune hover:underline">
                    Connect Tunarr
                  </Link>{" "}
                  to push this channel live.
                </p>
              )}
            </div>
            {/* A rebuild that fails despite Tunarr being set is a real error worth showing. */}
            {reconcile.error != null && <ErrorState error={reconcile.error} />}
          </section>
        )}

        {/* Advanced — the scheduler internals, admin-only + collapsed. This is where the
            relaxation rules, the commercial-break breakdown, and the Tunarr id live, so the
            viewer surface above stays in plain language. */}
        {isAdmin && (
          <section className="rounded-lg border border-border">
            <button
              type="button"
              onClick={() => setShowAdvanced((v) => !v)}
              aria-expanded={showAdvanced}
              className="flex w-full cursor-pointer items-center gap-2 px-4 py-3 text-left text-muted-foreground text-sm transition-colors hover:bg-static-800 hover:text-foreground"
            >
              <span className="font-medium">Advanced</span>
              <span className="text-static-400 text-xs">how this channel is built</span>
              <ChevronDown
                className={`ml-auto size-4 transition-transform ${showAdvanced ? "rotate-180" : ""}`}
                aria-hidden
              />
            </button>
            {showAdvanced && <ChannelAdvanced channel={ch} channelId={id} />}
          </section>
        )}
      </div>
    </div>
  );
};

// NowNextRow — a single "Now playing / Up next" line. An empty guide is the common, honest
// case (nothing pushed yet), rendered as a plain em-dash rather than a scary blank.
const NowNextRow = ({
  label,
  entry,
  live,
}: {
  label: string;
  entry?: { title: string; gap?: boolean; stopMs?: number };
  live: boolean;
}) => {
  const value = !entry
    ? live
      ? "Nothing scheduled right now"
      : "—"
    : entry.gap
      ? "A commercial break"
      : entry.title;
  return (
    <div className="flex items-baseline justify-between gap-3">
      <span className="text-muted-foreground text-sm">{label}</span>
      <span className="min-w-0 flex-1 truncate text-right text-sm">
        {value}
        {entry && !entry.gap && entry.stopMs ? (
          <span className="ml-2 font-mono text-muted-foreground text-xs">
            til {formatEpgTime(entry.stopMs)}
          </span>
        ) : null}
      </span>
    </div>
  );
};

const Route = createFileRoute("/_authed/channels/$id")({
  component: ChannelDetailScreen,
});

export { Route };
