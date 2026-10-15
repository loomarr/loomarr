/** The device-state storage the apps already use (expo-secure-store's async get/set). */
interface LastChannelStorage {
  getItem: (key: string) => Promise<string | null>;
  setItem: (key: string, value: string) => Promise<void>;
}

interface LastChannelStore {
  /** The channel last tuned on this device, or undefined when none is stored or the store can't be read. */
  load: () => Promise<string | undefined>;
  save: (channelId: string) => Promise<void>;
}

export type { LastChannelStorage, LastChannelStore };
