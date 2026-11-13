import type { SubtitleTrackDTO } from "@loomarr/api/models/subtitleTrackDTO";
import { languageLabel } from "../../format";

// SUBTITLES_OFF is the Subtitles menu's first row and a viewer's remembered key when they want none.
// A track key is a language code, so "off" can never be one.
const SUBTITLES_OFF = "off";

const UNTAGGED = "und";

interface SubtitleChoice {
  /** The quieter second line under a hearing-impaired track. */
  detail?: string;
  label: string;
  selected: boolean;
  /** The track key the server takes (`en`, `en-sdh`), or SUBTITLES_OFF. */
  value: string;
}

// A key's language, the part before its -sdh / -forced: the server plays that language's first
// unforced track when a programme has no track by the exact key.
const keyLanguage = (key: string) => key.split("-")[0] ?? key;

const languageOf = (key: string) => {
  const lang = keyLanguage(key);
  return lang === UNTAGGED ? "Unknown language" : languageLabel(lang);
};

/**
 * The Subtitles menu on every client (§9.5): Off, then each language the programme on air has, a
 * hearing-impaired track as "English, with sounds". A track is named by its language, never by its
 * own title. A forced track (only the lines in another language) is listed only for a language with
 * no full track, so a language reads once.
 *
 * The row marked is what plays: the remembered key, else (as the server picks) its language's full
 * track. A remembered language this programme does not carry stays listed and marked, as Audio
 * keeps a saved language: it applies again when a programme with it airs.
 */
const subtitleChoices = (tracks: readonly SubtitleTrackDTO[], chosen: string): SubtitleChoice[] => {
  const rows: SubtitleChoice[] = [];
  const add = (track: SubtitleTrackDTO) => {
    if (rows.some((row) => row.value === track.key)) return;
    const language = languageOf(track.key);
    rows.push(
      track.hearingImpaired
        ? {
            detail: "For the deaf and hard of hearing",
            label: `${language}, with sounds`,
            selected: false,
            value: track.key,
          }
        : { label: language, selected: false, value: track.key },
    );
  };
  for (const track of tracks) if (!track.forced) add(track);
  for (const track of tracks) {
    const lang = keyLanguage(track.key);
    if (track.forced && !rows.some((row) => keyLanguage(row.value) === lang)) add(track);
  }

  const off = !chosen || chosen === SUBTITLES_OFF;
  const fallback = tracks.find((t) => !t.forced && keyLanguage(t.key) === keyLanguage(chosen));
  const playing = off ? SUBTITLES_OFF : rows.some((row) => row.value === chosen) ? chosen : fallback?.key;
  if (playing === undefined) rows.push({ label: languageOf(chosen), selected: false, value: chosen });
  const marked = playing ?? chosen;
  return [{ label: "Off", selected: false, value: SUBTITLES_OFF }, ...rows].map((row) =>
    row.value === marked ? { ...row, selected: true } : row,
  );
};

/** The choice after the marked one, round to Off: what `C` steps to. */
const nextSubtitle = (choices: readonly SubtitleChoice[]): string => {
  const at = choices.findIndex((choice) => choice.selected);
  return choices[(at + 1) % choices.length]?.value ?? SUBTITLES_OFF;
};

export type { SubtitleChoice };
export { nextSubtitle, SUBTITLES_OFF, subtitleChoices };
