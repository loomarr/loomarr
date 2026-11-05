import { EVENT_TYPES, type EventType } from "@loomarr/core/events";
import { describe, expect, it } from "vitest";
import { FakeEventSource } from "./fake-event-source";

// The shared EventSource double speaks only the frame vocabulary the spec declares. A test that
// emits, or a component that listens for, a frame the server never sends is a bug the fake must
// surface rather than quietly deliver.
describe("FakeEventSource", () => {
  it("is the global every test sees", () => {
    expect(globalThis.EventSource).toBe(FakeEventSource);
  });

  it("delivers every spec frame type to its listeners as JSON", () => {
    const stream = new FakeEventSource("/v1/events");
    const seen: [string, unknown][] = [];
    for (const type of EVENT_TYPES) {
      stream.addEventListener(type, (event) => seen.push([type, JSON.parse(event.data)]));
    }
    for (const type of EVENT_TYPES) stream.emit(type, { frame: type });
    expect(seen).toEqual(EVENT_TYPES.map((type) => [type, { frame: type }]));
  });

  it("refuses to emit a frame type the spec does not declare", () => {
    const stream = new FakeEventSource("/v1/events");
    expect(() => stream.emit("not_a_frame" as EventType, {})).toThrow(/not_a_frame/);
  });

  it("refuses a listener for a frame type the spec does not declare", () => {
    const stream = new FakeEventSource("/v1/events");
    expect(() => stream.addEventListener("not_a_frame", () => {})).toThrow(/not_a_frame/);
    expect(() => stream.addEventListener("error", () => {})).not.toThrow();
  });

  it("broadcasts to every open stream and none that closed", () => {
    const open = new FakeEventSource("/v1/events");
    const closed = new FakeEventSource("/v1/events");
    const got: string[] = [];
    open.addEventListener("job", () => got.push("open"));
    closed.addEventListener("job", () => got.push("closed"));
    closed.close();

    FakeEventSource.broadcast("job", { jobId: "j" });
    expect(got).toEqual(["open"]);
  });
});
