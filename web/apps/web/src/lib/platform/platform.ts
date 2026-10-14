// Whether the viewer is on an Apple platform, where the command key is ⌘ rather than Ctrl. The
// palette opens on either (`use-command-shortcut`); this only decides which one the UI NAMES, so a
// wrong guess mislabels a hint and never breaks the shortcut.
//
// `userAgentData.platform` first (Chromium), then the older `navigator.platform` (Safari, Firefox).
// iPadOS reports "MacIntel", which is right here: an iPad with a keyboard has ⌘.
type PlatformNavigator = Pick<Navigator, "platform"> & { userAgentData?: { platform?: string } };

const isApplePlatform = (nav: PlatformNavigator | undefined = globalThis.navigator) => {
  const platform = nav?.userAgentData?.platform || nav?.platform || "";
  return /mac|iphone|ipad|ipod/i.test(platform);
};

// The label for the palette shortcut, in the form the web mock draws it (`⌘K`).
const commandShortcutLabel = (nav?: PlatformNavigator) => (isApplePlatform(nav) ? "⌘K" : "Ctrl K");

// The same shortcut for `aria-keyshortcuts`, which names keys rather than drawing them.
const commandShortcutAria = (nav?: PlatformNavigator) => (isApplePlatform(nav) ? "Meta+K" : "Control+K");

export type { PlatformNavigator };
export { commandShortcutAria, commandShortcutLabel, isApplePlatform };
