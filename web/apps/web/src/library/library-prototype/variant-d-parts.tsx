// PROTOTYPE — throwaway (#2151 step 2). Variant D ("Appliance") shared bits, built from the design
// system and @loomarr/ui rather than beside them: ArtworkFrame for posters, Action for the back
// link, Badge for the rating, ChannelIdentity for a channel, BrandMark for the colour bars.
// Tailwind is layout glue only. Motion is <=300ms and `motion-safe`.
import { Action, ArtworkFrame, Badge, BrandMark, Surface, Text } from "@loomarr/design-system";
import { ChannelIdent, ChannelIdentity } from "@loomarr/ui";
import { type ReactNode, useEffect, useState } from "react";

// One page frame for the Library and for title pages, so content starts at the same x.
const FRAME = "mx-auto flex w-full max-w-6xl flex-col gap-6 p-4 md:p-8";

// A poster: the shared ArtworkFrame on the artwork-placeholder token, held to 2:3. The real build
// puts the TMDB image inside; the stand-in sets the title in it. ArtworkFrame fixes a 16:9 ratio, so
// a 2:3 box sizes it (a frame given both width and height ignores its own ratio). Gap for the real
// build: ArtworkFrame needs a poster aspect (README).
const Poster = ({
  title,
  big = false,
  bare = false,
  dim = false,
  children,
}: {
  title: string;
  big?: boolean;
  bare?: boolean; // thumbnails: the tile alone, too small to carry type
  dim?: boolean;
  children?: ReactNode;
}) => (
  <div className="relative aspect-[2/3] w-full">
    <ArtworkFrame
      borderRadius="control"
      height="100%"
      justifyContent="flex-end"
      opacity={dim ? 0.55 : 1}
      padding={bare ? 0 : big ? "section" : "control"}
      state="ready"
      width="100%"
    >
      {bare ? (
        <Surface backgroundColor="transparent" borderWidth={0} />
      ) : (
        <Text numberOfLines={3} textRole={big ? "body" : "label"} tone="secondary">
          {title}
        </Text>
      )}
    </ArtworkFrame>
    {children}
  </div>
);

// A fixed-width poster for rows and lists.
const Thumb = ({ title, width = 40 }: { title: string; width?: number }) => (
  <div className="shrink-0" style={{ width }}>
    <Poster bare title={title} />
  </div>
);

// "← Library": the design-system ghost Action with its back glyph.
const BackLink = ({ label, onBack }: { label: string; onBack: () => void }) => (
  <div>
    <Action icon="back" onPress={onBack} variant="ghost">
      {label}
    </Action>
  </div>
);

// A rating is a neutral pill.
const RatingPill = ({ children }: { children: string }) => <Badge tone="neutral">{children}</Badge>;

// "CH 12 · Holiday" -> "On Holiday (channel 12)".
const channelPhrase = (raw: string): string => {
  const m = /^CH (\d+) · (.+)$/.exec(raw);
  return m ? `On ${m[2]} (channel ${m[1]})` : raw;
};

// A channel as the rest of the app draws it: ident tile, number and name.
const ChannelBug = ({ raw }: { raw: string }) => {
  const m = /^CH (\d+) · (.+)$/.exec(raw);
  const number = m?.[1] ?? "";
  const name = m?.[2] ?? raw;
  return (
    <ChannelIdentity
      channel={{ channelLogoState: "ready", channelName: name, channelNumber: number }}
      logo={<ChannelIdent name={name} number={Number(number)} />}
    />
  );
};

// The brand chroma bar, for idle surfaces only (empty states). Never on the grid or forms.
const ColorBars = () => <BrandMark contained={false} decorative size={6} width={120} />;

// Signature moment: static -> clear (200ms), then the card leaves. Instant under reduced motion.
const LockIn = ({
  children,
  onDone,
  hold = 1400,
}: {
  children: ReactNode;
  onDone: () => void;
  hold?: number;
}) => {
  const [clear, setClear] = useState(false);
  useEffect(() => {
    const a = window.setTimeout(() => setClear(true), 30);
    const b = window.setTimeout(onDone, hold);
    return () => {
      window.clearTimeout(a);
      window.clearTimeout(b);
    };
  }, [onDone, hold]);
  return (
    <div
      className="motion-safe:transition-[filter,opacity] motion-safe:duration-200"
      style={clear ? undefined : { filter: "grayscale(1) blur(2px)", opacity: 0.6 }}
    >
      {children}
    </div>
  );
};

// "44:02" -> "44 min"; "1:38:01" -> "1 h 38 min".
const minutesPhrase = (clock: string): string => {
  const parts = clock.split(":").map((p) => Number.parseInt(p, 10));
  const [h, m] = parts.length === 3 ? [parts[0] ?? 0, parts[1] ?? 0] : [0, parts[0] ?? 0];
  return h > 0 ? `${h} h ${m} min` : `${m} min`;
};

const Heading = ({ children }: { children: ReactNode }) => <Text textRole="headline">{children}</Text>;

export {
  BackLink,
  ChannelBug,
  ColorBars,
  channelPhrase,
  FRAME,
  Heading,
  LockIn,
  minutesPhrase,
  Poster,
  RatingPill,
  Thumb,
};
