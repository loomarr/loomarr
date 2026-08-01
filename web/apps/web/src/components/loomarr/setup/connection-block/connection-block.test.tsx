import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { ConnectionBlock } from "./connection-block";

// The `.reveal` grid trick keeps the body in the DOM even when collapsed (clipped, not
// unmounted), so open/closed is asserted via aria-expanded + the reveal's data-open — the
// same convention CollapsibleSection uses. A failing block's "Fix" is a routed Link, so the
// tree renders inside RouterHarness (which mounts async — hence findBy* on first query).
const withRouter = (ui: ReactElement) => render(<RouterHarness content={ui} initialPath="/settings" />);

const bodyText = "the connection fields";

describe("ConnectionBlock", () => {
  it("is controlled: header toggles via onToggle, open drives the reveal", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    const { rerender } = withRouter(
      <ConnectionBlock title="Media server" open={false} onToggle={onToggle}>
        <p>{bodyText}</p>
      </ConnectionBlock>,
    );

    const header = await screen.findByRole("button", { name: /media server/i });
    expect(header).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByText(bodyText).closest(".reveal")).toHaveAttribute("data-open", "false");

    await user.click(header);
    expect(onToggle).toHaveBeenCalledOnce();

    // Controlled — clicking doesn't open it; the parent flipping `open` does.
    rerender(
      <RouterHarness
        content={
          <ConnectionBlock title="Media server" open onToggle={onToggle}>
            <p>{bodyText}</p>
          </ConnectionBlock>
        }
        initialPath="/settings"
      />,
    );
    expect(await screen.findByRole("button", { name: /media server/i })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expect(screen.getByText(bodyText).closest(".reveal")).toHaveAttribute("data-open", "true");
  });

  it("shows a failing verdict inline with a Fix link into the Help center", async () => {
    withRouter(
      <ConnectionBlock
        title="Media server"
        open
        onToggle={() => {}}
        verdict={{ ok: false, hint: "status 401" }}
        docHref="troubleshooting#media-server"
      >
        <p>{bodyText}</p>
      </ConnectionBlock>,
    );

    expect(await screen.findByRole("status")).toHaveTextContent("status 401");
    const fix = screen.getByRole("link", { name: /fix/i });
    // parseDocHref turns "troubleshooting#media-server" into /help?page=…&section=… — the
    // href must be an absolute app route, never a raw fragment that 404s under /settings.
    expect(fix.getAttribute("href")).toMatch(/^\/help\?/);
  });

  it("shows a passing verdict as 'Connection OK' and no Fix link", async () => {
    withRouter(
      <ConnectionBlock title="TMDB" open onToggle={() => {}} verdict={{ ok: true }}>
        <p>{bodyText}</p>
      </ConnectionBlock>,
    );

    expect(await screen.findByRole("status")).toHaveTextContent(/connection ok/i);
    expect(screen.queryByRole("link", { name: /fix/i })).toBeNull();
  });

  it("marks an optional connection in the header", async () => {
    withRouter(
      <ConnectionBlock title="Requester (Seerr)" optional open onToggle={() => {}}>
        <p>{bodyText}</p>
      </ConnectionBlock>,
    );
    expect(await screen.findByText("optional")).toBeInTheDocument();
  });

  // The v2 mock's `bk.summary`. It exists so a COLLAPSED block still says where it stands:
  // the status dot cannot separate "failed" from "never tested" for anyone who does not read
  // colour, and the full hint only renders in the open body.
  it.each([
    ["not tested yet", {}],
    ["OK", { verdict: { ok: true } }],
    ["needs attention", { verdict: { ok: false } }],
    // testing wins over the standing verdict — a probe in flight is the newer truth.
    ["testing…", { verdict: { ok: true }, testing: true }],
  ])("summarises the connection as %s in the header", async (expected, props) => {
    withRouter(
      <ConnectionBlock title="Tunarr" open={false} onToggle={() => {}} {...props}>
        <p>{bodyText}</p>
      </ConnectionBlock>,
    );
    expect(await screen.findByRole("button", { name: /tunarr/i })).toHaveTextContent(expected);
  });

  // The summary is DERIVED, never sent: `SetupCheck` is {name, ok, hint, docHref} and gains
  // no `summary` field. A hint is diagnosis and belongs in the body; the header line is the
  // one-word standing. Asserting they differ is what stops the hint being hoisted up here.
  it("keeps the header summary distinct from the body's hint", async () => {
    withRouter(
      <ConnectionBlock
        title="Tunarr"
        open
        onToggle={() => {}}
        verdict={{ ok: false, hint: "dial tcp 127.0.0.1:8000: connection refused" }}
      >
        <p>{bodyText}</p>
      </ConnectionBlock>,
    );

    expect(await screen.findByRole("button", { name: /tunarr/i })).toHaveTextContent("needs attention");
    expect(screen.getByRole("status")).toHaveTextContent(/connection refused/i);
  });
});
