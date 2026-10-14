interface NumberedChannel {
  id: string;
  number: number;
}

interface UseNumberEntryOptions {
  channels: readonly NumberedChannel[];
  onTune: (channelId: string) => void;
}

interface UseNumberEntry {
  // The digits typed so far; undefined when no entry is open.
  digits?: string;
  // The typed number matched no channel; the readout says so, then clears.
  missed: boolean;
  // How many digits the longest channel number has: the readout's width.
  width: number;
  // Returns false for a key that isn't a digit, so the caller can offer it elsewhere.
  press: (key: string) => boolean;
  // Enter: tune now rather than waiting out the entry.
  commit: () => boolean;
}

export type { NumberedChannel, UseNumberEntry, UseNumberEntryOptions };
