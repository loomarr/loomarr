import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RouterHarness } from "@/test/story-utils";
import { SettingsContextNav } from "./settings-context-nav";

describe("SettingsContextNav", () => {
  it("keeps leaf pages anchored to the settings task home", async () => {
    render(<RouterHarness initialPath="/settings" content={<SettingsContextNav section="This server" />} />);

    expect(await screen.findByRole("navigation", { name: "Settings location" })).toHaveTextContent(
      /Settings\s*\/\s*This server/,
    );
    expect(screen.getByRole("link", { name: "Settings" })).toHaveAttribute("href", "/settings");
    expect(screen.getByRole("link", { name: "Find another setting" })).toHaveAttribute("href", "/settings");
  });
});
