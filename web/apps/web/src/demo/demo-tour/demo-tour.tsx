import * as setupApi from "@loomarr/api/endpoints/setup";
import { DemoBanner } from "@loomarr/ui";
import { useQuery } from "@tanstack/react-query";
import { Link, useLocation } from "@tanstack/react-router";
import { type ReactElement, useEffect, useState } from "react";

// The demo tour (#2268): on a demo site, a banner at the top of every page says so and offers the
// next thing to try. Whether this is a demo site comes from the SERVER (GET /v1/setup/state, the
// same request the sign-in screen reads), never from a build flag.
//
// A step is done once its page has been visited (nav is path-based), so the Watch player, which
// has no banner of its own, still ticks off step 1. Progress lives in this browser only: it is a
// convenience, and a visitor who clears it just sees the tour again.

type StepId = "watch" | "guide" | "channel";

const STEPS: readonly { id: StepId; label: string; visited: (pathname: string) => boolean }[] = [
  { id: "watch", label: "Watch a channel live", visited: (p) => p.startsWith("/watch/") },
  { id: "guide", label: "Browse the Guide", visited: (p) => p === "/guide" || p.startsWith("/guide/") },
  { id: "channel", label: "Look inside a channel", visited: (p) => p.startsWith("/channels/") },
];

const STORAGE_KEY = "loomarr.demo-tour";

type Progress = { visited: StepId[]; dismissed: boolean };

const isStepId = (value: unknown): value is StepId => STEPS.some((step) => step.id === value);

// Storage can be missing or throw (a private window, blocked site data), and its contents are
// whatever an earlier version wrote, so every read falls back to a fresh tour.
const readProgress = (): Progress => {
  try {
    const raw = JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? "null") as Partial<Progress> | null;
    return {
      visited: Array.isArray(raw?.visited) ? raw.visited.filter(isStepId) : [],
      dismissed: raw?.dismissed === true,
    };
  } catch {
    return { visited: [], dismissed: false };
  }
};

const writeProgress = (progress: Progress) => {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(progress));
  } catch {
    // Not saved: the tour simply starts again next time.
  }
};

type DemoTourState = {
  /** The server says this is a demo site and this viewer hasn't hidden the tour. */
  show: boolean;
  progress: Progress;
  dismiss: () => void;
};

// useDemoTour records visits on every authenticated page, the full-bleed player included, so call
// it from the layout rather than from the banner.
const useDemoTour = (): DemoTourState => {
  const setupState = useQuery(setupApi.getSetupStateQueryOptions({ query: { retry: false } }));
  // Optional chaining on `demo`: a server older than demo mode doesn't send it.
  const enabled = setupState.data?.status === 200 && setupState.data.data.demo?.enabled === true;
  const { pathname } = useLocation();
  const [progress, setProgress] = useState(readProgress);

  useEffect(() => {
    const step = enabled ? STEPS.find((candidate) => candidate.visited(pathname)) : undefined;
    if (!step) return;
    setProgress((current) =>
      current.visited.includes(step.id) ? current : { ...current, visited: [...current.visited, step.id] },
    );
  }, [enabled, pathname]);

  useEffect(() => {
    if (enabled) writeProgress(progress);
  }, [enabled, progress]);

  return {
    show: enabled && !progress.dismissed,
    progress,
    dismiss: () => setProgress((current) => ({ ...current, dismissed: true })),
  };
};

// The page each step opens. Without a channel to point at, both channel steps fall back to the
// Guide, where every channel is.
const stepLink = (id: StepId, channelId: string | undefined): ReactElement<Record<string, unknown>> => {
  if (id === "watch" && channelId) return <Link to="/watch/$id" params={{ id: channelId }} />;
  if (id === "channel" && channelId) return <Link to="/channels/$id" params={{ id: channelId }} />;
  return <Link to="/guide" />;
};

type DemoTourProps = {
  tour: DemoTourState;
  /** The channel the steps open: the same one the Watch nav item plays. */
  channelId: string | undefined;
};

const DemoTour = ({ tour, channelId }: DemoTourProps) => {
  if (!tour.show) return null;
  const index = STEPS.findIndex((step) => !tour.progress.visited.includes(step.id));
  const next = index === -1 ? undefined : STEPS[index];
  return (
    <DemoBanner
      next={next ? { label: next.label, link: stepLink(next.id, channelId) } : undefined}
      onDismiss={tour.dismiss}
      step={index + 1}
      stepCount={STEPS.length}
    />
  );
};

export type { DemoTourProps, DemoTourState };
export { DemoTour, useDemoTour };
