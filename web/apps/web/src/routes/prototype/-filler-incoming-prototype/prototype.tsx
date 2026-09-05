import { ArrowLeft, ArrowRight } from "lucide-react";
import { useEffect } from "react";
import { Button } from "@/components/ui/button";
import { VariantConsole } from "./variant-console";
import { VariantFlow } from "./variant-flow";
import { VariantReview } from "./variant-review";

// PROTOTYPE — three read-only answers to “what should the filler pipeline feel like?”, switchable
// through `?variant=review|flow|console` on the existing /filler/incoming route. Do not promote
// this code directly; fold the chosen hierarchy into the production modules and remove the rest.
type FillerPrototypeVariant = "review" | "flow" | "console";

const variants: { id: FillerPrototypeVariant; label: string }[] = [
  { id: "review", label: "Decision desk" },
  { id: "flow", label: "Pipeline map" },
  { id: "console", label: "Safety console" },
];

const FillerIncomingPrototype = ({
  variant,
  onVariantChange,
}: {
  variant: FillerPrototypeVariant;
  onVariantChange: (variant: FillerPrototypeVariant) => void;
}) => {
  const index = variants.findIndex((entry) => entry.id === variant);
  const cycle = (direction: -1 | 1) => {
    const next = (index + direction + variants.length) % variants.length;
    onVariantChange(variants[next]?.id ?? "review");
  };

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (target?.matches("input, textarea, [contenteditable='true']")) return;
      if (event.key === "ArrowLeft") cycle(-1);
      if (event.key === "ArrowRight") cycle(1);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  });

  return (
    <>
      {variant === "review" ? <VariantReview /> : variant === "flow" ? <VariantFlow /> : <VariantConsole />}
      <div className="fixed bottom-5 left-1/2 z-50 flex -translate-x-1/2 items-center gap-2 rounded-full border border-static-600 bg-static-950 px-2 py-1.5 text-static-50 shadow-2xl">
        <Button
          variant="ghost"
          size="icon"
          className="rounded-full text-static-50 hover:bg-static-800"
          onClick={() => cycle(-1)}
          aria-label="Previous prototype"
        >
          <ArrowLeft className="size-4" />
        </Button>
        <span className="min-w-40 text-center font-medium text-xs">
          {String.fromCharCode(65 + index)} — {variants[index]?.label}
        </span>
        <Button
          variant="ghost"
          size="icon"
          className="rounded-full text-static-50 hover:bg-static-800"
          onClick={() => cycle(1)}
          aria-label="Next prototype"
        >
          <ArrowRight className="size-4" />
        </Button>
      </div>
    </>
  );
};

export type { FillerPrototypeVariant };
export { FillerIncomingPrototype };
