import { useCallback, useRef, useState } from "react";
import { App as Viewer } from "../../web/src/App";
import type { ViewerProvider } from "../../web/src/provider";
import codingExample from "../../examples/gallery/coding-agent-bugfix.ndjson?raw";
import researchExample from "../../examples/gallery/web-research-agent.ndjson?raw";
import cohortExample from "../../examples/gallery/checkout-cohort.ndjson?raw";
import { adapterIdentity, runAdapter } from "./adapter";
import { createInMemoryProvider } from "./provider";
import { limits, parseTrace } from "./wasm";

const encoder = new TextEncoder();
const examples = [
  ["300-event coding trace", "coding-agent-bugfix.ndjson", codingExample],
  ["web research trace", "web-research-agent.ndjson", researchExample],
  ["checkout cohort", "checkout-cohort.ndjson", cohortExample],
] as const;

const adapterPrompt = `Write a browser adapter for RLViz for the attached trace format.

Read https://rlviz.dev/adapter-authoring.md#browser-adapters and the canonical schema at https://rlviz.dev/data-model.html plus https://github.com/TheSnakeFang/rlviz/tree/main/schemas/v1alpha1.

Build an import-free WebAssembly module that exports:
- memory
- rlviz_alloc(size: i32) -> i32
- rlviz_adapt(input_ptr: i32, input_len: i32) -> i32
- rlviz_result_len() -> i32
- rlviz_free(ptr: i32, len: i32)

rlviz_adapt must read the raw source bytes and return a pointer to canonical NDJSON bytes. Emit parents before children and end with one complete record. Make no network requests, execute no trace content, and produce deterministic stable IDs. Include a small synthetic fixture and a test that validates the output.

Go build: tinygo build -target wasm -o adapter.wasm .
Rust build: cargo build --release --target wasm32-unknown-unknown`;

interface PendingAdapter {
  bytes: Uint8Array;
  name: string;
  digest: string;
  size: number;
}

export function BrowserApp() {
  const [provider, setProvider] = useState<ViewerProvider>();
  const [source, setSource] = useState<{ bytes: Uint8Array; name: string }>();
  const [status, setStatus] = useState("Ready for a canonical, Inspect AI, or Verifiers trace.");
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [help, setHelp] = useState(false);
  const [pendingAdapter, setPendingAdapter] = useState<PendingAdapter>();
  const traceInput = useRef<HTMLInputElement>(null);
  const adapterInput = useRef<HTMLInputElement>(null);

  const openBytes = useCallback(async (bytes: Uint8Array, name: string) => {
    setBusy(true); setStatus(`Parsing ${name} in this tab…`);
    try {
      const { maxRecommendedBytes } = await limits();
      if (bytes.byteLength > maxRecommendedBytes) throw new Error(`${name} is ${(bytes.byteLength / 1024 / 1024).toFixed(1)} MiB; the browser maximum is ${maxRecommendedBytes / 1024 / 1024} MiB. Use the CLI for larger files`);
      setSource({ bytes, name });
      const parsed = await parseTrace(bytes, name);
      setProvider(createInMemoryProvider(parsed.collection, parsed.collection_id));
      setStatus(`${name} is open. No trace bytes left this tab.`);
    } catch (error) {
      setProvider(undefined);
      setStatus(`${error instanceof Error ? error.message : "Could not parse trace"}. Use a browser adapter for another format.`);
    } finally { setBusy(false); }
  }, []);

  const openFile = async (file?: File) => {
    if (!file) return;
    await openBytes(new Uint8Array(await file.arrayBuffer()), file.name);
  };

  const chooseAdapter = async (file?: File) => {
    if (!file) return;
    const bytes = new Uint8Array(await file.arrayBuffer());
    const identity = await adapterIdentity(bytes);
    setPendingAdapter({ bytes, name: file.name, ...identity });
  };

  const confirmAdapter = async () => {
    if (!pendingAdapter || !source) return;
    const adapter = pendingAdapter;
    setPendingAdapter(undefined); setBusy(true); setStatus(`Running confirmed adapter ${adapter.name} in the browser sandbox…`);
    try {
      const parsed = await runAdapter(adapter.bytes, source.bytes, source.name);
      setProvider(createInMemoryProvider(parsed.collection, parsed.collection_id));
      setStatus(`${source.name} is open through ${adapter.name}. The adapter is held only for this session.`);
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Adapter failed");
    } finally { setBusy(false); }
  };

  const picker = <>
    <input ref={traceInput} hidden type="file" onChange={(event) => void openFile(event.target.files?.[0])} />
    <input ref={adapterInput} hidden type="file" accept=".wasm,application/wasm" onChange={(event) => void chooseAdapter(event.target.files?.[0])} />
  </>;

  return <div className="browser-app" onDragOver={(event) => { event.preventDefault(); setDragging(true); }} onDragLeave={() => setDragging(false)} onDrop={(event) => { event.preventDefault(); setDragging(false); void openFile(event.dataTransfer.files[0]); }}>
    {picker}
    {!provider ? <main className={`landing ${dragging ? "dragging" : ""}`}>
      <nav><a className="wordmark" href="https://rlviz.dev">RLViz</a><button onClick={() => setHelp(true)}>adapter help</button></nav>
      <section className="hero">
        <p className="kicker">Browser viewer · local files only</p>
        <h1>Open the trace. Keep the trace.</h1>
        <p className="privacy">Your trace is parsed in this tab and never uploaded.</p>
        <p className="support">Canonical NDJSON, Inspect AI EvalLog JSON, and Verifiers GenerateOutputs JSON run fully client-side through the same Go core as the CLI.</p>
        <div className="primary-actions"><button className="primary" disabled={busy} onClick={() => traceInput.current?.click()}>{busy ? "parsing…" : "open a local trace"}</button><span>or drag it anywhere onto this page</span></div>
        <div className="example-actions"><span>load example</span>{examples.map(([label, name, value]) => <button key={name} disabled={busy} onClick={() => void openBytes(encoder.encode(value), name)}>{label}</button>)}</div>
        <p className="status" role="status">{status}</p>
        {source && <div className="adapter-callout"><p>This format needs an adapter. The module runs in the browser sandbox after you confirm its digest.</p><button onClick={() => adapterInput.current?.click()}>upload WASM adapter</button><button onClick={() => setHelp(true)}>show adapter prompt</button></div>}
      </section>
      <section className="privacy-proof" aria-label="Privacy guarantees"><div><b>zero upload</b><span>File and adapter APIs read local bytes directly.</span></div><div><b>in-memory index</b><span>No SQLite, account, cache, or server is involved.</span></div><div><b>static app</b><span>No CDN dependencies or trace-data requests.</span></div></section>
    </main> : <>
      <header className="viewer-bar"><div><b>RLViz browser viewer</b><span>{status}</span></div><div><button onClick={() => traceInput.current?.click()}>open another trace</button><button onClick={() => adapterInput.current?.click()}>WASM adapter</button><button onClick={() => setHelp(true)}>adapter help</button></div></header>
      <Viewer provider={provider} />
    </>}
    <footer className="browser-footer"><a href="https://rlviz.dev">docs at rlviz.dev</a><span>private formats and big cohorts: <code>brew install TheSnakeFang/tap/rlviz</code></span></footer>
    {help && <div className="browser-dialog" role="dialog" aria-modal="true" aria-labelledby="adapter-help-title"><section><header><h2 id="adapter-help-title">Ask your local coding agent</h2><button onClick={() => setHelp(false)}>close</button></header><p>This prompt defines the complete browser adapter contract. The app does not send it or your trace anywhere.</p><pre>{adapterPrompt}</pre><button onClick={() => void navigator.clipboard.writeText(adapterPrompt)}>copy prompt</button></section></div>}
    {pendingAdapter && <div className="browser-dialog" role="dialog" aria-modal="true" aria-labelledby="adapter-confirm-title"><section><header><h2 id="adapter-confirm-title">Confirm browser adapter</h2><button onClick={() => setPendingAdapter(undefined)}>cancel</button></header><p>This module can compute inside the browser sandbox. It receives the current trace bytes and is not persisted.</p><dl><dt>module</dt><dd>{pendingAdapter.name}</dd><dt>size</dt><dd>{pendingAdapter.size.toLocaleString()} bytes</dd><dt>SHA-256</dt><dd><code>{pendingAdapter.digest}</code></dd></dl><button className="primary" disabled={!source} onClick={() => void confirmAdapter()}>{source ? "confirm and run once" : "open a trace first"}</button></section></div>}
  </div>;
}
