import { afterEach, describe, expect, it, vi } from "vitest";

describe("Go WASM readiness", () => {
  afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

  it("uses the Go-ready handshake and retries after a rejected initialization", async () => {
    class FakeGo {
      importObject = {};
      async run() {
        globalThis.rlvizWasmLimits = { maxRecommendedBytes: 32 << 20 };
        globalThis.rlvizParse = () => `{"ok":true,"value":{}}`;
        globalThis.rlvizAnalyze = () => `{"ok":true,"value":{}}`;
        globalThis.rlvizCompare = () => `{"ok":true,"value":{}}`;
        globalThis.rlvizResolveWasmReady?.();
      }
    }
    vi.stubGlobal("Go", FakeGo);
    const fetch = vi.fn()
      .mockResolvedValueOnce(new Response("missing", { status: 503 }))
      .mockResolvedValueOnce(new Response(new Uint8Array([0]), { status: 200 }));
    vi.stubGlobal("fetch", fetch);
    vi.spyOn(WebAssembly, "compile").mockResolvedValue({} as WebAssembly.Module);
    vi.spyOn(WebAssembly, "instantiate").mockResolvedValue({} as WebAssembly.Instance);
    const { loadCore } = await import("./wasm");
    await expect(loadCore()).rejects.toThrow("503");
    await expect(loadCore()).resolves.toBeUndefined();
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(globalThis.rlvizWasmReady).toBeInstanceOf(Promise);
  });
});
