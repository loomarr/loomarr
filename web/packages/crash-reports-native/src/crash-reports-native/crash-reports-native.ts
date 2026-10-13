import type { CrashReportDevice, CrashReporter, NativeCrashEnvelope } from "@loomarr/core/crash-reports";
import { NativeEventEmitter, NativeModules } from "react-native";

type NativeDevice = Omit<CrashReportDevice, "platform">;
type NativeCrashReportsModule = Partial<NativeDevice> & {
  addListener(event: string): void;
  getConstants?: () => Partial<NativeDevice>;
  pending(): Promise<unknown[]>;
  remove(id: string): Promise<void>;
  removeListeners(count: number): void;
  start(): void;
};

/** MetricKit payloads the native module is holding, and its record of this build and device. */
type NativeCrashSource = {
  device: Partial<NativeDevice>;
  pending(): Promise<NativeCrashEnvelope[]>;
  remove(id: string): Promise<void>;
  /** Called when MetricKit delivers more payloads while the app runs. */
  subscribe(listener: () => void): () => void;
};

const isEnvelope = (value: unknown): value is NativeCrashEnvelope => {
  const candidate = value as Partial<NativeCrashEnvelope> | null;
  return (
    typeof candidate?.id === "string" &&
    typeof candidate.endedAt === "string" &&
    typeof candidate.payload === "string"
  );
};

const nonEmpty = (value: unknown) => (typeof value === "string" && value !== "" ? value : undefined);

/**
 * Starts the MetricKit subscription and answers its payloads, or `undefined` where the module is not
 * linked: Android, the TV apps and Expo Go have no MetricKit.
 */
const createNativeCrashSource = (): NativeCrashSource | undefined => {
  const module = NativeModules.LoomarrCrashReports as NativeCrashReportsModule | undefined;
  if (!module) return undefined;
  module.start();
  const constants = module.getConstants?.() ?? module;
  const events = new NativeEventEmitter(module);
  return {
    device: {
      appBuild: nonEmpty(constants.appBuild),
      appVersion: nonEmpty(constants.appVersion),
      deviceModel: nonEmpty(constants.deviceModel),
      osVersion: nonEmpty(constants.osVersion),
    },
    pending: async () => (await module.pending()).filter(isEnvelope),
    remove: (id) => module.remove(id),
    subscribe(listener) {
      const subscription = events.addListener("loomarrCrashPayloadsAvailable", listener);
      return () => subscription.remove();
    },
  };
};

/**
 * Moves every pending MetricKit payload into the reporter's queue, removing each from native storage
 * only once its reports are queued. Answers how many crashes were queued.
 */
const forwardNativeCrashes = async (
  source: Pick<NativeCrashSource, "pending" | "remove">,
  reporter: Pick<CrashReporter, "captureNative">,
): Promise<number> => {
  let queued = 0;
  for (const envelope of await source.pending()) {
    queued += reporter.captureNative(envelope);
    await source.remove(envelope.id);
  }
  return queued;
};

export type { NativeCrashSource };
export { createNativeCrashSource, forwardNativeCrashes };
