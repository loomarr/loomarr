import type { NativeEventRequest } from "@loomarr/player/native";
import { describe, expect, it, vi } from "vitest";

vi.mock("expo-video", () => ({
  createVideoPlayer: vi.fn(),
  VideoView: vi.fn(),
}));

const { createNativeEventStream, parseEventFrames } = await import("@loomarr/player/native");

class RequestStub implements NativeEventRequest {
  aborted = false;
  headers = new Map<string, string>();
  onerror: ((event: ProgressEvent<EventTarget>) => unknown) | null = null;
  onreadystatechange: ((event: Event) => unknown) | null = null;
  readyState = 0;
  responseText = "";
  status = 0;
  url = "";

  abort = () => {
    this.aborted = true;
  };
  open = (_method: string, url: string) => {
    this.url = url;
  };
  send = vi.fn();
  setRequestHeader = (name: string, value: string) => {
    this.headers.set(name, value);
  };
}

const timerHarness = () => {
  const callbacks: Array<() => void> = [];
  return {
    callbacks,
    clearTimer: vi.fn(),
    setTimer: (callback: () => void) => {
      callbacks.push(callback);
      return callbacks.length as unknown as ReturnType<typeof setTimeout>;
    },
  };
};

describe("native event stream", () => {
  it("parses named, multiline, and incomplete SSE frames", () => {
    expect(
      parseEventFrames('event: channel\ndata: {"id":\ndata: "seven"}\n\n: keepalive\n\npartial'),
    ).toEqual({
      frames: [{ data: '{"id":\n"seven"}', type: "channel" }],
      rest: "partial",
    });
  });

  it("authenticates, dispatches incremental channel frames, and closes cleanly", () => {
    const timers = timerHarness();
    const requests: RequestStub[] = [];
    const stream = createNativeEventStream("http://loomarr.test/v1/events", {
      clearTimer: timers.clearTimer,
      createRequest: () => {
        const request = new RequestStub();
        requests.push(request);
        return request;
      },
      headers: { Authorization: "Bearer device-token" },
      setTimer: timers.setTimer,
    });
    const frames = vi.fn();
    stream.addEventListener("channel", frames);

    timers.callbacks.shift()?.();
    const request = requests[0]!;
    expect(request.url).toBe("http://loomarr.test/v1/events");
    expect(request.headers.get("Authorization")).toBe("Bearer device-token");
    request.status = 200;
    request.readyState = 3;
    request.responseText = 'event: channel\ndata: {"channelId":"seven"}';
    request.onreadystatechange?.(new Event("readystatechange"));
    expect(frames).not.toHaveBeenCalled();
    request.responseText += "\n\n";
    request.onreadystatechange?.(new Event("readystatechange"));
    expect(frames).toHaveBeenCalledWith({ data: '{"channelId":"seven"}' });

    stream.close();
    expect(request.aborted).toBe(true);
  });

  it("fails closed on revoked credentials and reconnects other terminal failures", () => {
    const timers = timerHarness();
    const requests: RequestStub[] = [];
    const onUnauthorized = vi.fn();
    const stream = createNativeEventStream("http://loomarr.test/v1/events", {
      clearTimer: timers.clearTimer,
      createRequest: () => {
        const request = new RequestStub();
        requests.push(request);
        return request;
      },
      onUnauthorized,
      setTimer: timers.setTimer,
    });

    timers.callbacks.shift()?.();
    requests[0]!.status = 500;
    requests[0]!.readyState = 4;
    requests[0]!.onreadystatechange?.(new Event("readystatechange"));
    expect(timers.callbacks).toHaveLength(1);
    timers.callbacks.shift()?.();
    requests[1]!.status = 401;
    requests[1]!.readyState = 4;
    requests[1]!.onreadystatechange?.(new Event("readystatechange"));
    expect(onUnauthorized).toHaveBeenCalledOnce();
    expect(requests[1]!.aborted).toBe(true);
    expect(timers.callbacks).toHaveLength(0);

    stream.close();
  });
});
