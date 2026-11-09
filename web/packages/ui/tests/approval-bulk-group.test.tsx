import { LoomarrProvider } from "@loomarr/design-system";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { ApprovalBulkGroup } from "../index";

// #2738: a request's own Approve is the amber primary action; the group's bulk Approve acts on a
// selection and is outlined, so two filled buttons never compete on one screen.
describe("ApprovalBulkGroup bulk Approve", () => {
  const markup = renderToStaticMarkup(
    <LoomarrProvider theme="dark">
      <ApprovalBulkGroup
        onApprove={vi.fn()}
        onSelectedChange={vi.fn()}
        rows={[{ content: <span>Noir</span>, id: "a", label: "Noir" }]}
        selected={new Set(["a"])}
        title="Channel requests"
      />
    </LoomarrProvider>,
  );

  it("is not an amber fill", () => {
    expect(markup).toContain("Approve 1");
    expect(markup).not.toMatch(/background-color:rgba\(255,176,32,1\.00\)/);
  });
});
