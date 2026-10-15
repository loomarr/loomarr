import type { LastChannelStorage, LastChannelStore } from "./last-channel.type";

const LAST_CHANNEL_KEY = "loomarr.lastChannelId";

/**
 * The channel a device last watched (#23), so a relaunch resumes it instead of channel 101. Storage is
 * injected (the apps' SecureStore, as for the TV's auto-tune wait) so this stays testable without a
 * native module. Whether the id is still in the catalog is the controller's call, not the store's.
 */
const createLastChannelStore = (storage: LastChannelStorage): LastChannelStore => ({
  async load() {
    try {
      return (await storage.getItem(LAST_CHANNEL_KEY)) || undefined;
    } catch {
      // An unreadable store is the same as nothing remembered: the app starts on its first channel.
      return undefined;
    }
  },
  async save(channelId) {
    await storage.setItem(LAST_CHANNEL_KEY, channelId);
  },
});

export { createLastChannelStore };
