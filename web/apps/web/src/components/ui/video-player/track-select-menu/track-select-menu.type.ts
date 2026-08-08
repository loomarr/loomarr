import type { LucideIcon } from "lucide-react";

interface TrackOption {
  value: string;
  label: string;
}

interface TrackSelectMenuProps {
  /** The trigger icon — a speaker for audio, a CC glyph for subtitles. */
  icon: LucideIcon;
  /** Accessible name + menu heading, e.g. "Audio" / "Subtitles". */
  label: string;
  /** The choices; exactly one is `value` at a time. */
  options: TrackOption[];
  /** The current selection (gets a check). */
  value: string;
  /** Pick an option. */
  onChange: (value: string) => void;
  /** Read-only: a member sees the current track but cannot change it — audio/subtitles are
   *  admin-scoped and channel-wide (§9.1), so a non-admin's menu is disabled with a note. */
  readOnly?: boolean;
}

export type { TrackOption, TrackSelectMenuProps };
