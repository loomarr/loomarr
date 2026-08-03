import type { PoolDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PoolHealth } from "./pool-health";

const pool = (over: Partial<PoolDTO> = {}): PoolDTO => ({
  clips: 120,
  commercials: 90,
  eligible: 61,
  untagged: 0,
  channels: [],
  ...over,
});

describe("PoolHealth", () => {
  // ⚠ The number that surprises people. A catalog of compilations reads as healthy by clip count
  // and can fill nothing, so "fits a break" is shown against the commercial count rather than
  // left for the operator to work out.
  it("shows how many clips can actually go in a break, against the total", () => {
    render(<PoolHealth pool={pool()} />);

    expect(screen.getByText("61")).toBeInTheDocument();
    expect(screen.getByText("of 90 commercials")).toBeInTheDocument();
  });

  it("counts untagged clips as work rather than hiding them", () => {
    render(<PoolHealth pool={pool({ untagged: 14 })} />);

    expect(screen.getByText("14 clips still need tagging")).toBeInTheDocument();
  });

  it("says so plainly when nothing needs tagging", () => {
    render(<PoolHealth pool={pool({ untagged: 0 })} />);

    expect(screen.getByText("all tagged")).toBeInTheDocument();
  });

  // The diagnosis. Naming the channel is the difference between "something is thin" and
  // something an operator can act on — and the server already ordered the list worst-first, so
  // the head is read positionally rather than re-sorted here.
  it("names the weakest channel and what its breaks fall back to", () => {
    render(
      <PoolHealth
        pool={pool({
          channels: [
            { channelId: "ch-3", name: "Newsreel", number: 3, level: "bumper_card", total: 0 },
            { channelId: "ch-42", name: "Cartoons", number: 42, level: "exact", total: 44 },
          ],
        })}
      />,
    );

    expect(screen.getByText("Newsreel")).toBeInTheDocument();
    expect(screen.getByText("breaks fall back to no commercials")).toBeInTheDocument();
    expect(screen.getByText("1/2")).toBeInTheDocument();
    expect(screen.getByText("1 channel has nothing to play")).toBeInTheDocument();
  });

  // ⚠ A callout that is always on teaches people to stop reading it, so a healthy install gets
  // no "weakest" stat at all.
  it("omits the weakest callout when every channel matches exactly", () => {
    render(
      <PoolHealth
        pool={pool({
          channels: [{ channelId: "ch-42", name: "Cartoons", number: 42, level: "exact", total: 44 }],
        })}
      />,
    );

    expect(screen.queryByText("Weakest")).not.toBeInTheDocument();
    expect(screen.getByText("every channel matches exactly")).toBeInTheDocument();
  });

  // ⚠ **The note must describe the same population the fraction counts.** It used to say "every
  // channel has commercials" whenever nothing had fallen to the bumper card — true, but an answer
  // to a different question than the headline, which counts EXACT matches. An install where every
  // channel sat on the middle rung rendered "0/2 · every channel has commercials": two numbers
  // that read as a contradiction and were both correct. Seen live 2026-08-03.
  it("names the middle rung rather than claiming coverage the fraction denies", () => {
    render(
      <PoolHealth
        pool={pool({
          channels: [
            { channelId: "ch-1", name: "Cartoons", number: 1, level: "audience", total: 12 },
            { channelId: "ch-2", name: "Sci-Fi", number: 2, level: "widened", total: 9 },
          ],
        })}
      />,
    );

    expect(screen.getByText("0/2")).toBeInTheDocument();
    expect(screen.getByText("2 channels fall back to a looser match")).toBeInTheDocument();
    // The old copy claimed these channels were fine. They play commercials, but not matched ones.
    expect(screen.queryByText("every channel has commercials")).not.toBeInTheDocument();
  });

  // A fresh install has no channels yet — that is the state the pull button exists for, and the
  // strip must not claim "0/0 covered" at it.
  it("drops the channel stats entirely when there are no channels", () => {
    render(<PoolHealth pool={pool()} />);

    expect(screen.queryByText("Channels covered")).not.toBeInTheDocument();
  });

  // huma types every Go slice as nullable, so the generated DTO says `channels: ... | null`
  // even though the handler always sends []. Rendering must survive the null.
  it("survives a null channels list", () => {
    render(<PoolHealth pool={{ ...pool(), channels: null as unknown as PoolDTO["channels"] }} />);

    expect(screen.getByText("61")).toBeInTheDocument();
  });

  it("offers the pull only to a caller that passed a handler", async () => {
    const onProposePull = vi.fn();
    const { rerender } = render(<PoolHealth pool={pool()} onProposePull={onProposePull} />);

    await userEvent.click(screen.getByRole("button", { name: "Propose a pull" }));
    expect(onProposePull).toHaveBeenCalledOnce();

    // A member reads catalog health but cannot start an acquisition.
    rerender(<PoolHealth pool={pool()} />);
    expect(screen.queryByRole("button", { name: "Propose a pull" })).not.toBeInTheDocument();
  });

  it("says a pull is being planned rather than looking inert", () => {
    render(<PoolHealth pool={pool()} onProposePull={() => {}} proposing />);

    expect(screen.getByRole("button", { name: "Planning…" })).toBeDisabled();
  });
});
