import { decodeAdapter, limits } from "./wasm";
import type { BrowserCollectionHandle } from "./wasm";

const adapterTimeoutMs = 5_000;
const requiredExports = new Map<string, WebAssembly.ImportExportKind>([
  ["memory", "memory"],
  ["rlviz_alloc", "function"],
  ["rlviz_adapt", "function"],
  ["rlviz_result_len", "function"],
  ["rlviz_free", "function"],
]);

type WorkerResponse = { ok: true; output: Uint8Array } | { ok: false; error: string };

export async function adapterIdentity(bytes: Uint8Array): Promise<{ digest: string; size: number }> {
  const hash = await crypto.subtle.digest("SHA-256", bytes);
  return { digest: [...new Uint8Array(hash)].map((value) => value.toString(16).padStart(2, "0")).join(""), size: bytes.byteLength };
}

function validateModule(module: WebAssembly.Module): void {
  const imports = WebAssembly.Module.imports(module);
  if (imports.length) throw new Error(`Adapter must be import-free; found ${imports[0].module}.${imports[0].name}`);
  const exports = new Map(WebAssembly.Module.exports(module).map((item) => [item.name, item.kind]));
  for (const [name, kind] of requiredExports) {
    if (exports.get(name) !== kind) throw new Error(`Adapter is missing ${kind} export ${name}`);
  }
}

/** Execute a confirmed adapter in a disposable worker, then validate its output in the Go core. */
export async function runAdapter(moduleBytes: Uint8Array, sourceBytes: Uint8Array, sourceName: string): Promise<BrowserCollectionHandle> {
  // Compilation validates the binary without running its start section. Export
  // and import checks therefore happen before any adapter code executes.
  const module = await WebAssembly.compile(moduleBytes);
  validateModule(module);
  const { maxRecommendedBytes } = await limits();
  const worker = new Worker(new URL("./adapter-worker.ts", import.meta.url), { type: "module", name: "rlviz-adapter" });
  let timeout: ReturnType<typeof setTimeout> | undefined;
  try {
    const output = await new Promise<Uint8Array>((resolve, reject) => {
      timeout = setTimeout(() => {
        worker.terminate();
        reject(new Error(`Adapter exceeded the ${adapterTimeoutMs / 1000}-second execution timeout`));
      }, adapterTimeoutMs);
      worker.onmessage = (event: MessageEvent<WorkerResponse>) => event.data.ok ? resolve(event.data.output) : reject(new Error(event.data.error));
      worker.onerror = (event) => reject(new Error(event.message || "Adapter worker failed"));
      const sourceCopy = sourceBytes.slice();
      worker.postMessage({ module, sourceBytes: sourceCopy, maxOutputBytes: maxRecommendedBytes }, [sourceCopy.buffer]);
    });
    return await decodeAdapter(output, sourceName, sourceBytes.byteLength);
  } finally {
    if (timeout !== undefined) clearTimeout(timeout);
    worker.terminate();
  }
}
