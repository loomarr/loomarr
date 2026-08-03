import type { ChannelFitDTO } from "@loomarr/api";

interface ChannelOverridePickerProps {
  /** The clip being placed — its name is what the picker is about. */
  clipName: string;
  /** The server's per-channel answer, verbatim (contract 1:1). Already number-sorted. */
  channels: ChannelFitDTO[];
  /**
   * Commit an override for one channel.
   *
   * ⚠ `pinned` and `excluded` are sent as an explicit PAIR rather than a single boolean,
   * because the domain has THREE states: pinned (forced in ahead of the ladder), excluded
   * (never used, and it wins over pinned), and neither — the default, where the clip competes
   * on the ladder like everything else. A single flag would collapse "not pinned" into
   * "excluded" and silently block clips the operator merely did not tick.
   */
  onSet: (channelId: string, next: { pinned: boolean; excluded: boolean }) => void;
  /** Clear BOTH lists for one channel — the mock's "Back to automatic". */
  onReset: (channelId: string) => void;
  /** Which channel's write is in flight, so one row disables rather than the whole list. */
  busyChannelId?: string | null;
  loading?: boolean;
  error?: string | null;
  className?: string;
}

export type { ChannelOverridePickerProps };
