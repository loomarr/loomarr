import { CHANNEL_TEMPLATES } from "@loomarr/core/templates";
import { Loader2, Sparkles } from "lucide-react";
import { useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { Button } from "@/components/ui/button";
import { useCompleteSetup } from "../use-complete-setup";

interface FirstChannelStepProps {
  // The `llm` setup check (design §13, decision G2): a template still hands off an intent
  // when the LLM is off, but the Guide can't build a lineup from it until AI is configured.
  // Honest up front, rather than letting the operator discover that after the handoff.
  llmReady: boolean;
}

// Wizard step 7 — the guided first channel (§13). It hands off rather than rebuilding:
// picking a template marks setup complete and drops the operator on the Guide — the channels
// surface (§12) — with `?intent=` prefilled, which auto-opens its inline describe panel where
// the real pipeline (generate → review → approve) lives. Templates are the blank-page killer
// (§13) and ship in packages/core, so this step and the panel always offer the same set.
//
// G2 (redesign map, 2026-09-28 decisions): with the LLM off, templates stay offered (picking
// one still saves a draft the Guide holds once AI is connected), and "Finish setup without a
// channel" is the manual path — no channel, nothing to build by hand until a lineup editor
// exists. The one-line reason says why up front instead of letting the operator discover it
// after the handoff, where the Guide's own describe panel repeats it.
const FirstChannelStep = ({ llmReady }: FirstChannelStepProps) => {
  const [chosen, setChosen] = useState<string | undefined>();
  const complete = useCompleteSetup();

  const start = (id: string) => {
    const template = CHANNEL_TEMPLATES.find((t) => t.id === id);
    if (!template) return;
    setChosen(id);
    complete.finish({ to: "/guide", intent: template.description });
  };

  return (
    <div className="flex flex-col gap-4">
      <p className="text-muted-foreground text-sm">
        Pick a starting point. You can edit it before Loomarr builds anything. Since you're an admin, you can
        approve your own lineup and watch it go on the air.
        {!llmReady &&
          " No AI service is connected yet, so Loomarr can't build a lineup from a template until you " +
            "set one up. Pick one anyway to save it as a draft, or finish without a channel for now."}
      </p>

      {complete.error != null && <ErrorState error={complete.error} />}

      <ul className="flex flex-col gap-2">
        {CHANNEL_TEMPLATES.map((t) => (
          <li key={t.id}>
            <button
              type="button"
              disabled={complete.isPending}
              onClick={() => start(t.id)}
              className="flex w-full cursor-pointer items-start gap-3 rounded-md border border-border bg-card px-3 py-2.5 text-left transition-colors hover:border-suggest disabled:opacity-50"
            >
              {complete.isPending && chosen === t.id ? (
                <Loader2 className="mt-0.5 size-4 shrink-0 animate-spin text-suggest" aria-hidden />
              ) : (
                <Sparkles className="mt-0.5 size-4 shrink-0 text-suggest" aria-hidden />
              )}
              <span className="min-w-0">
                <span className="block font-medium text-sm">{t.label}</span>
                <span className="block text-muted-foreground text-sm">{t.description}</span>
              </span>
            </button>
          </li>
        ))}
      </ul>

      <Button
        variant="ghost"
        className="w-fit"
        disabled={complete.isPending}
        onClick={() => complete.finish({ to: "/guide" })}
      >
        Finish setup without a channel
      </Button>
    </div>
  );
};

export { FirstChannelStep };
