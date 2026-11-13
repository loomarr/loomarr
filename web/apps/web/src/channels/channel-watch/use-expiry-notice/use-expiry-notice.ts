import { useEffect, useRef } from "react";
import { toast } from "sonner";

// Says once per notice that a paused point has gone and the viewer is back live. The revision counts
// the transport's notices; a different channel starts the count again.
const useExpiryNotice = (channelId: string, noticeRevision: number) => {
  const seen = useRef({ channelId, revision: 0 });
  useEffect(() => {
    if (seen.current.channelId !== channelId) {
      seen.current = { channelId, revision: 0 };
    }
    if (noticeRevision <= seen.current.revision) return;
    seen.current.revision = noticeRevision;
    toast.info("That paused point is no longer available, so you're back live.");
  }, [channelId, noticeRevision]);
};

export { useExpiryNotice };
