import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { UpdateChannelInputBody } from "@loomarr/api/models/updateChannelInputBody";
import type { ReactNode } from "react";
import { createContext, useContext } from "react";
import type { OnAirState } from "@/components/loomarr/channels/on-air-indicator";

// The channel-detail layout's shared state (V-nav-paths), read by each section route
// (`info.tsx`, `programming.tsx`, `filler.tsx`, `danger.tsx`). Previously all of this lived
// in one component switching on `activeId === "..."`; splitting the sections into their own
// routes means each one needs the channel, the mutators and the derived air-state without
// re-fetching or re-deriving them — the same reasoning `SettingsEditsProvider` hoists the
// edit buffer above Settings' `<Outlet/>`.
//
// ⚠ The layout (`route.tsx`) gates loading/error BEFORE rendering `<Outlet/>`, so every
// consumer of this context can assume `channel` is the loaded ChannelDTO, never undefined.
type AirState = { dot: OnAirState; label: string; detail: string };

type ChannelDetailValue = {
  id: string;
  channel: ChannelDTO;
  isAdmin: boolean;
  air: AirState;
  /** Titles actually in the library — see route.tsx for why this counts titles, not slots. */
  ready: number;
  total: number;
  invalidate: () => void;
  saving: boolean;
  deleting: boolean;
  update: (data: Omit<UpdateChannelInputBody, "revision">) => void;
  updateAsync: (data: Omit<UpdateChannelInputBody, "revision">) => Promise<unknown>;
  savePolicy: (policy: ChannelPolicy) => void;
  /** `""` clears it — the icon is just another field on the same PATCH (§7). */
  saveLogo: (logo: string) => Promise<void>;
  onDelete: (opts: { purge: boolean }) => void;
};

const ChannelDetailContext = createContext<ChannelDetailValue | undefined>(undefined);

const ChannelDetailProvider = ({ value, children }: { value: ChannelDetailValue; children: ReactNode }) => (
  <ChannelDetailContext.Provider value={value}>{children}</ChannelDetailContext.Provider>
);

// Throws outside the layout rather than falling back — a silent fallback here would render a
// section with no channel to show, which is worse than a clear "used outside its route" error.
const useChannelDetail = (): ChannelDetailValue => {
  const ctx = useContext(ChannelDetailContext);
  if (!ctx) throw new Error("useChannelDetail must be used inside the channel-detail layout (route.tsx)");
  return ctx;
};

export type { ChannelDetailValue };
export { ChannelDetailProvider, useChannelDetail };
