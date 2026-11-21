// PROTOTYPE — throwaway (#4476 A1). /prototype/library draws the redesigned Library wall, Needs you
// and the title page inside the real AppShell from stub data (no backend, no auth). Every view is a
// URL:
//
//   /prototype/library?screen=wall&state=needs&role=admin&tab=all&device=desktop
//
// screen: wall | needs | title
// state (wall): needs | clear | importing | loading | search-empty | home-video
// state (needs): several | undo | folder-undo | ignored | clear
// state (title): default | fix-open | tech-open | extra | extra-undo | airing | unplayable
// title (title screen): movie | series | home
// device=phone frames the same URL in a 390×844 iframe, so the shell switches to its bottom bar.
import { createFileRoute } from "@tanstack/react-router";
import { AppShell } from "@/components/loomarr/shell/app-shell";
import { NEEDS } from "@/library/library-prototype/mock-data";
import { type NeedsState, NeedsYou } from "@/library/library-prototype/mock-needs";
import { type TitleKind, TitlePage, type TitleState } from "@/library/library-prototype/mock-title";
import { LibraryWall, type Role, type Tab, type WallState } from "@/library/library-prototype/mock-wall";

const SCREENS = ["wall", "needs", "title"] as const;
type Screen = (typeof SCREENS)[number];

interface Search {
  screen: Screen;
  state: string;
  role: Role;
  tab: Tab;
  title: TitleKind;
  device: "desktop" | "phone";
  framed?: boolean;
}

const pick = <T extends string>(raw: unknown, allowed: readonly T[], fallback: T): T =>
  allowed.includes(raw as T) ? (raw as T) : fallback;

const PrototypeLibrary = () => {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const set = (next: Partial<Search>) => void navigate({ search: (prev) => ({ ...prev, ...next }), replace: true });

  if (search.device === "phone" && !search.framed) {
    const qs = new URLSearchParams({ ...search, framed: "true" } as unknown as Record<string, string>);
    return (
      <div className="bg-static-950 flex min-h-dvh items-start justify-center py-10">
        <iframe
          className="ring-static-800 h-[844px] w-[390px] rounded-[36px] bg-black ring-8"
          src={`/prototype/library?${qs.toString()}`}
          title="Phone frame"
        />
      </div>
    );
  }

  const admin = search.role === "admin";
  const phone = search.device === "phone";
  const toWall = () => set({ screen: "wall", state: "needs" });
  const body =
    search.screen === "title" ? (
      <TitlePage
        admin={admin}
        key={`${search.title}-${search.state}-${search.role}`}
        kind={search.title}
        onBack={toWall}
        phone={phone}
        state={pick(
          search.state,
          ["default", "fix-open", "tech-open", "extra", "extra-undo", "airing", "unplayable"] as TitleState[],
          "default",
        )}
      />
    ) : search.screen === "needs" && admin ? (
      <NeedsYou
        key={search.state}
        onBack={toWall}
        phone={phone}
        state={pick(search.state, ["several", "undo", "folder-undo", "ignored", "clear"] as NeedsState[], "several")}
      />
    ) : (
      <LibraryWall
        key={`${search.state}-${search.tab}-${search.role}`}
        onReview={() => set({ screen: "needs", state: "several" })}
        phone={phone}
        role={search.role}
        state={pick(search.state, ["needs", "clear", "importing", "loading", "search-empty", "home-video"] as WallState[], "needs")}
        tab={search.tab}
      />
    );

  const needs = search.state === "clear" ? 0 : search.state === "importing" ? 3 : NEEDS.length;
  return (
    <AppShell
      badges={admin ? { "/library": needs, "/requests": 2 } : {}}
      isAdmin={admin}
      userName={admin ? "Adela Hale" : "Rory Hale"}
      watchChannelId="c1"
    >
      {body}
    </AppShell>
  );
};

const Route = createFileRoute("/prototype/library")({
  validateSearch: (raw: Record<string, unknown>): Search => ({
    screen: pick(raw.screen, SCREENS, "wall"),
    state: typeof raw.state === "string" ? raw.state : "needs",
    role: pick(raw.role, ["admin", "member"] as const, "admin"),
    tab: pick(raw.tab, ["all", "movies", "shows", "collections"] as const, "all"),
    title: pick(raw.title, ["movie", "series", "home"] as const, "movie"),
    device: pick(raw.device, ["desktop", "phone"] as const, "desktop"),
    framed: raw.framed === true || raw.framed === "true",
  }),
  component: PrototypeLibrary,
});

export { Route };
