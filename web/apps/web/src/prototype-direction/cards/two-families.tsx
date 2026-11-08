// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Variant C — Two families. Media items (programmes, requests, clips) share MediaCard; Sources and
// People share a plain list row: a small round lead, title, one metadata line, trailing action.
import { Text } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { clips, PROGRAMMES, people, requests, type State, sources } from "./data";
import { MediaCard, RowGroup } from "./media-card";
import { CardAction, ChannelChip, initialOf, MetaLine, Panel, SourceIcon, StatusLine, tintOf } from "./parts";
import type { SurfaceProps } from "./specimen";
import { Still } from "./still";

// The plain list row. No chip, no picture slot: the lead is an icon or an initial.
const PlainRow = ({
  action,
  attention,
  lead,
  meta,
  title,
}: {
  action: string;
  attention: boolean;
  lead: ReactNode;
  meta: ReactNode[];
  title: string;
}) => (
  <div className="flex min-w-0 items-center gap-3 px-3 py-2.5">
    <div className="bg-static-800 flex size-9 shrink-0 items-center justify-center rounded-full">{lead}</div>
    <div className="flex min-w-0 flex-1 flex-col gap-0.5">
      <Text as="p" textRole="cardTitle">
        {title}
      </Text>
      <MetaLine parts={meta} />
    </div>
    <CardAction label={action} attention={attention} />
  </div>
);

const TwoFamilies = ({ state, tint, phone }: SurfaceProps & { state: State }) => (
  <>
    <Panel title="On now" note="media card · 16:9 on top">
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

    <Panel title="Request item" note="media card · 2:3 poster on the left">
      <RowGroup label="Requests">
        {requests(state).map((r) => (
          <MediaCard
            key={r.id}
            action={r.action}
            attention={state === "problems" && r.id === "r1"}
            media={<Still hue={r.hue} poster />}
            meta={[<StatusLine key="s" status={r.status} />, r.meta]}
            orientation="left"
            ratio="2/3"
            rule={tintOf(r.channel, tint)}
            title={r.title}
            {...(r.channel ? { chip: <ChannelChip chan={r.channel} tint={tint} /> } : {})}
          />
        ))}
      </RowGroup>
    </Panel>

    <Panel title="Clip card" note="media card · square on top">
      <div className={`grid gap-3 ${phone ? "grid-cols-2" : "grid-cols-5"}`}>
        {clips(state).map((c) => (
          <MediaCard
            key={c.id}
            action={c.action}
            attention={c.status !== undefined}
            corner={c.duration}
            media={<Still hue={c.hue} />}
            meta={[
              ...(c.status ? [<StatusLine key="s" status={c.status} />] : []),
              `${c.kind} · ${c.era}`,
              c.meta,
            ]}
            orientation="top"
            ratio="1/1"
            title={c.title}
          />
        ))}
      </div>
    </Panel>

    <Panel title="Filler source row" note="plain list row">
      <RowGroup label="Sources">
        {sources(state).map((s) => (
          <PlainRow
            key={s.id}
            action={s.action}
            attention={state === "problems" && s.id === "s3"}
            lead={<SourceIcon icon={s.icon} />}
            meta={[<StatusLine key="s" status={s.status} />, s.meta]}
            title={s.title}
          />
        ))}
      </RowGroup>
    </Panel>

    <Panel title="People row" note="plain list row">
      <RowGroup label="People">
        {people(state).map((u) => (
          <PlainRow
            key={u.id}
            action={u.action}
            attention={state === "problems" && u.id === "u3"}
            lead={<Text textRole="cardInitials">{initialOf(u.name)}</Text>}
            meta={[<StatusLine key="s" status={u.status} />, u.kind, u.meta]}
            title={u.name}
          />
        ))}
      </RowGroup>
    </Panel>
  </>
);

export { TwoFamilies };
