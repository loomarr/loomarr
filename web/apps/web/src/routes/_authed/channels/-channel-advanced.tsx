import type { ChannelDTO } from "@loomarr/api";
import { humanizeRelaxation } from "@loomarr/core";
import { Badge } from "@/components/ui";
import { ChannelPods } from "@/filler";

// ChannelAdvanced — the scheduler internals, shown only inside the admin-only "Advanced"
// disclosure on the channel-detail page. This is deliberately the place for the things a
// VIEWER should never see: which programming rules Loomarr had to ease to fill the channel,
// the commercial-break breakdown, and the Tunarr id. Keeping them here is what lets the page
// above speak in plain language. Everything is explained in a sentence, not left as a code.
const ChannelAdvanced = ({ channel, channelId }: { channel: ChannelDTO; channelId: string }) => {
  const applied = channel.policy?.applied ?? [];

  return (
    <div className="flex flex-col gap-5 border-border border-t px-4 py-4">
      {/* Programming rules that were eased (the relaxation ladder). */}
      <section className="flex flex-col gap-2">
        <h3 className="font-medium text-sm">Programming rules</h3>
        {applied.length > 0 ? (
          <>
            <p className="text-muted-foreground text-sm">
              To keep this channel full, Loomarr eased a few of its own scheduling rules:
            </p>
            <div className="flex flex-wrap gap-1.5">
              {applied.map((step) => {
                const r = humanizeRelaxation(step);
                return (
                  <Badge key={`${step.kind}:${step.from}->${step.to}`} variant="caution">
                    {r.label}: {r.from} → {r.to}
                  </Badge>
                );
              })}
            </div>
          </>
        ) : (
          <p className="text-muted-foreground text-sm">
            Running exactly as specified — nothing had to be eased.
          </p>
        )}
      </section>

      {/* The commercial-break breakdown — the same assembly the channel actually plays. */}
      <section className="flex flex-col gap-2">
        <h3 className="font-medium text-sm">Commercial break</h3>
        <p className="text-muted-foreground text-sm">
          The clips that play during a break, in order, assembled exactly as the channel builds them.
        </p>
        <ChannelPods channelId={channelId} />
      </section>

      {/* The Tunarr link — pure plumbing, useful only for debugging. */}
      <section className="flex flex-col gap-1">
        <h3 className="font-medium text-sm">Broadcast link</h3>
        {channel.tunarrId ? (
          <p className="font-mono text-muted-foreground text-xs">Tunarr channel: {channel.tunarrId}</p>
        ) : (
          <p className="text-muted-foreground text-sm">
            Not on Tunarr yet — the first rebuild creates the channel there.
          </p>
        )}
      </section>
    </div>
  );
};

export { ChannelAdvanced };
