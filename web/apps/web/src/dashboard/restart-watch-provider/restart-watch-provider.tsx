import { createContext, type ReactNode, useContext } from "react";
import { useRestartWatch } from "../use-restart-watch";

// RestartWatchProvider lifts the restart watch to the app shell (§9.2, V13).
//
// ⚠ **A restart is an APP-WIDE event, not a Dashboard one.** The control lives on the
// Dashboard, but the operator can navigate away the moment they click it — and every page
// is equally broken while the server is down. Holding the state here means the overlay
// and the banner follow them, instead of vanishing the moment they change route and
// leaving a dead app with no explanation.

type RestartWatchValue = ReturnType<typeof useRestartWatch>;

const RestartWatchContext = createContext<RestartWatchValue | null>(null);

const RestartWatchProvider = ({ children }: { children: ReactNode }) => {
  const watch = useRestartWatch();
  return <RestartWatchContext.Provider value={watch}>{children}</RestartWatchContext.Provider>;
};

// useRestartWatchContext returns the shared watch.
//
// Falls back to an inert value rather than throwing when no provider is present: a
// component rendered in isolation (a story, a unit test) is not a bug, and a hook that
// throws there would make every such test set up a provider it does not care about.
const INERT: RestartWatchValue = {
  restarting: false,
  justCameBack: false,
  failed: null,
  begin: () => {},
};

const useRestartWatchContext = (): RestartWatchValue => useContext(RestartWatchContext) ?? INERT;

export { RestartWatchProvider, useRestartWatchContext };
