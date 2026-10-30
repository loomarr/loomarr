/** One language choice: "Auto (default)" / "Off", or a track's language. */
interface ChannelAudioOption {
  label: string;
  value: string;
}

/** One of the card's pickers: its choices, the saved one, and the autosave. */
interface ChannelLanguagePicker {
  options: readonly ChannelAudioOption[];
  onValueChange: (value: string) => void;
  value: string;
}

interface ChannelAudioSectionProps {
  /** Auto first, then the airing programme's languages, then a saved one it doesn't carry. */
  options: readonly ChannelAudioOption[];
  onValueChange: (value: string) => void;
  value: string;
  /** Burned-in subtitles: Off first, then the airing programme's image-subtitle languages. */
  subtitles: ChannelLanguagePicker;
}

export type { ChannelAudioOption, ChannelAudioSectionProps };
