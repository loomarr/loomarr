import type { ClipDTO } from "@loomarr/api";
import { getListFillerMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { server } from "@/test/msw/server";
import { FillerClipList } from "./filler-clip-list";

// The remove buttons are Tooltip triggers, and the add-search hits GET /v1/filler.
const renderList = (ui: ReactElement) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>{ui}</TooltipProvider>
    </QueryClientProvider>,
  );
};

beforeEach(() => {
  server.use(getListFillerMockHandler({ clips: [], total: 0 }));
});

const clip: ClipDTO = {
  hash: "aaaa1111",
  tunarrProgramId: "p1",
  name: "Frosted Flakes",
  kind: "commercial",
  durationMs: 30000,
  tagged: true,
  aiTagged: false,
  playCount: 0,
  playsCounted: true,
};

const base = {
  label: "Always include",
  hint: "Pin specific clips.",
  onChange: vi.fn(),
  excludeIds: [] as string[],
};

describe("FillerClipList", () => {
  // ⚠ **A pin whose clip left the catalog used to render the raw 64-character hash.** This file's
  // own comment called that "the honest signal that the override points at something no longer
  // airable" — honest to someone who already knew, unreadable to everyone else, and silent about
  // the fact that the assembler skips the pin entirely.
  it("names a clip that is no longer in the catalog instead of showing a hash", () => {
    renderList(
      <FillerClipList
        {...base}
        ids={["deadbeefdeadbeefdeadbeef"]}
        resolve={() => undefined}
        resolving={false}
      />,
    );

    expect(screen.getByText("This clip is no longer in your catalog")).toBeInTheDocument();
    expect(screen.queryByText("deadbeefdeadbeefdeadbeef")).not.toBeInTheDocument();
  });

  // ⚠ **The distinction `resolve` alone cannot make.** It returns undefined for "still loading"
  // and for "gone", and announcing the second during the first accuses a perfectly good clip of
  // having been deleted every time the panel opens.
  it("says nothing about a missing clip while the lookup is still in flight", () => {
    renderList(<FillerClipList {...base} ids={["aaaa1111"]} resolve={() => undefined} resolving={true} />);

    expect(screen.queryByText("This clip is no longer in your catalog")).not.toBeInTheDocument();
  });

  it("offers a remove action on a dead pin, so it can be cleared without editing policy", async () => {
    const onChange = vi.fn();
    renderList(
      <FillerClipList
        {...base}
        onChange={onChange}
        ids={["gone-1", "aaaa1111"]}
        resolve={(id) => (id === "aaaa1111" ? clip : undefined)}
        resolving={false}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /no longer in your catalog/ }));

    expect(onChange).toHaveBeenCalledWith(["aaaa1111"]);
  });

  it("still renders a resolved clip with its name and duration", () => {
    renderList(<FillerClipList {...base} ids={["aaaa1111"]} resolve={() => clip} resolving={false} />);

    expect(screen.getByText("Frosted Flakes")).toBeInTheDocument();
    expect(screen.queryByText("This clip is no longer in your catalog")).not.toBeInTheDocument();
  });

  // --- the pod_max clamp (#237) ---

  // ⚠ **The clamp said nothing at all before this.** `filler.pod_max` caps clips per break; pin
  // more and the extras are saved, honoured, and simply cannot all fit — with no count anywhere.
  it("warns when the pin list is longer than a break can play", () => {
    renderList(
      <FillerClipList
        {...base}
        ids={["a", "b", "c", "d", "e"]}
        resolve={() => clip}
        resolving={false}
        cap={4}
      />,
    );

    const note = screen.getByTestId("pod-max-clamp");
    expect(note).toHaveTextContent("A break plays at most 4 clips");
    expect(note).toHaveTextContent("only some of these 5");
  });

  it("stays quiet when the pins fit", () => {
    renderList(<FillerClipList {...base} ids={["a", "b"]} resolve={() => clip} resolving={false} cap={4} />);
    expect(screen.queryByTestId("pod-max-clamp")).not.toBeInTheDocument();
  });

  // ⚠ No cap passed is the EXCLUSION list, where length costs nothing — excluding is a filter, not
  // something that has to fit in a break. A warning there would be noise about a limit that does
  // not apply.
  it("never warns when no cap applies", () => {
    renderList(
      <FillerClipList {...base} ids={["a", "b", "c", "d", "e"]} resolve={() => clip} resolving={false} />,
    );
    expect(screen.queryByTestId("pod-max-clamp")).not.toBeInTheDocument();
  });
});
