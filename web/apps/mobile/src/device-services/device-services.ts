import { createPairingCredentialStore } from "@loomarr/core/pairing";
import type { RequestsController } from "@loomarr/core/requests";
import type { HapticFeedback } from "@loomarr/design-system/haptics";
import { createLastChannelStore } from "@loomarr/paired-client";
import type { ApprovalBrowser } from "@loomarr/ui";
import * as SecureStore from "expo-secure-store";
import * as WebBrowser from "expo-web-browser";
import { Platform } from "react-native";

const credentialStore = createPairingCredentialStore({
  deleteItem: SecureStore.deleteItemAsync,
  getItem: SecureStore.getItemAsync,
  setItem: SecureStore.setItemAsync,
});

// What the device remembers: the channel last watched (#23), which the Guide starts on and Watching
// tunes first, and the viewer's subtitles.
const deviceStorage = { getItem: SecureStore.getItemAsync, setItem: SecureStore.setItemAsync };
const lastChannel = createLastChannelStore(deviceStorage);

// The phone approves itself on the server's `/pair` page, in a browser sheet over the app (an
// SFSafariViewController, or a Custom Tab on Android); the pairing shell dismisses it once paired.
// ⚠ Android has no dismissBrowser: Expo returns undefined there, not a promise, and the `.catch` on
// it threw as every Android phone finished pairing. The viewer leaves the Custom Tab themselves.
const approvalBrowser: ApprovalBrowser = {
  close: () => void Promise.resolve(WebBrowser.dismissBrowser()).catch(() => undefined),
  name: Platform.OS === "ios" ? "Safari" : "your browser",
  open: (url) => {
    void WebBrowser.openBrowserAsync(url, {
      presentationStyle: WebBrowser.WebBrowserPresentationStyle.PAGE_SHEET,
    });
  },
};

// A request decided (approved or denied) taps success; a refused one stays silent.
const withDecisionHaptic = (controller: RequestsController, haptic: HapticFeedback): RequestsController => {
  const tapWhenDone =
    <Args extends unknown[], Result extends { kind: string }>(decide: (...args: Args) => Promise<Result>) =>
    async (...args: Args) => {
      const result = await decide(...args);
      if (result.kind === "done") haptic.success();
      return result;
    };
  return {
    ...controller,
    approve: tapWhenDone(controller.approve),
    approveFillerPull: tapWhenDone(controller.approveFillerPull),
    approveFillerPulls: tapWhenDone(controller.approveFillerPulls),
    approveMany: tapWhenDone(controller.approveMany),
    deny: tapWhenDone(controller.deny),
  };
};

export { approvalBrowser, credentialStore, deviceStorage, lastChannel, withDecisionHaptic };
