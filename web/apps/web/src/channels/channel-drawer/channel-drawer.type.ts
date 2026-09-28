// One block of a row's schedule strip.
interface DrawerBlock {
  key: string;
  // The programme's name; a break has none (it's drawn as a tinted sliver).
  label?: string;
  // Share of the strip, in minutes on the air.
  minutes: number;
  pod: boolean;
}

// One channel in the drawer, already read from the guide.
interface DrawerChannel {
  id: string;
  number: number;
  name: string;
  // What is on now ("Series — “Episode”", or the film), when something is.
  now?: string;
  minutesLeft?: number;
  // A channel with nothing to tune says why instead of what's on.
  offAir?: "building" | "paused";
  blocks: DrawerBlock[];
}

interface ChannelDrawerProps {
  channels: DrawerChannel[];
  tunedId: string;
  // Channel ids, oldest-starred first.
  favourites: string[];
  // Channel ids, newest first.
  recent: string[];
  // Where the now line sits on every row's strip, 0-100.
  nowPercent: number;
  onTune: (channelId: string) => void;
  onFavourite: (channelId: string, favourite: boolean) => void;
  onClose: () => void;
}

export type { ChannelDrawerProps, DrawerBlock, DrawerChannel };
