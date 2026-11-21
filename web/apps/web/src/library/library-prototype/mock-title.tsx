// PROTOTYPE — throwaway (#4476 A1). The title page as a TV app draws it: backdrop and logo, what it
// is, its story and where it airs lead; cast, episodes and facts follow. Update details, Check
// files again and Wrong title? live in one Fix menu (admin only); the file's facts stay behind one
// Technical details disclosure.
import {
  Action,
  Badge,
  Disclosure,
  Icon,
  icons,
  Menu,
  MenuAction,
  MenuContent,
  MenuTrigger,
  Text,
} from "@loomarr/design-system";
import { ChannelIdent, ChannelIdentity } from "@loomarr/ui";
import { type CSSProperties, useState } from "react";
import { FileName, Proposal, UnderlineTabs } from "./mock-parts";
import { type Episode, HOME, type MockFile, type MockTitlePage, MOVIE, type Person, SERIES } from "./mock-title-data";

type TitleKind = "movie" | "series" | "home";
type TitleState = "default" | "fix-open" | "tech-open" | "extra" | "extra-undo" | "airing" | "unplayable";

const HUES = ["var(--color-tune)", "var(--color-lock)", "var(--color-onair)", "var(--color-suggest)", "var(--color-caution)"];
const hue = (i: number) => HUES[i % HUES.length] ?? "var(--color-static-400)";
const mix = (c: string, pct: number, base = "var(--color-static-950)") => `color-mix(in srgb, ${c} ${pct}%, ${base})`;

// Stand-in backdrop: a wide film still (sky, a lit horizon, dark ground), never a flat block.
const backdrop = (t: MockTitlePage): CSSProperties => ({
  background:
    t.kind === "home"
      ? `radial-gradient(60% 50% at 35% 45%, ${mix(hue(t.hue2), 55, "transparent")}, transparent 70%), linear-gradient(180deg, ${mix(hue(t.hue), 45)}, ${mix(hue(t.hue2), 25)})`
      : `radial-gradient(18% 22% at 68% 40%, ${mix("var(--color-static-100)", 80, "transparent")}, transparent 70%), radial-gradient(90% 30% at 60% 58%, ${mix(hue(t.hue), 70, "transparent")}, transparent 70%), linear-gradient(180deg, ${mix(hue(t.hue2), 55)} 0%, ${mix(hue(t.hue), 30)} 55%, var(--color-static-950) 62%, ${mix(hue(t.hue2), 15)} 100%)`,
});

const Still = ({ e, dim }: { e: Episode; dim?: boolean }) => (
  <div
    aria-hidden
    className={`aspect-video w-28 shrink-0 rounded-md sm:w-40 ${dim ? "opacity-40" : ""}`}
    style={{
      background: `radial-gradient(40% 50% at ${30 + e.n * 9}% 45%, ${mix(hue(e.hue + 1), 70, "transparent")}, transparent 70%), linear-gradient(160deg, ${mix(hue(e.hue), 50)}, var(--color-static-950))`,
    }}
  />
);

// A person: their photo once it is imported, their initials until then.
const Face = ({ p }: { p: Person }) => (
  <li className="flex w-24 shrink-0 flex-col items-center gap-2 text-center">
    {p.photo === undefined ? (
      <div className="bg-static-800 text-static-100 flex size-16 items-center justify-center rounded-full text-lg font-semibold">
        {p.name
          .split(" ")
          .map((w) => w[0])
          .slice(0, 2)
          .join("")}
      </div>
    ) : (
      <div
        aria-hidden
        className="size-16 rounded-full"
        style={{
          background: `radial-gradient(35% 35% at 50% 38%, ${mix("var(--color-static-100)", 70, "transparent")}, transparent 72%), linear-gradient(180deg, ${mix(hue(p.photo), 60)}, ${mix(hue(p.photo + 2), 35)})`,
        }}
      />
    )}
    <div>
      <Text as="p" numberOfLines={2} strong textRole="compact">
        {p.name}
      </Text>
      <Text as="p" numberOfLines={2} textRole="compact" tone="secondary">
        {p.role}
      </Text>
    </div>
  </li>
);

const FixMenu = ({ open }: { open: boolean }) => (
  <Menu defaultOpen={open}>
    <MenuTrigger>
      <Action icon="settingsAdjust" variant="outline">
        Fix
      </Action>
    </MenuTrigger>
    <MenuContent align="end" width={260}>
      <MenuAction icon={<Icon decorative glyph={icons.refresh} size="compact" tone="muted" />} onSelect={() => undefined}>
        Update details
      </MenuAction>
      <MenuAction icon={<Icon decorative glyph={icons.folder} size="compact" tone="muted" />} onSelect={() => undefined}>
        Check files again
      </MenuAction>
      <MenuAction icon={<Icon decorative glyph={icons.swap} size="compact" tone="muted" />} onSelect={() => undefined}>
        Wrong title?
      </MenuAction>
    </MenuContent>
  </Menu>
);

const Row = ({ k, children }: { k: string; children: React.ReactNode }) => (
  <div className="grid grid-cols-[7.5rem_minmax(0,1fr)] gap-3 py-1.5 sm:grid-cols-[9rem_minmax(0,1fr)]">
    <Text as="dt" textRole="compact" tone="secondary">
      {k}
    </Text>
    <dd className="min-w-0">{typeof children === "string" ? <Text textRole="compact">{children}</Text> : children}</dd>
  </div>
);

// Technical details: the plain summary first; "More about this file" opens the rest in place
// (no second disclosure inside the first, so no card in a card).
const TechnicalDetails = ({ f, open }: { f: MockFile; open: boolean }) => {
  const [more, setMore] = useState(open);
  return (
    <Disclosure defaultExpanded={open} description={`${f.picture} · ${f.sound} · ${f.subtitles === "None" ? "No subtitles" : `${f.subtitles} subtitles`}`} label="Technical details">
      <dl className="flex flex-col">
        <Row k="File">
          <FileName name={f.name} strong={false} />
        </Row>
        <Row k="Where">{f.folder}</Row>
        <Row k="Size">{f.size}</Row>
        <Row k="Picture">{f.picture}</Row>
        <Row k="Sound">{f.sound}</Row>
        <Row k="Subtitles">{f.subtitles}</Row>
        <Row k="Length">{f.length}</Row>
      </dl>
      <div className="pt-2">
        <Action icon={more ? "showLess" : "showMore"} onPress={() => setMore(!more)} variant="ghost">
          {more ? "Less about this file" : "More about this file"}
        </Action>
      </div>
      {more ? (
        <dl className="border-static-700 mt-2 flex flex-col border-t pt-2">
          <Row k="Container">{f.more.container}</Row>
          <Row k="Video">{f.more.video}</Row>
          <Row k="Bitrate">{f.more.bitrate}</Row>
          <Row k="Frame rate">{f.more.frameRate}</Row>
          <Row k="HDR">{f.more.hdr}</Row>
          <Row k="Sound tracks">
            <ul>
              {f.more.audio.map((t) => (
                <li key={t.detail}>
                  <Text textRole="compact">{t.language}</Text>{" "}
                  <Text textRole="compact" tone="secondary">
                    {t.detail}
                  </Text>
                </li>
              ))}
            </ul>
          </Row>
          <Row k="Subtitles">
            <ul>
              {f.more.subs.map((t) => (
                <li key={t.detail}>
                  <Text textRole="compact">{t.language}</Text>{" "}
                  <Text textRole="compact" tone="secondary">
                    {t.detail}
                  </Text>
                </li>
              ))}
            </ul>
          </Row>
          <Row k="Chapters">
            <ul>
              {f.more.chapters.map((c) => (
                <li key={c}>
                  <Text textRole="compact">{c}</Text>
                </li>
              ))}
            </ul>
          </Row>
        </dl>
      ) : null}
    </Disclosure>
  );
};

// An extra copy: names the file, says where it goes and for how long; the remove is not amber.
const ExtraCopy = ({ t }: { t: MockTitlePage }) =>
  t.extra ? (
    <section className="border-caution/40 bg-caution/5 flex flex-col gap-3 rounded-lg border p-4">
      <Text as="h2" strong textRole="body">
        There's an extra copy of this film
      </Text>
      <Text as="p" textRole="compact" tone="secondary">
        Loomarr plays the {t.file.picture} copy. This one never plays:
      </Text>
      <div className="flex flex-col gap-0.5">
        <FileName name={t.extra.name} />
        <Text as="p" textRole="compact" tone="secondary">
          {t.extra.size} · {t.extra.picture} · in {t.extra.folder}
        </Text>
      </div>
      <Text as="p" textRole="compact" tone="secondary">
        Removing it moves it to Loomarr's trash in the same folder. It's deleted for good after 30 days, and
        until then you can put it back.
      </Text>
      <div>
        <Action variant="outline">Remove extra copy</Action>
      </div>
    </section>
  ) : null;

const Unplayable = ({ t }: { t: MockTitlePage }) => (
  <section className="border-caution/40 bg-caution/5 flex flex-col gap-3 rounded-lg border p-4">
    <Text as="h2" strong textRole="body">
      2 episodes in Season 2 can't be played
    </Text>
    <Text as="p" textRole="compact" tone="secondary">
      Loomarr can see these files but can't read the picture. They may still be copying, or be damaged.
      If they're still copying, check again when it's done; if they're damaged, replace them.
    </Text>
    <ul className="flex flex-col gap-1">
      {["Copperline S02E02.mkv", "Copperline S02E03.mkv"].map((n) => (
        <li key={n}>
          <FileName name={n} />
        </li>
      ))}
    </ul>
    <Text as="p" textRole="compact" tone="secondary">
      In {t.file.folder}
    </Text>
    <div>
      <Action icon="refresh" variant="outline">
        Check files again
      </Action>
    </div>
  </section>
);

const Episodes = ({ t, phone, initial }: { t: MockTitlePage; phone: boolean; initial?: string }) => {
  const seasons = t.seasons ?? [];
  const [season, setSeason] = useState(initial ?? seasons[0]?.label ?? "");
  const current = seasons.find((s) => s.label === season);
  return (
    <section className="flex flex-col gap-3">
      <UnderlineTabs
        label="Seasons"
        onChange={setSeason}
        options={seasons.map((s) => ({ value: s.label, label: s.label, count: String(s.episodes.length) }))}
        value={season}
      />
      {current?.note ? (
        <Text as="p" textRole="compact" tone="secondary">
          {current.note}
        </Text>
      ) : null}
      <ol className="flex flex-col">
        {current?.episodes.map((e) => (
          <li className="flex gap-3 py-2.5 sm:gap-4" key={e.n}>
            <Still dim={e.ready === false || e.unplayable} e={e} />
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <div className="flex items-baseline justify-between gap-3">
                <Text as="p" numberOfLines={1} strong textRole="compact">
                  {e.n}. {e.name}
                </Text>
                {phone ? null : (
                  <Text textRole="compact" tone="secondary">
                    {e.length}
                  </Text>
                )}
              </div>
              <Text as="p" numberOfLines={phone ? 2 : 1} textRole="compact" tone="secondary">
                {e.summary}
              </Text>
              {e.ready === false ? (
                <Text as="p" textRole="compact" tone="info">
                  Getting ready
                </Text>
              ) : e.unplayable ? (
                <Text as="p" textRole="compact" tone="warning">
                  Can't be played
                </Text>
              ) : phone ? (
                <Text as="p" textRole="compact" tone="secondary">
                  {e.length}
                </Text>
              ) : null}
            </div>
          </li>
        ))}
      </ol>
    </section>
  );
};

const TitlePage = ({
  kind,
  state,
  admin,
  phone,
  onBack,
}: {
  kind: TitleKind;
  state: TitleState;
  admin: boolean;
  phone: boolean;
  onBack: () => void;
}) => {
  const t = kind === "series" ? SERIES : kind === "home" ? HOME : MOVIE;
  const home = t.kind === "home";

  const top = (
    <div className="flex items-center justify-between gap-2">
      <Action icon="back" onPress={onBack} variant="ghost">
        Library
      </Action>
      {admin ? <FixMenu open={state === "fix-open"} /> : null}
    </div>
  );

  const identity = (
    <div className="flex flex-col gap-2">
      {home ? (
        <Text as="h1" textRole="title">
          {t.name}
        </Text>
      ) : (
        // Stand-in for the title's logo art; the real page draws the imported logo image.
        <h1 className="text-static-0 text-[32px] leading-[0.95] font-black tracking-tight uppercase sm:text-[52px]">
          {t.name}
        </h1>
      )}
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <Text textRole="compact">{t.year}</Text>
        <Text textRole="compact" tone="secondary">
          ·
        </Text>
        <Text textRole="compact">{t.length}</Text>
        {t.rating ? <Badge tone="neutral">{t.rating}</Badge> : null}
        <Text textRole="compact" tone="secondary">
          · {t.genres.join(", ")}
        </Text>
        {t.score ? (
          <Text textRole="compact" tone="secondary">
            · Score <Text strong textRole="compact">{t.score}</Text>/10
          </Text>
        ) : null}
      </div>
      {t.tagline ? (
        <p className="text-static-100 text-base italic">{t.tagline}</p>
      ) : null}
    </div>
  );

  const story = (
    <div className="flex flex-col gap-4">
      <p className="text-static-100 max-w-[62ch] text-base leading-relaxed">
        {t.overview ??
          (admin
            ? "A home video you kept as it is. Loomarr won't look it up, and it keeps the file's own name."
            : "A home video from your household's library.")}
      </p>
      <div className="flex flex-col gap-2">
        <Text as="h2" strong textRole="compact" tone="secondary">
          {t.channels.length ? "On these channels" : "Not on a channel yet"}
        </Text>
        {t.channels.length ? (
          <div className="flex flex-wrap gap-x-6 gap-y-2">
            {t.channels.map((c) => (
              <ChannelIdentity
                channel={{ channelLogoState: "ready", channelName: c.name, channelNumber: String(c.number) }}
                key={c.name}
                logo={<ChannelIdent name={c.name} number={c.number} />}
              />
            ))}
          </div>
        ) : (
          <Text as="p" textRole="compact" tone="secondary">
            A channel can play it once it's in one of the channel's collections.
          </Text>
        )}
        {state === "airing" ? (
          <Proposal className="mt-2" question="Show when it next airs, beside the channels">
            <Text as="p" textRole="compact">
              Next on Late Show, tonight at 9:00 · then Mystery Hour, Saturday at 11:30
            </Text>
          </Proposal>
        ) : null}
      </div>
    </div>
  );

  const facts =
    t.facts.length || t.crew.length ? (
      <dl className="flex flex-col">
        {t.crew.map(([k, v]) => (
          <Row k={k} key={k}>
            {v}
          </Row>
        ))}
        {t.facts.map(([k, v]) => (
          <Row k={k} key={k}>
            {v}
          </Row>
        ))}
      </dl>
    ) : null;

  const memberSummary = (
    <Proposal question="Members see a one-line technical summary (recommended), or nothing">
      <Text as="p" textRole="compact">
        {t.file.picture} · {t.file.sound.split(",")[0]} · {t.file.subtitles === "None" ? "No subtitles" : `${t.file.subtitles} subtitles`}
      </Text>
    </Proposal>
  );

  return (
    <div className="flex flex-col pb-28">
      {phone ? (
        <>
          <div className="relative aspect-video w-full" style={backdrop(t)}>
            <div className="absolute inset-x-0 top-0 bg-gradient-to-b from-static-950/70 to-transparent px-3 pt-2 pb-6">{top}</div>
          </div>
          <div className="flex flex-col gap-5 px-6 pt-4">
            {identity}
            {story}
          </div>
        </>
      ) : (
        <div className="relative min-h-[400px] w-full" style={backdrop(t)}>
          <div className="from-static-950 absolute inset-0 bg-gradient-to-t via-transparent" />
          <div className="from-static-950/90 absolute inset-0 bg-gradient-to-r via-static-950/30 to-transparent" />
          <div className="relative flex min-h-[400px] flex-col justify-between px-8 pt-4 pb-6">
            {top}
            <div className="max-w-2xl">{identity}</div>
          </div>
        </div>
      )}
      <div className="flex flex-col gap-8 px-6 pt-2 sm:px-8">
        {phone ? null : (
          <div className="grid grid-cols-[minmax(0,1fr)_20rem] gap-10">
            {story}
            {facts}
          </div>
        )}
        {admin && (state === "extra" || state === "extra-undo") ? <ExtraCopy t={t} /> : null}
        {admin && state === "unplayable" ? <Unplayable t={t} /> : null}
        {t.seasons ? <Episodes initial={state === "unplayable" ? "Season 2" : undefined} phone={phone} t={t} /> : null}
        {t.cast.length ? (
          <section className="flex flex-col gap-3">
            <Text as="h2" textRole="heading">
              Cast
            </Text>
            <ul className="-mx-6 flex gap-4 overflow-x-auto px-6 sm:mx-0 sm:flex-wrap sm:px-0">
              {t.cast.map((p) => (
                <Face key={p.name} p={p} />
              ))}
            </ul>
          </section>
        ) : null}
        {phone && facts ? <section>{facts}</section> : null}
        {admin ? <TechnicalDetails f={t.file} open={state === "tech-open"} /> : memberSummary}
      </div>
      {state === "extra-undo" && t.extra ? (
        <div
          className="bg-static-800 fixed inset-x-4 bottom-24 z-20 mx-auto flex max-w-md items-center gap-3 rounded-lg p-3 shadow-lg sm:bottom-6"
          role="status"
        >
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <FileName name={t.extra.name} strong={false} />
            <Text as="p" textRole="compact" tone="secondary">
              Moved to the trash. Deleted for good on 21 December.
            </Text>
          </div>
          <Action variant="outline">Undo</Action>
        </div>
      ) : null}
    </div>
  );
};

export type { TitleKind, TitleState };
export { TitlePage };
