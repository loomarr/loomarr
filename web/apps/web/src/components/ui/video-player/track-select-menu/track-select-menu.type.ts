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
  /** Per-viewer on/off settings shown under the options (e.g. the channel-change sound). They are the
   *  viewer's own, so `readOnly` never disables them. */
  toggles?: TrackToggle[];
}

interface TrackToggle {
  label: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
}

export type { TrackOption, TrackSelectMenuProps, TrackToggle };
