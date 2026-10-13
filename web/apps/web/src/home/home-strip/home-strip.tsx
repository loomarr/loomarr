import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import type { HomeStripProps } from "./home-strip.type";

const plural = (n: number, one: string, many: string) => (n === 1 ? one : many);

// HomeStrip — the top of Home (#1822 evidence: "Neutral channel inventory count"). Services not
// answering, requests needing a decision and a pending restart now each have their own surface —
// the Requests nav badge and Settings → This server's Diagnostics (#1659, Q-H7) — so Home no
// longer repeats them; this is just where things stand, truthfully and without a verdict.
//
// Loading, zero-channels and initial-error are whole-page states (the record's state contract):
// HomePage renders nothing else alongside them, which is why this component owns their copy too
// rather than leaving the caller to duplicate it per state.
const HomeStrip = ({ state, count, isAdmin, onRetry }: HomeStripProps) => {
  if (state === "loading") {
    return (
      <section aria-busy="true">
        <p className="m-0 text-sm text-static-400">Loading your channels…</p>
        <div aria-hidden className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {[0, 1, 2].map((i) => (
            <div key={i} className="aspect-video w-full animate-pulse rounded-md bg-static-800" />
          ))}
        </div>
      </section>
    );
  }

  if (state === "error") {
    return (
      <section className="flex max-w-[460px] flex-col items-start gap-4 py-10">
        <h2 className="m-0 font-semibold text-2xl">Couldn’t load your channels.</h2>
        <Button onClick={onRetry}>Try again</Button>
      </section>
    );
  }

  if (state === "empty") {
    return (
      <section className="flex max-w-[460px] flex-col items-start gap-4 py-10">
        <h2 className="m-0 font-semibold text-2xl">Your first channel starts here</h2>
        <p className="m-0 text-static-400">{isAdmin ? "Create" : "Request"} a channel to start watching.</p>
        <Button render={<Link to="/guide" search={{ new: "1" }} />}>
          {isAdmin ? "Add your first channel" : "Request a channel"}
        </Button>
      </section>
    );
  }

  return (
    <p className="m-0 text-sm">
      <strong className="font-medium">
        {count} {plural(count, "channel", "channels")}
      </strong>
    </p>
  );
};

export { HomeStrip };
