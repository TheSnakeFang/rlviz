interface AdapterExports extends WebAssembly.Exports {
  memory: WebAssembly.Memory;
  rlviz_alloc(size: number): number;
  rlviz_adapt(pointer: number, length: number): number;
  rlviz_result_len(): number;
  rlviz_free(pointer: number, length: number): void;
}

type AdapterRequest = { module: WebAssembly.Module; sourceBytes: Uint8Array; maxOutputBytes: number };

self.onmessage = async (event: MessageEvent<AdapterRequest>) => {
  let inputPointer = 0, outputPointer = 0, outputLength = 0;
  let exports: AdapterExports | undefined;
  try {
    const instance = await WebAssembly.instantiate(event.data.module, {});
    exports = instance.exports as AdapterExports;
    inputPointer = exports.rlviz_alloc(event.data.sourceBytes.byteLength);
    if (inputPointer < 0 || inputPointer + event.data.sourceBytes.byteLength > exports.memory.buffer.byteLength) throw new Error("Adapter returned an invalid input buffer");
    new Uint8Array(exports.memory.buffer, inputPointer, event.data.sourceBytes.byteLength).set(event.data.sourceBytes);
    outputPointer = exports.rlviz_adapt(inputPointer, event.data.sourceBytes.byteLength);
    outputLength = exports.rlviz_result_len();
    if (outputPointer <= 0 || outputLength <= 0 || outputLength > event.data.maxOutputBytes || outputPointer + outputLength > exports.memory.buffer.byteLength) throw new Error("Adapter returned an invalid or oversized result buffer");
    const output = new Uint8Array(exports.memory.buffer, outputPointer, outputLength).slice();
    self.postMessage({ ok: true, output }, { transfer: [output.buffer] });
  } catch (error) {
    self.postMessage({ ok: false, error: error instanceof Error ? error.message : "Adapter failed" });
  } finally {
    try { if (exports && inputPointer >= 0) exports.rlviz_free(inputPointer, event.data.sourceBytes.byteLength); } catch { /* worker is disposable */ }
    try { if (exports && outputPointer > 0 && outputLength > 0) exports.rlviz_free(outputPointer, outputLength); } catch { /* worker is disposable */ }
  }
};
