import { useCanGoBack, useNavigate, useRouter } from "@tanstack/react-router";
import { useCallback, useMemo } from "react";

/**
 * The ways out of Watch. The back control goes back where the viewer came from, and is named
 * "Back"; a deep link with no history lands on the Guide, and the control is named for it. Until
 * the Guide is a view of the watching screen, the Guide control and `G` open the Guide page.
 */
const useWatchExits = () => {
  const navigate = useNavigate();
  const router = useRouter();
  const canGoBack = useCanGoBack();
  const back = useMemo(
    () => ({
      label: canGoBack ? "Back" : "Guide",
      onPress: () => {
        if (canGoBack) router.history.back();
        else void navigate({ to: "/guide" });
      },
    }),
    [canGoBack, router, navigate],
  );
  const openGuide = useCallback(() => void navigate({ to: "/guide" }), [navigate]);
  return { back, openGuide };
};

export { useWatchExits };
