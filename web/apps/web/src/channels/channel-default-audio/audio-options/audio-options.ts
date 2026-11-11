import type { TrackDTO } from "@loomarr/api/models/trackDTO";
import { languageLabel } from "@loomarr/core/format";

// AUTO_SENTINEL is the "follow the channel/global default" choice. Radix Select forbids an
// empty-string item value, so the picker carries this named sentinel and lowers it back to "" on
// the wire — the same shape channel-seasonal/ordering use for their "" defaults.
const AUTO_SENTINEL = "auto";

// UNTAGGED is how a track with no language is saved: ISO 639's "undetermined", which playout's
// track picker reads as "the track with no language tag" (#3409). A positional "Track 2" would name
// a different track on every programme, so it can't be a per-channel setting.
const UNTAGGED = "und";
const UNTAGGED_LABEL = "Unknown language";

// audioOptions builds the Audio choices from the REAL audio tracks of what's airing (§9.1, V46) —
// never a hardcoded list. Always leads with "Auto", then one entry per distinct track language the
// airing media carries. Labels come from Intl (languageLabel), not a hand-written language table.
// A track is named by its language and never by its raw title: an encoder's handler name ("root",
// "SoundHandler") is not viewer copy (#2776). A track with no language is listed once, as
// "Unknown language".
const audioOptions = (tracks: TrackDTO[]): { value: string; label: string }[] => {
  const seen = new Set<string>();
  const opts: { value: string; label: string }[] = [{ value: AUTO_SENTINEL, label: "Auto (default)" }];
  for (const t of tracks) {
    const tag = (t.language ?? "").trim();
    const lang = !tag || tag.toLowerCase() === UNTAGGED ? UNTAGGED : tag;
    if (seen.has(lang)) continue;
    seen.add(lang);
    opts.push({ value: lang, label: lang === UNTAGGED ? UNTAGGED_LABEL : languageLabel(lang) });
  }
  return opts;
};

// withSaved keeps the currently-saved value present in an options list even when the airing does
// not carry that track. The channel may be set to French while this film is English-only: the
// preference still applies when a programme with that track airs, so it stays visible, labelled by
// its raw code.
const withSaved = (options: { value: string; label: string }[], value: string) =>
  value && !options.some((o) => o.value === value)
    ? [...options, { value, label: value === UNTAGGED ? UNTAGGED_LABEL : value.toUpperCase() }]
    : options;

export { AUTO_SENTINEL, audioOptions, withSaved };
