import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Card, CardContent, CardHeader, CardTitle } from "./card";

describe("Card", () => {
  it("composes header, title and content", () => {
    render(
      <Card>
        <CardHeader>
          <CardTitle>Channel 42</CardTitle>
        </CardHeader>
        <CardContent>90s action</CardContent>
      </Card>,
    );
    expect(screen.getByText("Channel 42")).toBeInTheDocument();
    expect(screen.getByText("90s action")).toBeInTheDocument();
  });
});
