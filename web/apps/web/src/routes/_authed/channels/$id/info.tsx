import { channelNumber, pluralize } from "@loomarr/core";
import { createFileRoute } from "@tanstack/react-router";
import { ChannelIconField, ChannelUpcoming, CollapsibleSection, OnAirIndicator } from "@/components/loomarr";
import { ChannelAdvanced } from "./-channel-advanced";
import { useChannelDetail } from "./-channel-detail-context";

// CHANNEL INFO — the read-only "is it working" at a glance. The default section, and the
// viewer's ONLY one (a member never sees the tab bar at all, so this is the whole page for
// them — see route.tsx, which redirects bare /channels/$id here).
const InfoScreen = () => {
  const { id, channel: ch, isAdmin, air, ready, total, savePolicy, saveLogo } = useChannelDetail();

  return (
    <>
      <section className="flex flex-col gap-3 rounded-lg border border-border bg-card p-5">
        <div className="flex items-center gap-3">
          <span className="font-mono text-3xl tabular-nums leading-none">{channelNumber(ch.number)}</span>
          <div className="flex items-center gap-2">
            <OnAirIndicator state={air.dot} />
            {/* The status is the info panel's heading (also what the reachability check
                keys on — a real heading proves the screen composed). */}
            <h2 className="font-semibold text-lg">{air.label}</h2>
          </div>
        </div>
        <p className="text-muted-foreground text-sm">{air.detail}</p>

        {/* The viewer's "what's on later" — the program airing now, then the next few,
            with real Tunarr airtimes (the prototype's "Today's guide", P7). Shown to
            everyone; refreshes on the `channel` SSE frame via the layout's invalidate. */}
        <div className="border-border border-t pt-3">
          <ChannelUpcoming channelId={id} live={air.dot === "live"} />
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
          <ChannelIconField channelId={id} logo={ch.logo} onSetLogo={saveLogo} isAdmin={isAdmin} />
        </div>
      </section>

      {/* Admin-only diagnostics disclosure (P7): the relaxation ladder the last reconcile
          applied + the Tunarr channel id are STATUS, not settings — so they live here as a
          collapsed disclosure rather than a top-level "Advanced" tab. A viewer never sees it. */}
      {isAdmin && (
        <CollapsibleSection
          title="Diagnostics"
          description="How this channel is being built right now, relaxations applied and its Tunarr link."
        >
          <ChannelAdvanced channel={ch} onPolicyChange={savePolicy} />
        </CollapsibleSection>
      )}
    </>
  );
};

const Route = createFileRoute("/_authed/channels/$id/info")({
  component: InfoScreen,
});

export { Route };
