// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// The specimen sheet inside the device frame: five labelled panels, scrolling inside the frame.
import type { Tint } from "../shared";
import type { State } from "./data";
import { OneCard } from "./one-card";
import { Today } from "./today";
import { TwoFamilies } from "./two-families";

interface SurfaceProps {
  phone: boolean;
  tint: Tint;
}

const VARIANTS = [
  { key: "A", name: "Today" },
  { key: "B", name: "One MediaCard" },
  { key: "C", name: "Two families" },
] as const;

const STATES = ["normal", "problems"] as const;

const Specimen = ({
  variant,
  state,
  phone,
  tint,
}: {
  variant: string;
  state: string;
  phone: boolean;
  tint: Tint;
}) => {
  const s: State = state === "problems" ? "problems" : "normal";
  const Body = variant === "B" ? OneCard : variant === "C" ? TwoFamilies : Today;
  return (
    <div data-proto-scroll className="h-full overflow-y-auto">
      <div className={`mx-auto flex flex-col gap-8 ${phone ? "p-4 pt-12" : "max-w-[1200px] p-10 pt-14"}`}>
        <Body phone={phone} state={s} tint={tint} />
      </div>
    </div>
  );
};

export type { SurfaceProps };
export { Specimen, STATES, VARIANTS };
