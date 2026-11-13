import { useCallback, useState } from "react";

/**
 * Whether the Watching controls are showing, and what restarts or ends their inactivity timer. Both
 * callbacks keep their identity across renders: playback re-renders the shell constantly, and a new
 * `dismiss` each time would restart the timer on every render.
 */
const useWatchingControls = () => {
  const [activityKey, setActivityKey] = useState(0);
  const [visible, setVisible] = useState(true);
  const showForActivity = useCallback(() => {
    setVisible(true);
    setActivityKey((key) => key + 1);
  }, []);
  const dismiss = useCallback(() => setVisible(false), []);
  return { activityKey, dismiss, showForActivity, visible };
};

export { useWatchingControls };
