import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TMDB_NOTICE, TmdbAttribution } from "./tmdb-attribution";

describe("TmdbAttribution", () => {
  // ⚠ The wording is asserted LITERALLY, not via the exported constant alone, and that is the
  // point of this test. TMDB's terms require this exact sentence; a refactor that "tidied" the
  // copy would keep every constant-based assertion green while putting the instance out of
  // compliance. Spelling it out here means the test has to be edited deliberately.
  it("renders TMDB's required notice verbatim", () => {
    render(<TmdbAttribution />);

    expect(
      screen.getByText(
        "This product uses TMDB and the TMDB APIs but is not endorsed, certified, or otherwise approved by TMDB.",
      ),
    ).toBeInTheDocument();
    expect(TMDB_NOTICE).toBe(
      "This product uses TMDB and the TMDB APIs but is not endorsed, certified, or otherwise approved by TMDB.",
    );
  });

  // The notice is the half that is always required; the logo is supplied by the build. Rendering
  // without one must still produce the notice rather than nothing.
  it("renders the notice with no logo supplied", () => {
    const { container } = render(<TmdbAttribution />);

    expect(screen.getByText(TMDB_NOTICE)).toBeInTheDocument();
    expect(container.querySelector("img")).toBeNull();
  });

  it("shows a supplied logo alongside the notice", () => {
    render(<TmdbAttribution logo={<img src="/tmdb.svg" alt="TMDB" />} />);

    expect(screen.getByAltText("TMDB")).toBeInTheDocument();
    expect(screen.getByText(TMDB_NOTICE)).toBeInTheDocument();
  });

  // A labelled landmark, so the notice is announced as a distinct statement about a third party
  // rather than running on from the settings above it.
  it("is a labelled landmark", () => {
    render(<TmdbAttribution />);

    expect(screen.getByRole("complementary", { name: "TMDB attribution" })).toBeInTheDocument();
  });
});
