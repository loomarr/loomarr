import type { IncomingRejectDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { RejectedSection } from "./rejected-section";

const reject = (over: Partial<IncomingRejectDTO> = {}): IncomingRejectDTO => ({
  hash: "hash-mystery",
  name: "clip_0042.mp4",
  reason: "unidentified",
  detail: "no era, audience, tag, brand, transcript or on-screen text",
  restorable: true,
  stage: "score",
  at: "2026-08-08T09:00:00Z",
  ...over,
});

describe("RejectedSection", () => {
  // The server sends a stable CODE and the frontend owns the wording (§11's refusal-code
  // precedent): a server sentence cannot be translated and freezes copy into an API response.
  it("renders the refusal in the operator's words, with the measured detail beside it", () => {
    render(<RejectedSection rows={[reject()]} />);

    expect(screen.getByText("nothing in it said what it was")).toBeInTheDocument();
    expect(screen.getByText(/no era, audience, tag/)).toBeInTheDocument();
  });

  it("offers the override for a soft refusal", async () => {
    const onRestore = vi.fn();
    const clip = reject();
    render(<RejectedSection rows={[clip]} onRestore={onRestore} />);

    await userEvent.click(screen.getByRole("button", { name: /use it anyway/i }));

    expect(onRestore).toHaveBeenCalledWith(clip);
  });

  // ⚠ THE asymmetry, and it is a kindness rather than a restriction: restoring a clip with no
  // audio track puts silence in a break. The server owns which refusals are soft (`Soft()`,
  // mirrored onto `restorable`); deriving that list a second time here is the drift class this
  // codebase keeps finding.
  it("offers NO override for a hard refusal, even when a handler is supplied", () => {
    render(
      <RejectedSection rows={[reject({ reason: "no_audio", restorable: false })]} onRestore={vi.fn()} />,
    );

    expect(screen.getByText("it has no sound")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /use it anyway/i })).not.toBeInTheDocument();
  });

  it("explains content-silent and black-stream refusals separately from missing streams", () => {
    render(
      <RejectedSection
        rows={[
          reject({ hash: "black", reason: "black_content", restorable: false }),
          reject({ hash: "silent", reason: "silent_content", restorable: false }),
        ]}
      />,
    );

    expect(screen.getByText("the picture is almost entirely black")).toBeInTheDocument();
    expect(screen.getByText("the audio track is almost entirely silent")).toBeInTheDocument();
  });

  // A code this build has no copy for comes from a NEWER backend. The raw code tells an operator —
  // and a bug report — something; "Unknown reason" tells nobody anything.
  it("falls back to the server's own code rather than inventing a placeholder", () => {
    render(<RejectedSection rows={[reject({ reason: "fingerprint_clash" })]} />);

    expect(screen.getByText("fingerprint_clash")).toBeInTheDocument();
  });

  // One row writing disables only that row — the pattern the ask queue already follows, because a
  // list that greys out entirely while one write lands reads as the page having frozen.
  it("disables only the row being written", () => {
    render(
      <RejectedSection
        rows={[reject(), reject({ hash: "other", name: "another.mp4" })]}
        onRestore={vi.fn()}
        busyHash="hash-mystery"
      />,
    );

    const [first, second] = screen.getAllByRole("button", { name: /use it anyway/i });
    expect(first).toBeDisabled();
    expect(second).toBeEnabled();
  });

  it("falls back to the hash when a refused clip has no name", () => {
    render(<RejectedSection rows={[reject({ name: "" })]} />);

    expect(screen.getByText("hash-mystery")).toBeInTheDocument();
  });
});
