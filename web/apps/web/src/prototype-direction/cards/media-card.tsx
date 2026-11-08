// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// The review's grammar as one component: media on the left or top at a fixed ratio, chip plus
// title, ONE metadata line, ONE action at the right edge, a hairline between rows. Variant B uses
// it for all five surfaces; variant C only where there is real media.
import { Text } from "@loomarr/design-system";
import type { ReactNode } from "react";
import { CardAction, MetaLine } from "./parts";

type Ratio = "16/9" | "2/3" | "1/1";

// The fixed media width per ratio, so every row of one kind lines up.
const LEFT_WIDTH: Record<Ratio, string> = { "16/9": "w-28", "2/3": "w-16", "1/1": "w-16" };

interface MediaCardProps {
  action: string;
  attention?: boolean;
  chip?: ReactNode;
  // Corner readout on the picture (a clip's length): mono.
  corner?: string;
  media: ReactNode;
  meta: ReactNode[];
  // The channel colour, drawn as a 2px rule when the era tint is on.
  rule?: string | undefined;
  // `top` is a bordered card in a grid; `left` is a row inside a hairline-divided list.
  orientation: "top" | "left";
  ratio: Ratio;
  title: string;
}

const Picture = ({
  corner,
  media,
  ratio,
}: {
  corner?: string | undefined;
  media: ReactNode;
  ratio: Ratio;
}) => (
  <div
    className="bg-static-800 relative w-full overflow-hidden"
    style={{ aspectRatio: ratio.replace("/", " / ") }}
  >
    {media}
    {corner ? (
      <span className="bg-static-950/80 absolute right-1.5 bottom-1.5 rounded px-1.5 py-0.5">
        <Text textRole="cardTime">{corner}</Text>
      </span>
    ) : null}
  </div>
);

const MediaCard = ({
  action,
  attention,
  chip,
  corner,
  media,
  meta,
  orientation,
  ratio,
  rule,
  title,
}: MediaCardProps) => {
  const text = (
    <>
      <div className="flex min-w-[8rem] flex-1 flex-col items-start gap-1">
        {chip}
        <Text as="h3" textRole="cardTitle">
          {title}
        </Text>
        <MetaLine parts={meta} />
      </div>
      <CardAction label={action} attention={attention ?? false} />
    </>
  );
  if (orientation === "top") {
    return (
      <article
        className="border-border bg-card flex min-w-0 flex-col overflow-hidden rounded-lg border"
        style={rule ? { borderTop: `2px solid ${rule}` } : undefined}
      >
        <Picture corner={corner} media={media} ratio={ratio} />
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2 p-3">{text}</div>
      </article>
    );
  }
  return (
    <article
      className="flex min-w-0 items-center gap-3 px-3 py-3"
      style={rule ? { boxShadow: `inset 2px 0 0 ${rule}` } : undefined}
    >
      <div className={`${LEFT_WIDTH[ratio]} shrink-0 overflow-hidden rounded-md`}>
        <Picture corner={corner} media={media} ratio={ratio} />
      </div>
      {text}
    </article>
  );
};

// The hairline-divided group of `left` rows.
const RowGroup = ({ children, label }: { children: ReactNode[]; label: string }) => (
  <ul
    aria-label={label}
    className="border-border bg-card divide-border divide-y overflow-hidden rounded-lg border"
  >
    {children.map((child, i) => (
      // biome-ignore lint/suspicious/noArrayIndexKey: static specimen
      <li key={i}>{child}</li>
    ))}
  </ul>
);

export { MediaCard, RowGroup };
