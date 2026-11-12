import * as channelsApi from "@loomarr/api/endpoints/channels";
import { toProblem } from "@loomarr/api/mutator";
import { channelNumber, pluralize } from "@loomarr/core/format";
import { Action, Disclosure } from "@loomarr/design-system";
import { createFileRoute } from "@tanstack/react-router";
import { toast } from "sonner";
import { ChannelDefaultAudio } from "@/channels/channel-default-audio";
import { ChannelOnScreenLogo } from "@/channels/channel-on-screen-logo";
import { ChannelBreaks } from "@/components/loomarr/channels/channel-breaks";
import { ChannelHero } from "@/components/loomarr/channels/channel-hero";
import { ChannelIconField } from "@/components/loomarr/channels/channel-icon-field";
import { ChannelUpcoming } from "@/components/loomarr/channels/channel-upcoming";
import { OnAirIndicator } from "@/components/loomarr/channels/on-air-indicator";
import { ChannelAdvanced } from "./-channel-advanced";
import { useChannelDetail } from "./-channel-detail-context";

// CHANNEL INFO — the "is it working" at a glance. The default section, and the viewer's ONLY one
// (a member never sees the tab bar at all, so this is the whole page for them — see index.tsx,
// which redirects bare /channels/$id here). An admin also sets the channel's on-air presentation
// here: the On-screen logo and the default audio language.
const InfoScreen = () => {
  const { id, channel: ch, isAdmin, air, ready, total, savePolicy, saveLogo } = useChannelDetail();
  const navigate = Route.useNavigate();
  // Build schedule (#2728) asks the server to converge the channel now. The `channel` SSE frame
  // refreshes the page when it lands; a failure says so.
  const reconcile = channelsApi.useReconcileChannel({
    mutation: { onError: (e) => toast.error(toProblem(e).title ?? "Couldn't build the schedule") },
  });

  return (
    <>
      {/* The page opens on a picture (#2753): the title backdrop, else generated channel art. */}
      <ChannelHero channelId={id} channel={{ name: ch.name, number: ch.number }} />
      <section className="border-border bg-card flex flex-col gap-3 rounded-lg border p-5">
        <div className="flex items-center gap-3">
          <span className="font-mono text-3xl leading-none tabular-nums">{channelNumber(ch.number)}</span>
          <div className="flex items-center gap-2">
            <OnAirIndicator state={air.dot} />
            {/* The status is the info panel's heading (also what the reachability check
                keys on — a real heading proves the screen composed). */}
            <h2 className="text-lg font-semibold">{air.label}</h2>
          </div>
        </div>
        <p className="text-muted-foreground text-sm">{air.detail}</p>
        {air.buildSchedule && isAdmin ? (
          <Action
            style={{ alignSelf: "flex-start" }}
            disabled={reconcile.isPending}
            onPress={() => reconcile.mutate({ id })}
            variant="outline"
          >
            {reconcile.isPending ? "Building schedule…" : "Build schedule"}
          </Action>
        ) : null}

        {/* The viewer's "what's on later" — the program airing now, then the next few,
            with real airtimes from the selected backend (the prototype's "Today's guide", P7). Shown to
            everyone; refreshes on the `channel` SSE frame via the layout's invalidate. */}
        <div className="border-border border-t pt-3">
          <ChannelUpcoming channelId={id} live={air.dot === "live"} />
        </div>

        {/* What plays BETWEEN those shows. Read-only and shown to everyone, which is the whole
            point: `GET /v1/channels/{id}/pods` is member-readable where the Filler tab's draft
            sandbox is admin-only, so this is the only route by which a viewer can see why a
            channel sounds the way it does. It renders nothing when the channel has no break pool
            (or the install has no filler), so a channel without commercials is simply quiet here
            rather than showing an empty panel. */}
        <div className="border-border border-t pt-3 empty:hidden">
          <ChannelBreaks channelId={id} />
        </div>

        <p className="border-border border-t pt-3 text-sm">
          <span className="font-medium">{`${ready} of ${pluralize(total, "title")} ready`}</span>
          {ready < total && (
            <span className="text-muted-foreground">
              {" "}
              , the rest fill in as titles arrive, so the channel never goes dark.
            </span>
          )}
        </p>

        {/* The channel icon is what the family actually sees in the media server's guide,
            so it belongs on the info panel next to the number and status rather than behind
            an editing tab. The component gates its own affordances on `isAdmin` — a viewer
            sees the current artwork, an admin can change it. */}
        <div className="border-border border-t pt-3">
          <ChannelIconField
            channelId={id}
            logo={ch.logo}
            logoImage={ch.logoImage}
            onSetLogo={saveLogo}
            isAdmin={isAdmin}
          />
        </div>
      </section>

      {/* The On-screen logo (#1659): admin-only like Programming and Filler, autosaving
          policy.watermark like the Programming fields. Members never see it. */}
      {isAdmin && (
        <ChannelOnScreenLogo
          channel={ch}
          onOpenProgramming={() => void navigate({ to: "/channels/$id/programming", params: { id } })}
          savePolicy={savePolicy}
        />
      )}

      {/* The default audio language (#2088), admin-only: it moved here from the player when Watch
          became its own route. */}
      {isAdmin && <ChannelDefaultAudio channel={ch} savePolicy={savePolicy} />}

      {/* Admin-only diagnostics disclosure (P7): the relaxation ladder the last reconcile
          applied + the Tunarr channel id are STATUS, not settings — so they live here as a
          collapsed disclosure rather than a top-level "Advanced" tab. A viewer never sees it. */}
      {isAdmin && (
        <Disclosure
          description="Technical details about how Loomarr builds this channel."
          label="Diagnostics"
        >
          <ChannelAdvanced channel={ch} onPolicyChange={savePolicy} />
        </Disclosure>
      )}
    </>
  );
};

const Route = createFileRoute("/_authed/channels/$id/info")({
  component: InfoScreen,
  // The panel's "What's on" strip and its break preview each read their own endpoint and answer
  // for themselves when empty or failed, so they are warmed together (a prefetch never rejects)
  // rather than left to pop in under the status line. The layout's loader has the channel.
  loader: async ({ context: { queryClient }, params: { id } }) => {
    await Promise.all([
      queryClient.prefetchQuery(
        channelsApi.getChannelUpcomingQueryOptions(id, undefined, { query: { retry: false } }),
      ),
      queryClient.prefetchQuery(
        channelsApi.getPreviewChannelPodsQueryOptions(id, { query: { retry: false } }),
      ),
    ]);
  },
});

export { Route };
