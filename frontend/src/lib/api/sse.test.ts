import { describe, expect, it } from "vitest";
import { readSSEStream, type SSEEventHandler } from "./sse";

function responseFromChunks(chunks: string[]): Response {
  const encoder = new TextEncoder();
  return new Response(
    new ReadableStream({
      start(controller) {
        for (const chunk of chunks) {
          controller.enqueue(encoder.encode(chunk));
        }
        controller.close();
      },
    }),
    {
      status: 200,
      headers: { "Content-Type": "text/event-stream" },
    },
  );
}

describe("readSSEStream", () => {
  it("parses events across chunk boundaries and ignores comments", async () => {
    const received: Array<[string, Record<string, unknown>]> = [];
    const onEvent: SSEEventHandler = (eventName, payload) => {
      received.push([eventName, payload]);
    };

    await readSSEStream(
      responseFromChunks([
        ": heartbeat\n\nevent: rea",
        'dy\ndata: {"ok":true}\n\nevent: feeds_changed\ndata: {}',
        "\n\n",
      ]),
      onEvent,
    );

    expect(received).toEqual([
      ["ready", { ok: true }],
      ["feeds_changed", {}],
    ]);
  });

  it("supports CRLF and multi-line JSON data", async () => {
    const received: Array<[string, Record<string, unknown>]> = [];
    await readSSEStream(
      responseFromChunks([
        'event: message\r\ndata: {"first":\r\ndata: 1}\r\n\r\n',
      ]),
      (eventName, payload) => received.push([eventName, payload]),
    );

    expect(received).toEqual([["message", { first: 1 }]]);
  });

  it("throws on an error event", async () => {
    await expect(
      readSSEStream(
        responseFromChunks(['event: error\ndata: {"error":"failed"}\n\n']),
        () => {},
      ),
    ).rejects.toThrow("failed");
  });
});
