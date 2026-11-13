import { useEffect, useState } from "react";
import { BackHandler } from "react-native";

import { type ClientDestination, clientBackDestination } from "../client-navigation";

/** The part of React Native's `BackHandler` the hook uses. */
interface BackHandlerSource {
  addEventListener: (
    type: "hardwareBackPress",
    listener: () => boolean | undefined,
  ) => { remove: () => void };
}

/**
 * The destination a client shell is showing, and the hardware Back that closes it: Back from any
 * transient screen returns to Watching, and from Watching is left to the platform (home).
 */
const useClientDestination = (
  initial: ClientDestination,
  backHandler: BackHandlerSource = BackHandler,
): readonly [ClientDestination, (destination: ClientDestination) => void] => {
  const [active, setActive] = useState<ClientDestination>(initial);
  useEffect(() => {
    const subscription = backHandler.addEventListener("hardwareBackPress", () => {
      const destination = clientBackDestination(active);
      if (!destination) return false;
      setActive(destination);
      return true;
    });
    return () => subscription.remove();
  }, [active, backHandler]);
  return [active, setActive];
};

export type { BackHandlerSource };
export { useClientDestination };
