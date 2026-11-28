import type { ReactElement, ReactNode } from "react";
import type { LibraryExternalId, LibraryNeedSearch } from "../library-needs/library-needs.type";
import type { LibraryKind } from "../library-shelf/library-shelf.type";

/** One of a movie's files, with what its header says once read. */
interface LibraryTitleFile {
  path: string;
  /** Bytes. */
  size: number;
  /** How long it plays, measured from the file. */
  lengthMillis?: number;
  /** Its first video stream. */
  picture?: { width: number; height: number; hdr: boolean };
  /** The channels of the sound that plays. */
  audioChannels?: number;
  /** Each subtitle stream; a language is an ISO 639 code, absent when the file doesn't say. */
  subtitles: readonly { language?: string }[];
  /**
   * What the file is to the title: the copy that plays, an extra copy that never plays because
   * another of its files plays in its place, one that can't be played, one being read, or a copy
   * waiting while the copy in its place is missing or can't be played.
   */
  state: "plays" | "extra" | "unplayable" | "reading" | "waiting";
  /** Which part of a movie split over several files it is, from 1; absent for a whole file. */
  part?: number;
  /** The format its header names, such as matroska,webm. */
  container?: string;
  /** Its overall bit rate in bit/s. */
  bitrate?: number;
  /** Every track, as the header lists them; empty while unread. */
  streams?: readonly LibraryTitleStream[];
  chapters?: readonly { name?: string; startMillis: number }[];
}

/** One track of a file. */
interface LibraryTitleStream {
  index: number;
  kind: "video" | "audio" | "subtitle";
  codec?: string;
  profile?: string;
  bitDepth?: number;
  bitrate?: number;
  /** As the file states it, such as 24000/1001. */
  frameRate?: string;
  /** A video track's high dynamic range; absent for standard range. */
  dynamicRange?: string;
  audioFormat?: string;
  channels?: number;
  channelLayout?: string;
  language?: string;
  languageName?: string;
  title?: string;
  default: boolean;
  forced: boolean;
  commentary: boolean;
  descriptive: boolean;
  hearingImpaired: boolean;
  external: boolean;
}

/** Where a title's details came from, each as a name to show ("TMDB", "Sonarr"). */
interface LibraryTitleDetailsFrom {
  description?: string;
  rating?: string;
  /** Where most of a series' episode names came from. */
  episodeNames?: string;
}

interface LibraryTitleEpisode {
  id: string;
  name: string;
  number: number;
  /** The last episode a multi-episode file holds. */
  end?: number;
  ready: boolean;
  /** Named and numbered by its file until the show's episode order is said (#2151 decision 53). */
  fromFile?: boolean;
  /** Its own story, shown in one or two lines. */
  overview?: string;
  lengthMillis?: number;
  /** Its still, drawn by the app; absent when it has none. */
  still?: ReactNode;
}

interface LibraryTitleSeason {
  /** 0 is the specials. */
  number: number;
  episodes: readonly LibraryTitleEpisode[];
}

/** When a channel next shows the title, its time already worded in the household's clock. */
interface LibraryTitleAiring {
  channel: LibraryTitleChannel;
  /** "Next tonight at 9:00 PM", or "On now". */
  when: string;
  /** Past the window on air, so it can still change. */
  forecast: boolean;
  /** A series' episode, worded. */
  episode?: string;
}

interface LibraryTitlePerson {
  name: string;
  /** Who they are in the library, for the link to their titles; absent or empty draws plain text. */
  personId?: string;
  character?: string;
  role: "actor" | "director" | "writer" | "creator";
  /** An actor's photo, drawn by the app; absent shows their initials. */
  photo?: ReactNode;
}

/** What plays, for a member's one technical line. */
interface LibraryTitlePlaying {
  picture?: { width: number; height: number; hdr: boolean };
  audioChannels?: number;
  /** Language names. */
  subtitleLanguages: readonly string[];
}

/** A movie or a series, as its page shows it. */
interface LibraryTitleDetail {
  kind: LibraryKind;
  /** Kept as the household's own video: no catalogue to read, so no Update details. */
  homeVideo?: boolean;
  /** The wide art behind the title, drawn by the app. */
  backdrop?: ReactNode;
  /** The title's logo, drawn by the app in place of its name. */
  logo?: ReactNode;
  tagline?: string;
  /** Out of 10. */
  score?: number;
  credits?: readonly LibraryTitlePerson[];
  /** One per channel, earliest first. */
  airings?: readonly LibraryTitleAiring[];
  playing?: LibraryTitlePlaying;
  name: string;
  year?: number;
  /** Labelled facts beside the crew, such as Studio and Country. */
  about?: readonly { label: string; value: string }[];
  overview?: string;
  genres: readonly string[];
  /** The certification, such as TV-14. */
  rating?: string;
  runtimeMillis?: number;
  /** A movie's files; empty for a series. */
  files: readonly LibraryTitleFile[];
  /** A series' episodes by season; empty for a movie. */
  seasons: readonly LibraryTitleSeason[];
  /** Its files that can't be played: their details couldn't be read. */
  unplayable: number;
  match: LibraryTitleMatch;
  detailsFrom: LibraryTitleDetailsFrom;
  /** The channels whose lineup holds it, by number. */
  channels: readonly LibraryTitleChannel[];
}

/** A channel a title is on. */
interface LibraryTitleChannel {
  id: string;
  name: string;
  number: number;
}

/**
 * What decides which title this is, for "Wrong title?" (decision 26): the app that manages it, the
 * file itself (an id in its name, or an NFO), the household's answer, a TMDB search that agreed on
 * its name, year and length (decision 56), or nothing yet.
 */
interface LibraryTitleMatch {
  by: "app" | "file" | "you" | "search" | "none";
  /** The app's name, such as Sonarr, when by is app. */
  app?: string;
}

/** The app's own link element to a person's titles; the name is drawn onto it. */
type LibraryPersonLink = (personId: string) => ReactElement<Record<string, unknown>>;

interface LibraryTitleProps {
  title: LibraryTitleDetail;
  onBack: () => void;
  /** Makes each credit with a personId a link; absent leaves every name as text. */
  personLink?: LibraryPersonLink;
  /**
   * What an admin does about the title. Absent for a member, who reads the page only: no Update
   * details, Check files again or Wrong title?, no file cards and no Technical details, only the
   * one technical line.
   */
  fix?: LibraryTitleFix;
}

/** The title page's admin actions. */
interface LibraryTitleFix {
  /** Update details: settles once the details are fetched; rejects when they couldn't be. */
  onUpdateDetails: () => Promise<void>;
  /** Check files again: settles once the files are queued to be read; rejects when they couldn't be. */
  onCheckFiles: () => Promise<void>;
  /** An episode's own files, for its opened row; rejects when they couldn't be read. */
  onEpisodeFiles?: (episodeId: string) => Promise<readonly LibraryTitleFile[]>;
  /** Changes each time the title is read again, so an open episode's file is read again too. */
  episodeFilesAt?: number;
  /** Opens the title's page in the app that decides; absent when it has none. */
  onOpenApp?: () => void;
  /** Wrong title? when the household's answer, or nothing, decides: searches for the title. */
  onSearch: LibraryNeedSearch;
  /** Says which title it is; rejects when the answer couldn't be saved. */
  onLink: (externalId: LibraryExternalId) => Promise<void>;
  /** Forget my answer: the title goes back to needing the household. */
  onForget: () => Promise<void>;
  /** Remove extra copy: moves the file at path to the trash; rejects when it couldn't. */
  onRemoveExtra: (path: string) => Promise<void>;
  /** Choose: opens Needs you, where the show's episode order is said; absent hides the hint. */
  onChooseOrder?: () => void;
}

export type {
  LibraryPersonLink,
  LibraryTitleAiring,
  LibraryTitleChannel,
  LibraryTitleDetail,
  LibraryTitleEpisode,
  LibraryTitleFile,
  LibraryTitleFix,
  LibraryTitleMatch,
  LibraryTitlePerson,
  LibraryTitlePlaying,
  LibraryTitleProps,
  LibraryTitleSeason,
  LibraryTitleStream,
};
