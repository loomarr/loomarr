import { people } from "@loomarr/fixtures";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PeopleRoster } from "./people-roster";

describe("PeopleRoster", () => {
  it("keeps identity and account state scannable and selects a person", async () => {
    const onSelect = vi.fn();
    render(
      <PeopleRoster
        users={[people.localAdmin, people.importedMember]}
        selfId={people.localAdmin.id}
        onSelect={onSelect}
      />,
    );
    expect(screen.getByText("Local account")).toBeInTheDocument();
    expect(screen.getByText(/Media-server account/)).toBeInTheDocument();
    expect(screen.getByText("You")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Manage Grace Hopper" }));
    expect(onSelect).toHaveBeenCalledWith(people.importedMember);
  });

  it("searches and filters the loaded roster with an explicit no-results state", async () => {
    render(<PeopleRoster users={Object.values(people)} onSelect={() => {}} />);
    await userEvent.type(screen.getByLabelText("Search people"), "Grace");
    expect(screen.getByRole("button", { name: "Manage Grace Hopper" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Manage Ada Lovelace" })).not.toBeInTheDocument();

    await userEvent.clear(screen.getByLabelText("Search people"));
    await userEvent.type(screen.getByLabelText("Search people"), "Nobody");
    expect(screen.getByText("No matching people")).toBeInTheDocument();
  });

  it("points an empty roster to the page-header actions", () => {
    render(<PeopleRoster users={[]} onSelect={() => {}} />);
    expect(screen.getByText(/use the actions above/i)).toBeInTheDocument();
    expect(screen.queryByText(/create a local account below/i)).not.toBeInTheDocument();
  });
});
