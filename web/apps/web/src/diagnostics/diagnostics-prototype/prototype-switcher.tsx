import { ArrowLeft, ArrowRight } from "lucide-react";
import { useEffect } from "react";
import { Button } from "@/components/ui/button";

type PrototypeVariant = "A" | "B" | "C";

const labels: Record<PrototypeVariant, string> = {
  A: "Live stream",
  B: "Incident triage",
  C: "Readable timeline",
};

const order: PrototypeVariant[] = ["A", "B", "C"];

const PrototypeSwitcher = ({
  variant,
  onChange,
}: {
  variant: PrototypeVariant;
  onChange: (variant: PrototypeVariant) => void;
}) => {
  const move = (offset: number) => {
    const current = order.indexOf(variant);
    onChange(order[(current + offset + order.length) % order.length] ?? "A");
  };
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target;
      if (
        target instanceof HTMLInputElement ||
        target instanceof HTMLTextAreaElement ||
        (target instanceof HTMLElement && target.isContentEditable)
      )
        return;
      if (event.key === "ArrowLeft") move(-1);
      if (event.key === "ArrowRight") move(1);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  });
  return (
    <div className="fixed bottom-5 left-1/2 z-50 flex -translate-x-1/2 items-center gap-2 rounded-full border border-signal/40 bg-static-950 px-2 py-2 text-static-100 shadow-xl">
      <Button variant="ghost" size="icon" onClick={() => move(-1)} aria-label="Previous prototype">
        <ArrowLeft aria-hidden />
      </Button>
      <span className="min-w-40 text-center font-medium text-sm">
        {variant} — {labels[variant]}
      </span>
      <Button variant="ghost" size="icon" onClick={() => move(1)} aria-label="Next prototype">
        <ArrowRight aria-hidden />
      </Button>
    </div>
  );
};

export type { PrototypeVariant };
export { PrototypeSwitcher };
