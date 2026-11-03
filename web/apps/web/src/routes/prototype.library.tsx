// PROTOTYPE — throwaway (#2151 step 2). /prototype/library renders the Library surfaces inside the
// real AppShell (stub data, no backend, no auth) in three structurally different page shapes.
// Settings → Library and the wizard step are one design, shown under every variant.
//
//   /prototype/library?variant=A&screen=items&scan=idle&device=desktop
//
// device=phone wraps the same URL in a 390×844 iframe, so the real shell sees a phone-width window
// and switches to its bottom bar by itself.
import { createFileRoute } from "@tanstack/react-router";
import { AppShell } from "@/components/loomarr/shell/app-shell";
import { scanStatus } from "@/library/library-prototype/prototype-data";
import {
  DEVICES,
  type Device,
  SCAN_STATES,
  SCREENS,
  type ScanState,
  type Screen,
} from "@/library/library-prototype/prototype-state";
import { PrototypeSwitcher } from "@/library/library-prototype/prototype-switcher";
import { SettingsLibrary } from "@/library/library-prototype/settings-library";
import { VariantA } from "@/library/library-prototype/variant-a";
import { VariantB } from "@/library/library-prototype/variant-b";
import { VariantC } from "@/library/library-prototype/variant-c";
import { VariantD } from "@/library/library-prototype/variant-d";
import { needsYouCount } from "@/library/library-prototype/variant-d-needs";
import { WizardLibraryStep } from "@/library/library-prototype/wizard-library-step";

const VARIANTS = [
  { key: "A", name: "Table + side sheet", Component: VariantA },
  { key: "B", name: "Split view", Component: VariantB },
  { key: "C", name: "Pages + review mode", Component: VariantC },
  { key: "D", name: "Appliance, plain words", Component: VariantD },
] as const;

interface PrototypeSearch {
  variant: string;
  screen: Screen;
  scan: ScanState;
  device: Device;
  framed?: boolean; // internal: rendered inside the phone iframe
}

const PrototypeLibrary = () => {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const set = (next: Partial<PrototypeSearch>) =>
    void navigate({ search: (prev) => ({ ...prev, ...next }), replace: true });
  const current = VARIANTS.find((v) => v.key === search.variant) ?? VARIANTS[0];
  const status = scanStatus(search.scan);

  if (search.device === "phone" && !search.framed) {
    const qs = new URLSearchParams({
      variant: current.key,
      screen: search.screen,
      scan: search.scan,
      device: "phone",
      framed: "true",
    });
    return (
      <div className="bg-static-950 flex min-h-dvh items-start justify-center py-10">
        <iframe
          title="Phone frame"
          src={`/prototype/library?${qs.toString()}`}
          className="ring-static-800 h-[844px] w-[390px] rounded-[36px] bg-black ring-8"
        />
        <PrototypeSwitcher
          variants={VARIANTS}
          variant={current.key}
          screen={search.screen}
          scan={search.scan}
          device={search.device}
          onChange={set}
        />
      </div>
    );
  }

  const { Component } = current;
  const running = status.progress !== undefined;
  const body =
    search.screen === "settings" ? (
      <SettingsLibrary scan={search.scan} device={search.device} />
    ) : search.screen === "wizard" ? (
      <WizardLibraryStep scan={search.scan} device={search.device} />
    ) : (
      <Component
        screen={search.screen}
        scan={search.scan}
        device={search.device}
        go={(screen) => set({ screen })}
      />
    );

  return (
    <>
      {search.screen === "wizard" ? (
        body
      ) : (
        <AppShell
          isAdmin
          userName="Adela Hale"
          badges={{
            "/prototype/library": current.key === "D" ? needsYouCount(search.scan) : status.unmatched,
            "/requests": 2,
          }}
          progress={
            running && status.progress
              ? { "/prototype/library": status.progress.done / status.progress.total }
              : undefined
          }
        >
          {body}
        </AppShell>
      )}
      {!search.framed && (
        <PrototypeSwitcher
          variants={VARIANTS}
          variant={current.key}
          screen={search.screen}
          scan={search.scan}
          device={search.device}
          onChange={set}
        />
      )}
    </>
  );
};

const Route = createFileRoute("/prototype/library")({
  validateSearch: (raw: Record<string, unknown>): PrototypeSearch => ({
    variant: typeof raw.variant === "string" ? raw.variant : "D",
    screen: SCREENS.includes(raw.screen as Screen) ? (raw.screen as Screen) : "items",
    scan: SCAN_STATES.includes(raw.scan as ScanState) ? (raw.scan as ScanState) : "idle",
    device: DEVICES.includes(raw.device as Device) ? (raw.device as Device) : "desktop",
    framed: raw.framed === true || raw.framed === "true",
  }),
  component: PrototypeLibrary,
});

export { Route };
