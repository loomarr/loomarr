import type { RequestEntry, TitleDTO } from "@loomarr/core/requests";

type RequestDetailProps = {
  /** Absent when the request isn't in the person's list (an old link, or one that was removed). */
  entry: RequestEntry | undefined;
  onBack: () => void;
  onFix: (entry: RequestEntry) => void;
  onOpenChannel: (channelId: string) => void;
  titles: readonly TitleDTO[];
};

export type { RequestDetailProps };
