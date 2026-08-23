import { LoomarrProvider } from "@loomarr/design-system";
import { ClientPlatformProof } from "@loomarr/ui";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

describe("shared client browser adapter", () => {
  it("renders the universal proof through the web React runtime", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ClientPlatformProof />
      </LoomarrProvider>,
    );

    expect(markup).toContain("Loomarr");
    expect(markup).toContain("One product language");
  });
});
