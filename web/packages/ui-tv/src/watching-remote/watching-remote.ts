import type { TvWatchingRemotePort } from "./watching-remote.type";

/** Maps the Watching remote contract without leaking React Native TV events into product state. */
const handleTvWatchingRemoteEvent = (
  eventType: string,
  hasPendingNumber: boolean,
  port: TvWatchingRemotePort,
): void => {
  if (port.enterNumber(eventType)) {
    port.revealOverlay();
    return;
  }
  if (eventType === "select") {
    if (hasPendingNumber) port.commitNumber();
    else port.openGuide();
    return;
  }
  if (eventType === "up" || eventType === "channelUp") {
    port.step(1);
    return;
  }
  if (eventType === "down" || eventType === "channelDown") {
    port.step(-1);
    return;
  }
  if (eventType === "left" || eventType === "menu") {
    port.openSurf();
    return;
  }
  if (eventType === "play" || eventType === "pause" || eventType === "playPause") {
    if (eventType === "play") port.play();
    else if (eventType === "pause") port.pause();
    else port.togglePlayback();
    port.revealOverlay();
    return;
  }
  port.revealOverlay();
};

export { handleTvWatchingRemoteEvent };
