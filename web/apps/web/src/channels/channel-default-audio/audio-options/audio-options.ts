import type { TrackDTO } from "@loomarr/api/models/trackDTO";
import { languageLabel } from "@loomarr/core/language-label";

// AUTO_SENTINEL is the "follow the channel/global default" choice. Radix Select forbids an
// empty-string item value, so the picker carries this named sentinel and lowers it back to "" on
// the wire — the same shape channel-seasonal/ordering use for their "" defaults.
const AUTO_SENTINEL = "auto";

// audioOptions builds the Audio choices from the REAL audio tracks of what's airing (§9.1, V46) —
// never a hardcoded list. Always leads with "Auto", then one entry per distinct track language the
// airing media carries. Labels come from Intl (languageLabel), not a hand-written language table.
const audioOptions = (tracks: TrackDTO[]): { value: string; label: string }[] => {
  const seen = new Set<string>();
  const opts: { value: string; label: string }[] = [{ value: AUTO_SENTINEL, label: "Auto (default)" }];
  for (const t of tracks) {
    const lang = t.language ?? "";
    if (!lang || seen.has(lang)) continue;
    seen.add(lang);
    opts.push({ value: lang, label: t.title ? `${languageLabel(lang)} · ${t.title}` : languageLabel(lang) });
  }
  return opts;
};

// OFF_SENTINEL is "burn no subtitles in", lowered to "" on the wire like AUTO_SENTINEL.
const OFF_SENTINEL = "off";

// burnOptions builds the Burned-in subtitles choices (#2088 step 8): Off, then each language of
// the airing programme's IMAGE subtitle tracks (`burnable`). Text tracks are each viewer's own
// choice in the player, so they are not offered here.
const burnOptions = (tracks: TrackDTO[]): { value: string; label: string }[] => {
  const seen = new Set<string>();
  const opts: { value: string; label: string }[] = [{ value: OFF_SENTINEL, label: "Off" }];
  for (const t of tracks) {
    const lang = t.language ?? "";
    if (!t.burnable || !lang || seen.has(lang)) continue;
    seen.add(lang);
    opts.push({ value: lang, label: languageLabel(lang) });
  }
  return opts;
};

// withSaved keeps the currently-saved value present in an options list even when the airing does
// not carry that track. The channel may be set to French while this film is English-only: the
// preference still applies when a programme with that track airs, so it stays visible, labelled by
// its raw code.
const withSaved = (options: { value: string; label: string }[], value: string) =>
  value && !options.some((o) => o.value === value)
    ? [...options, { value, label: value.toUpperCase() }]
    : options;

export { AUTO_SENTINEL, audioOptions, burnOptions, OFF_SENTINEL, withSaved };
