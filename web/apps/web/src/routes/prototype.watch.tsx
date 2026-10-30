// PROTOTYPE — throwaway (#2088 step 0). /prototype/watch renders the full-bleed Watch player in
// three structurally different variants, each in every state, on a desktop or phone frame. Stub data,
// no backend, no auth: the question is only "what should /watch look like".
//
//   /prototype/watch?variant=A&state=playing&device=desktop
import { createFileRoute } from "@tanstack/react-router";
import {
  DEVICES,
  type Device,
  WATCH_STATES,
  type WatchState,
} from "@/channels/watch-prototype/prototype-state";
import { PrototypeSwitcher } from "@/channels/watch-prototype/prototype-switcher";
import { VariantA } from "@/channels/watch-prototype/variant-a";
import { VariantB, VariantB1, VariantB2 } from "@/channels/watch-prototype/variant-b";
import { VariantC } from "@/channels/watch-prototype/variant-c";

const VARIANTS = [
  { key: "A", name: "Overlay classic", Component: VariantA },
  { key: "B", name: "Lower third", Component: VariantB },
  { key: "B1", name: "Lower third · filmstrip bar", Component: VariantB1 },
  { key: "B2", name: "Lower third · hover previews", Component: VariantB2 },
  { key: "C", name: "Side rail", Component: VariantC },
] as const;

interface PrototypeSearch {
  variant: string;
  state: WatchState;
  device: Device;
}

const PrototypeWatch = () => {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const current = VARIANTS.find((v) => v.key === search.variant) ?? VARIANTS[0];
  const { Component } = current;
  const frame =
    search.device === "portrait"
      ? "h-[844px] w-[390px] rounded-[36px] ring-8 ring-static-800"
      : search.device === "landscape"
        ? "h-[390px] w-[844px] rounded-[36px] ring-8 ring-static-800"
        : "h-dvh w-full";
  return (
    <div
      className={`bg-static-950 flex min-h-dvh items-center justify-center ${search.device === "desktop" ? "" : "py-16"}`}
    >
      <div className={`relative overflow-hidden bg-black ${frame}`}>
        <Component state={search.state} device={search.device} />
      </div>
      <PrototypeSwitcher
        variants={VARIANTS}
        variant={current.key}
        state={search.state}
        device={search.device}
        onChange={(next) => void navigate({ search: (prev) => ({ ...prev, ...next }), replace: true })}
      />
    </div>
  );
};

const Route = createFileRoute("/prototype/watch")({
  validateSearch: (raw: Record<string, unknown>): PrototypeSearch => ({
    variant: typeof raw.variant === "string" ? raw.variant : "A",
    state: WATCH_STATES.includes(raw.state as WatchState) ? (raw.state as WatchState) : "playing",
    device: DEVICES.includes(raw.device as Device) ? (raw.device as Device) : "desktop",
  }),
  component: PrototypeWatch,
});

export { Route };
