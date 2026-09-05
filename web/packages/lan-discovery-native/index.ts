import type {
  DiscoveredServer,
  ServerDiscovery,
  ServerDiscoverySnapshot,
} from "@loomarr/core/server-discovery";
import { NativeEventEmitter, NativeModules, Platform } from "react-native";

type NativeDiscoveryModule = {
  addListener(event: string): void;
  removeListeners(count: number): void;
  start(): void;
  stop(): void;
};

type NativeServer = DiscoveredServer & { protocol: string };
const nativeModule = NativeModules.LoomarrLanDiscovery as NativeDiscoveryModule | undefined;

const createNativeServerDiscovery = (): ServerDiscovery => {
  let snapshot: ServerDiscoverySnapshot = {
    servers: [],
    status: Platform.OS === "android" && nativeModule ? "idle" : "unavailable",
  };
  const listeners = new Set<() => void>();
  let subscriptions: Array<{ remove(): void }> = [];
  const publish = (next: ServerDiscoverySnapshot) => {
    snapshot = next;
    for (const listener of listeners) listener();
  };
  const stop = () => {
    for (const subscription of subscriptions) subscription.remove();
    subscriptions = [];
    nativeModule?.stop();
  };

  return {
    snapshot: () => snapshot,
    start() {
      if (!nativeModule || Platform.OS !== "android") {
        publish({
          error: "Automatic discovery is unavailable on this platform.",
          servers: [],
          status: "unavailable",
        });
        return;
      }
      stop();
      publish({ servers: [], status: "searching" });
      const events = new NativeEventEmitter(nativeModule);
      subscriptions = [
        events.addListener("loomarrDiscoveryFound", (server: NativeServer) => {
          if (server.protocol !== "1" || !server.id || !server.name || !server.url) return;
          const byId = new Map(snapshot.servers.map((candidate) => [candidate.id, candidate]));
          byId.set(server.id, { id: server.id, name: server.name, url: server.url });
          publish({
            servers: [...byId.values()].sort((left, right) => left.name.localeCompare(right.name)),
            status: "searching",
          });
        }),
        events.addListener("loomarrDiscoveryLost", ({ id }: { id?: string }) => {
          if (!id) return;
          publish({ ...snapshot, servers: snapshot.servers.filter((server) => server.id !== id) });
        }),
        events.addListener("loomarrDiscoveryError", () => {
          publish({
            error: "Couldn't search this network. You can still enter the address manually.",
            servers: snapshot.servers,
            status: "unavailable",
          });
        }),
      ];
      nativeModule.start();
    },
    stop,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
};

export { createNativeServerDiscovery };
