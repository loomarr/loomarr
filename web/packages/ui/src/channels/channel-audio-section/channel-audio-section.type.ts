/** One audio language the channel can default to: "Auto (default)" or a track's language. */
interface ChannelAudioOption {
  label: string;
  value: string;
}

interface ChannelAudioSectionProps {
  /** Auto first, then the airing programme's languages, then a saved one it doesn't carry. */
  options: readonly ChannelAudioOption[];
  onValueChange: (value: string) => void;
  value: string;
}

export type { ChannelAudioOption, ChannelAudioSectionProps };
