// PROTOTYPE — throwaway (#2151 step 2). Variant D · a title page that looks like a title page:
// big artwork, a soft glow of its own colour, the story, where it airs, and ONE collapsed
// "Technical details" section at the bottom. Plain words per PLAIN-WORDS.md.
import { Action, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";
import { FARGO, type ItemDetail, WEST_WING } from "./prototype-data";
import type { Screen } from "./prototype-state";
import {
  BackLink,
  ChannelBug,
  Collapse,
  FRAME,
  Heading,
  minutesPhrase,
  Poster,
  RatingPill,
} from "./variant-d-parts";

const field = (item: ItemDetail, label: string): string =>
  item.fields.find((f) => f.label === label)?.value ?? "";

// Plain sentences, written here rather than rendered from the techie data labels.
const COPY = {
  "west-wing": {
    origin: "Description and rating from TMDB. Episode names from Sonarr.",
    picture: "1080p HD",
    sound: "Stereo",
    length: "43 min",
  },
  fargo: {
    origin: "Description and rating from TMDB. Length measured from the file.",
    picture: "4K, extra-rich colour",
    sound: "Surround 5.1",
    length: "1 h 38 min",
  },
} as const;

const Row = ({ label, children, mono = false }: { label: string; children: string; mono?: boolean }) => (
  <div className="flex flex-col gap-1 sm:flex-row sm:gap-4">
    <div className="shrink-0 sm:w-52">
      <Text textRole="compact" tone="muted">
        {label}
      </Text>
    </div>
    <div className="min-w-0 flex-1">
      <Text textRole={mono ? "data" : "compact"}>{children}</Text>
    </div>
  </div>
);

const Seasons = ({ item }: { item: ItemDetail }) => {
  const [open, setOpen] = useState(4);
  const special = item.openSeason?.find((e) => e.special);
  return (
    <section aria-label="Seasons" className="flex flex-col gap-3">
      <Heading>Seasons</Heading>
      <div className="flex flex-col gap-2">
        {item.seasons?.map((s) => (
          <Collapse
            hint={`${s.episodes} episodes`}
            key={s.number}
            label={s.label}
            onToggle={(o) => setOpen(o ? s.number : 0)}
            open={open === s.number}
          >
            <div className="flex flex-col gap-1 p-3">
              {s.number === 4 && item.openSeason ? (
                item.openSeason
                  .filter((e) => !e.special)
                  .map((e, i) => (
                    <div className="border-border flex items-baseline gap-3 border-b py-2" key={e.code}>
                      <span className="w-6 shrink-0">
                        <Text textRole="compact" tone="muted">
                          {i + 1}
                        </Text>
                      </span>
                      <div className="min-w-0 flex-1">
                        <Text textRole="body">{e.title}</Text>
                      </div>
                      <Text textRole="compact" tone="secondary">
                        {minutesPhrase(e.runtime)}
                      </Text>
                    </div>
                  ))
                  .concat(
                    <div className="py-2" key="more">
                      <Text textRole="compact" tone="muted">
                        {s.episodes - (item.openSeason.length - 1)} more episodes
                      </Text>
                    </div>,
                  )
              ) : (
                <Text textRole="compact" tone="secondary">
                  All {s.episodes} episodes are ready to play.
                </Text>
              )}
            </div>
          </Collapse>
        ))}
        {special ? (
          <Surface gap="inline" padding="control">
            <Text textRole="label">Special — airs before season 3</Text>
            <Text textRole="compact" tone="secondary">
              {special.title} · {minutesPhrase(special.runtime)}
            </Text>
          </Surface>
        ) : null}
      </div>
    </section>
  );
};

const TitlePage = ({ screen, go }: { screen: Screen; go: (s: Screen) => void }) => {
  const item = screen === "item-movie" ? FARGO : WEST_WING;
  const copy = COPY[item.id as keyof typeof COPY];
  const series = item.kind === "Series";
  const arr = item.identity === "Sonarr" ? "Sonarr" : "Radarr";
  const [copyGone, setCopyGone] = useState(false);
  const [wrong, setWrong] = useState(false);
  const [checked, setChecked] = useState(false);
  const [tech, setTech] = useState(screen === "provenance");
  const stray = item.files?.find((f) => f.stray);
  const playing = item.files?.find((f) => f.current);
  const fact = series ? `${item.year} · ${item.seasons?.length} seasons` : `${item.year} · ${copy.length}`;

  return (
    <div className={`${FRAME} gap-8`}>
      <BackLink current={item.title} label="Library" onBack={() => go("items")} />

      <div className="relative flex flex-col gap-6 md:flex-row md:items-center md:gap-10">
        {/* A soft pool of the poster's colour that fades to nothing before any edge. */}
        <div
          aria-hidden="true"
          className="pointer-events-none absolute top-0 -left-16 h-[520px] w-[calc(100%+4rem)]"
          style={{
            backgroundImage: `radial-gradient(ellipse 420px 260px at 420px 260px, hsl(${item.hue} 60% 50% / 0.3), transparent)`,
          }}
        />
        <div className="relative w-44 shrink-0 self-center md:w-52">
          <Poster big hue={item.hue} title={item.title} />
        </div>
        <div className="relative flex min-w-0 flex-1 flex-col gap-4">
          <Text textRole="display">{item.title}</Text>
          <div className="flex flex-wrap items-center gap-3">
            <Text textRole="body" tone="secondary">
              {fact}
            </Text>
            <RatingPill>{field(item, "Rating")}</RatingPill>
          </div>
          <Text textRole="compact" tone="muted">
            {field(item, "Genres")}
          </Text>
          <Text textRole="body">{field(item, "Overview")}</Text>
          <div className="flex flex-wrap gap-2">
            <Action variant="secondary">Update details</Action>
            <Action onPress={() => setChecked(true)} variant="secondary">
              {checked ? "Checked just now" : "Check files again"}
            </Action>
            <Action onPress={() => setWrong(!wrong)} variant="ghost">
              Wrong title?
            </Action>
          </div>
          {wrong ? (
            <Surface gap="inline" padding="control">
              <Text textRole="body">
                {arr} decides what this is. Change it in {arr}.
              </Text>
              <div>
                <Action variant="secondary">Open {arr}</Action>
              </div>
            </Surface>
          ) : null}
        </div>
      </div>

      {stray && !copyGone ? (
        <Surface borderColor="stateWarning" gap="control" padding="section">
          <Text textRole="title">An extra copy is taking {stray.size}</Text>
          <Text textRole="body" tone="secondary">
            Loomarr plays the better copy, so this one never plays. Removing it gets the space back.
          </Text>
          <div>
            <Action onPress={() => setCopyGone(true)} tone="primary">
              Remove extra copy
            </Action>
          </div>
        </Surface>
      ) : null}

      <section aria-label="On these channels" className="flex flex-col gap-3">
        <Heading>On these channels</Heading>
        <div className="flex flex-wrap gap-3">
          {item.usedBy.map((u) => (
            <ChannelBug key={u.channel} raw={u.channel} />
          ))}
        </div>
      </section>

      {series ? <Seasons item={item} /> : null}

      <Collapse label="Technical details" onToggle={setTech} open={tech}>
        <div className="flex flex-col gap-3 p-4">
          <Row label="Where details come from">{copy.origin}</Row>
          <Row label="Title match">{`Recognised from ${arr}`}</Row>
          {playing ? (
            <>
              <Row label="File" mono>
                {playing.path}
              </Row>
              <Row label="Size">{playing.size}</Row>
              <Row label="Picture">{copy.picture}</Row>
              <Row label="Sound">{copy.sound}</Row>
              <Row label="Subtitles">{playing.subtitles.join(", ") || "None"}</Row>
              <Row label="Length">{copy.length}</Row>
            </>
          ) : null}
          {stray && !copyGone ? <Row label="Extra copy">{`${stray.path} (${stray.size})`}</Row> : null}
        </div>
      </Collapse>
    </div>
  );
};

export { TitlePage };
