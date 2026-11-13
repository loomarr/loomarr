/** "channel_degraded" as "Channel Degraded": a provider type's event or outcome in the words a person reads. */
const words = (value: string) =>
  value
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");

export { words };
