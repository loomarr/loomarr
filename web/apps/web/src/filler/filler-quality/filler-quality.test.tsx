import type { FillerDefectReportDTO, FillerDefectRowDTO } from "@loomarr/core/filler-defects";
import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { FillerQuality } from "./filler-quality";

const wrapper = ({ children }: { children: ReactNode }) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <LoomarrProvider theme="dark">{children}</LoomarrProvider>
    </QueryClientProvider>
  );
};

const clip = (name: string): FillerDefectRowDTO => ({
  clipId: name,
  name,
  era: 1988,
  band: "tape",
  outcome: "fail",
  verdict: "fail",
  findings: [{ check: "blur", outcome: "fail", measured: 9.6, bar: 9 }],
  measuredAt: "2026-10-27T12:00:00Z",
});

// A server holding three unlabelled clips that answers the queue's paged reads and its labels.
const serveQueue = (names: string[]) => {
  const queue = [...names];
  const labelled: string[] = [];
  const offsets: number[] = [];
  server.use(
    http.get("*/v1/filler/defects", ({ request }) => {
      const offset = Number(new URL(request.url).searchParams.get("offset") ?? 0);
      offsets.push(offset);
      const body: FillerDefectReportDTO = {
        floorVersion: "floor-v3-provisional",
        clips: names.length,
        unmeasured: 0,
        labelled: labelled.length,
        verdicts: { pass: 0, fail: names.length, undecided: 0, pending: 0 },
        scores: [],
        total: queue.length,
        rows: queue.slice(offset, offset + 1).map(clip),
      };
      return HttpResponse.json(body);
    }),
    http.put("*/v1/filler/defects/:clipId/label", ({ params }) => {
      const id = String(params.clipId);
      labelled.push(id);
      queue.splice(queue.indexOf(id), 1);
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return { labelled, offsets };
};

// The clip on screen is the one the player plays.
const playing = (container: HTMLElement, name: string) =>
  waitFor(() => expect(container.querySelector("video")).toHaveAttribute("src", `/v1/filler/media/${name}`));

describe("FillerQuality", () => {
  it("labels one clip at a time, skips past one, and comes back to a skipped clip at the end", async () => {
    const user = userEvent.setup();
    const { labelled } = serveQueue(["Alpha", "Bravo", "Charlie"]);
    const { container } = render(<FillerQuality />, { wrapper });

    await playing(container, "Alpha");
    expect(screen.getByText("3 clips left to label")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "Skip" }));
    await playing(container, "Bravo");

    await user.click(screen.getByRole("button", { name: "Fail" }));
    await playing(container, "Charlie");
    expect(screen.getByText("2 clips left to label")).toBeVisible();

    // Charlie is the last in the list: labelling it leaves the skipped Alpha, not an empty queue.
    await user.click(screen.getByRole("button", { name: "Pass" }));
    await playing(container, "Alpha");
    expect(labelled).toEqual(["Bravo", "Charlie"]);

    await user.click(screen.getByRole("button", { name: "Pass" }));
    expect(await screen.findByText("Every measured clip is labelled")).toBeVisible();
  });

  it("says a label wasn't saved and keeps the clip", async () => {
    const user = userEvent.setup();
    serveQueue(["Alpha"]);
    server.use(
      http.put("*/v1/filler/defects/:clipId/label", () =>
        HttpResponse.json(
          { title: "Not Found", detail: "The defect floor has not measured this clip yet", status: 404 },
          { status: 404 },
        ),
      ),
    );
    const { container } = render(<FillerQuality />, { wrapper });
    await user.click(await screen.findByRole("button", { name: "Fail" }));
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(
        "That label wasn't saved. The defect floor has not measured this clip yet",
      ),
    );
    await playing(container, "Alpha");
  });
});
