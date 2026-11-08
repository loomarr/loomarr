// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Variant B — One MediaCard: all five surfaces are the same anatomy. Sources and People have no
// picture, so their media slot holds a fixed square tile (icon / initial), and the chip slot holds
// the kind (source type, role) instead of a channel.
import { Text } from "@loomarr/design-system";
import { clips, PROGRAMMES, people, requests, type State, sources } from "./data";
import { MediaCard, RowGroup } from "./media-card";
import { ChannelChip, initialOf, KindChip, Panel, SourceIcon, StatusLine, tintOf } from "./parts";
import type { SurfaceProps } from "./specimen";
import { Still } from "./still";

const OneCard = ({ state, tint, phone }: SurfaceProps & { state: State }) => (
  <>
    <Panel title="On now" note="16:9 · media on top">
      <div className={`grid gap-3 ${phone ? "grid-cols-1" : "grid-cols-3"}`}>
        {PROGRAMMES.map((p) => (
          <MediaCard
            key={p.id}
            action="Watch live"
            chip={<ChannelChip chan={p.channel} tint={tint} />}
            media={<Still hue={p.hue} />}
            meta={[
              p.episode ?? "Film",
              <Text key="t" textRole="cardTime" tone="secondary">
                {p.time}
              </Text>,
            ]}
            orientation="top"
            ratio="16/9"
            rule={tintOf(p.channel, tint)}
            title={p.title}
          />
        ))}
      </div>
    </Panel>

    <Panel title="Request item" note="2:3 poster · media on the left">
      <RowGroup label="Requests">
        {requests(state).map((r) => (
          <MediaCard
            key={r.id}
            action={r.action}
            attention={state === "problems" && r.id === "r1"}
            chip={
              r.channel ? <ChannelChip chan={r.channel} tint={tint} /> : <KindChip>No channel yet</KindChip>
            }
            media={<Still hue={r.hue} poster />}
            meta={[<StatusLine key="s" status={r.status} />, r.meta]}
            orientation="left"
            ratio="2/3"
            rule={tintOf(r.channel, tint)}
            title={r.title}
          />
        ))}
      </RowGroup>
    </Panel>

    <Panel title="Clip card" note="square · media on top">
      <div className={`grid gap-3 ${phone ? "grid-cols-2" : "grid-cols-5"}`}>
        {clips(state).map((c) => (
          <MediaCard
            key={c.id}
            action={c.action}
            attention={c.status !== undefined}
            chip={<KindChip>{`${c.kind} · ${c.era}`}</KindChip>}
            corner={c.duration}
            media={<Still hue={c.hue} />}
            meta={[...(c.status ? [<StatusLine key="s" status={c.status} />] : []), c.meta]}
            orientation="top"
            ratio="1/1"
            title={c.title}
          />
        ))}
      </div>
    </Panel>

    <Panel title="Filler source row" note="no picture: square icon tile">
      <RowGroup label="Sources">
        {sources(state).map((s) => (
          <MediaCard
            key={s.id}
            action={s.action}
            attention={state === "problems" && s.id === "s3"}
            chip={<KindChip>{s.kind}</KindChip>}
            media={
              <div className="flex size-full items-center justify-center">
                <SourceIcon icon={s.icon} />
              </div>
            }
            meta={[<StatusLine key="s" status={s.status} />, s.meta]}
            orientation="left"
            ratio="1/1"
            title={s.title}
          />
        ))}
      </RowGroup>
    </Panel>

    <Panel title="People row" note="no picture: square initial tile">
      <RowGroup label="People">
        {people(state).map((u) => (
          <MediaCard
            key={u.id}
            action={u.action}
            attention={state === "problems" && u.id === "u3"}
            chip={<KindChip>{u.kind}</KindChip>}
            media={
              <div className="flex size-full items-center justify-center">
                <Text textRole="cardTitle">{initialOf(u.name)}</Text>
              </div>
            }
            meta={[<StatusLine key="s" status={u.status} />, u.meta]}
            orientation="left"
            ratio="1/1"
            title={u.name}
          />
        ))}
      </RowGroup>
    </Panel>
  </>
);

export { OneCard };
