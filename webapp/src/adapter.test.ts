import { afterEach, describe, expect, it, vi } from "vitest";
import { runAdapter } from "./adapter";

vi.mock("./wasm", () => ({ decodeAdapter: vi.fn(), limits: async () => ({ maxRecommendedBytes: 32 * 1024 * 1024 }) }));

const completeExports: WebAssembly.ModuleExportDescriptor[] = [
  { name: "memory", kind: "memory" },
  { name: "rlviz_alloc", kind: "function" },
  { name: "rlviz_adapt", kind: "function" },
  { name: "rlviz_result_len", kind: "function" },
  { name: "rlviz_free", kind: "function" },
];

describe("browser adapter isolation", () => {
  afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

  it("checks required exports before constructing a worker", async () => {
    const module = {} as WebAssembly.Module;
    vi.spyOn(WebAssembly, "compile").mockResolvedValue(module);
    vi.spyOn(WebAssembly.Module, "imports").mockReturnValue([]);
    vi.spyOn(WebAssembly.Module, "exports").mockReturnValue([]);
    const worker = vi.fn(); vi.stubGlobal("Worker", worker);
    await expect(runAdapter(new Uint8Array(), new Uint8Array(), "trace")).rejects.toThrow("missing memory export");
    expect(worker).not.toHaveBeenCalled();
  });

  it("terminates an adapter worker at the hard timeout", async () => {
    vi.useFakeTimers();
    const module = {} as WebAssembly.Module;
    vi.spyOn(WebAssembly, "compile").mockResolvedValue(module);
    vi.spyOn(WebAssembly.Module, "imports").mockReturnValue([]);
    vi.spyOn(WebAssembly.Module, "exports").mockReturnValue(completeExports);
    const terminate = vi.fn();
    class HangingWorker {
      onmessage: ((event: MessageEvent) => void) | null = null;
      onerror: ((event: ErrorEvent) => void) | null = null;
      terminate = terminate;
      postMessage() { /* intentionally never responds */ }
    }
    vi.stubGlobal("Worker", HangingWorker);
    const result = runAdapter(new Uint8Array(), new Uint8Array([1]), "trace");
    const rejected = expect(result).rejects.toThrow("5-second execution timeout");
    await vi.advanceTimersByTimeAsync(5_000);
    await rejected;
    expect(terminate).toHaveBeenCalled();
  });
});
