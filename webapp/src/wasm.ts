import type { BrowserCollection } from "./provider";

type WasmResult<T> = { ok: true; value: T } | { ok: false; code?: string; error: string };
export type BrowserCollectionHandle = { collection_id: string; collection: BrowserCollection };
export type WasmLimits = { maxRecommendedBytes: number };

declare global {
  class Go {
    importObject: WebAssembly.Imports;
    run(instance: WebAssembly.Instance): Promise<void>;
  }
  var rlvizWasmReady: Promise<void> | undefined;
  var rlvizResolveWasmReady: (() => void) | undefined;
  var rlvizWasmLimits: WasmLimits | undefined;
  var rlvizParse: ((bytes: Uint8Array, name: string) => string) | undefined;
  var rlvizDecodeAdapter: ((bytes: Uint8Array, name: string, sourceSize: number) => string) | undefined;
  var rlvizAnalyze: ((collectionId: string, trajectoryId: string) => string) | undefined;
  var rlvizCompare: ((collectionId: string, left: string, right: string) => string) | undefined;
}

const readyTimeoutMs = 15_000;
let ready: Promise<void> | undefined;

function readyHandshake(): Promise<void> {
  let resolveReady!: () => void;
  globalThis.rlvizWasmReady = new Promise<void>((resolve) => { resolveReady = resolve; });
  globalThis.rlvizResolveWasmReady = resolveReady;
  return globalThis.rlvizWasmReady;
}

async function loadRuntime(): Promise<void> {
  if (typeof Go !== "undefined") return;
  await new Promise<void>((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>('script[data-rlviz-wasm-runtime="true"]');
    if (existing) { existing.addEventListener("load", () => resolve(), { once: true }); existing.addEventListener("error", () => reject(new Error("Could not load the local Go WebAssembly runtime")), { once: true }); return; }
    const script = document.createElement("script");
    script.src = "/wasm_exec.js";
    script.dataset.rlvizWasmRuntime = "true";
    script.onload = () => resolve();
    script.onerror = () => { script.remove(); reject(new Error("Could not load the local Go WebAssembly runtime")); };
    document.head.append(script);
  });
}

export function loadCore(): Promise<void> {
  if (ready) return ready;
  const attempt = (async () => {
    await loadRuntime();
    const handshake = readyHandshake();
    const go = new Go();
    const response = await fetch("/rlviz.wasm");
    if (!response.ok) throw new Error(`Could not load viewer core (${response.status})`);
    const module = await WebAssembly.compile(await response.arrayBuffer());
    const instance = await WebAssembly.instantiate(module, go.importObject);
    void go.run(instance);
    let timeout: ReturnType<typeof setTimeout> | undefined;
    try {
      await Promise.race([
        handshake,
        new Promise<never>((_, reject) => { timeout = setTimeout(() => reject(new Error("Viewer core did not initialize within 15 seconds")), readyTimeoutMs); }),
      ]);
    } finally { if (timeout !== undefined) clearTimeout(timeout); }
    if (!globalThis.rlvizWasmLimits || !globalThis.rlvizParse || !globalThis.rlvizAnalyze || !globalThis.rlvizCompare) throw new Error("Viewer core initialized without its required exports");
  })();
  ready = attempt.catch((error) => { ready = undefined; throw error; });
  return ready;
}

function unwrap<T>(encoded: string): T {
  const result = JSON.parse(encoded) as WasmResult<T>;
  if (!result.ok) throw new Error(result.code ? `${result.code}: ${result.error}` : result.error);
  return result.value;
}

export async function limits(): Promise<WasmLimits> {
  await loadCore();
  return globalThis.rlvizWasmLimits!;
}

export async function parseTrace(bytes: Uint8Array, name: string): Promise<BrowserCollectionHandle> {
  await loadCore();
  return unwrap(globalThis.rlvizParse!(bytes, name));
}

export async function decodeAdapter(bytes: Uint8Array, name: string, sourceSize: number): Promise<BrowserCollectionHandle> {
  await loadCore();
  return unwrap(globalThis.rlvizDecodeAdapter!(bytes, name, sourceSize));
}

export async function analyze(collectionId: string, trajectoryId: string): Promise<unknown> {
  await loadCore();
  return unwrap(globalThis.rlvizAnalyze!(collectionId, trajectoryId));
}

export async function compare(collectionId: string, left: string, right: string): Promise<unknown> {
  await loadCore();
  return unwrap(globalThis.rlvizCompare!(collectionId, left, right));
}
