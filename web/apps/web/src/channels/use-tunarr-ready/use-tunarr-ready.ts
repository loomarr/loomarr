import { settingsApi } from "@loomarr/api";

// useTunarrReady reports whether Tunarr is configured (tunarr.url is set). Rebuild/reconcile
// push to Tunarr, so a channel action that needs it must gate on this — otherwise the button
// invites a click that only 501s "scheduler not configured". Both the channels list and the
// detail page read the SAME signal here, so they can't drift on what "can I rebuild?" means.
//
// Returns `true` while the settings are still loading or unreadable, deliberately: the server
// still guards the action, so an optimistic default just avoids flickering the button
// disabled on first paint — it never lets a broken action succeed.
const useTunarrReady = (): boolean => {
  const settings = settingsApi.useSettingsList({ query: { retry: false } });
  if (settings.data?.status !== 200) return true;
  return Boolean(settings.data.data.settings?.find((s) => s.key === "tunarr.url")?.set);
};

export { useTunarrReady };
