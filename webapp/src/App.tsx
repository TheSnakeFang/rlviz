import { lazy, Suspense, useCallback, useEffect, useRef, useState } from "react";
import type { ViewerProvider } from "../../web/src/provider";
import { workspaceStorageKey } from "../../web/src/workspace";
import codingExample from "../../examples/gallery/coding-agent-bugfix.ndjson?url";
import researchExample from "../../examples/gallery/web-research-agent.ndjson?url";
import cohortExample from "../../examples/gallery/checkout-cohort.ndjson?url";
import { adapterIdentity, runAdapter } from "./adapter";
import { createInMemoryProvider } from "./provider";
import { fetchVerifiedPublicBundle, parsePublicBundleHandoff } from "./publicBundle";
import { limits, parseTrace } from "./wasm";

const viewerModule = import("../../web/src/App");
const Viewer = lazy(() => viewerModule.then(({ App }) => ({ default: App })));
const examples = [
  ["300-event coding trace", "coding-agent-bugfix.ndjson", codingExample],
  ["web research trace", "web-research-agent.ndjson", researchExample],
  ["checkout cohort", "checkout-cohort.ndjson", cohortExample],
] as const;
const initialStatus = "Ready for a reviewed .rlviz bundle or supported local trace.";

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
  const [publicBundle, setPublicBundle] = useState(() => parsePublicBundleHandoff(window.location.search));
  const [provider, setProvider] = useState<ViewerProvider>();
  const [viewerGeneration, setViewerGeneration] = useState(0);
  const [bootstrapping, setBootstrapping] = useState(true);
  const [activeSample, setActiveSample] = useState("checkout-cohort.ndjson");
  const [source, setSource] = useState<{ bytes: Uint8Array; name: string }>();
  const [status, setStatus] = useState(initialStatus);
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [help, setHelp] = useState(false);
  const [pendingAdapter, setPendingAdapter] = useState<PendingAdapter>();
  const traceInput = useRef<HTMLInputElement>(null);
  const directoryInput = useRef<HTMLInputElement>(null);
  const adapterInput = useRef<HTMLInputElement>(null);
  const autoLoaded = useRef(false);

  const resetWorkspaceForSource = useCallback(() => {
    try { localStorage.removeItem(workspaceStorageKey); } catch { /* persistence is optional */ }
    const url = new URL(window.location.href);
    ["workspace", "trajectory", "trajectory_id", "indexed", "mode", "view", "left", "right", "bundle", "sha256"].forEach((key) => url.searchParams.delete(key));
    history.replaceState(null, "", url);
    setViewerGeneration((value) => value + 1);
  }, []);

  const openBytes = useCallback(async (bytes: Uint8Array, name: string, resetWorkspace = false) => {
    setBusy(true); setStatus(`Parsing ${name} in this tab…`);
    try {
      const { maxRecommendedBytes } = await limits();
      if (bytes.byteLength > maxRecommendedBytes) throw new Error(`${name} is ${(bytes.byteLength / 1024 / 1024).toFixed(1)} MiB; the browser maximum is ${maxRecommendedBytes / 1024 / 1024} MiB. Use the CLI for larger files`);
      setSource({ bytes, name });
      const parsed = await parseTrace(bytes, name);
      if (resetWorkspace) resetWorkspaceForSource();
      setProvider(createInMemoryProvider(parsed.collection, parsed.collection_id));
      setStatus(`${name} is open. No trace bytes left this tab.`);
    } catch (error) {
      setProvider(undefined);
      setStatus(`${error instanceof Error ? error.message : "Could not parse trace"}. Use a browser adapter for another format.`);
    } finally { setBusy(false); }
  }, [resetWorkspaceForSource]);

  const openFile = async (file?: File) => {
    if (!file) return;
    setActiveSample("");
    await openBytes(new Uint8Array(await file.arrayBuffer()), file.name, true);
  };

  const openExample = async (url: string, name: string, resetWorkspace = true) => {
    setActiveSample(name);
    setBusy(true); setStatus(`Loading ${name}…`);
    try {
      const response = await fetch(url);
      if (!response.ok) throw new Error(`Could not load ${name}`);
      await openBytes(new Uint8Array(await response.arrayBuffer()), name, resetWorkspace);
    } catch (error) {
      setStatus(error instanceof Error ? error.message : `Could not load ${name}`);
      setBusy(false);
    }
  };

  useEffect(() => {
    if (autoLoaded.current) return;
    autoLoaded.current = true;
    if (publicBundle.kind !== "none") { setBootstrapping(false); return; }
    void openExample(cohortExample, "checkout-cohort.ndjson", false).finally(() => setBootstrapping(false));
  }, []);

  useEffect(() => { directoryInput.current?.setAttribute("webkitdirectory", ""); }, []);

  const openDirectory = async (files?: FileList | null) => {
    const candidates = [...(files ?? [])].filter((file) => !file.name.startsWith(".") && /\.(ndjson|json)$/i.test(file.name));
    if (candidates.length !== 1) {
      setStatus(`Choose a directory containing exactly one supported .ndjson or .json trace export; found ${candidates.length}.`);
      return;
    }
    await openFile(candidates[0]);
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
      resetWorkspaceForSource();
      setProvider(createInMemoryProvider(parsed.collection, parsed.collection_id));
      setStatus(`${source.name} is open through ${adapter.name}. The adapter is held only for this session.`);
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Adapter failed");
    } finally { setBusy(false); }
  };

  const confirmPublicBundle = async () => {
    if (publicBundle.kind !== "ready") return;
    setBusy(true); setStatus(`Fetching ${publicBundle.request.name} after your confirmation…`);
    try {
      const { maxRecommendedBytes } = await limits();
      const bytes = await fetchVerifiedPublicBundle(publicBundle.request, maxRecommendedBytes);
      setActiveSample("");
      await openBytes(bytes, publicBundle.request.name, true);
    } catch (error) {
      setStatus(error instanceof Error ? error.message : "Could not verify shared bundle");
      setBusy(false);
    }
  };

  const dismissPublicBundle = () => {
    const url = new URL(window.location.href);
    url.searchParams.delete("bundle"); url.searchParams.delete("sha256");
    history.replaceState(null, "", url);
    setPublicBundle({ kind: "none" });
  };

  const picker = <>
    <input ref={traceInput} hidden type="file" onChange={(event) => void openFile(event.target.files?.[0])} />
    <input ref={directoryInput} hidden type="file" multiple onChange={(event) => void openDirectory(event.target.files)} />
    <input ref={adapterInput} hidden type="file" accept=".wasm,application/wasm" onChange={(event) => void chooseAdapter(event.target.files?.[0])} />
  </>;

  return <div className={`browser-app ${provider || bootstrapping ? "viewer-open" : ""}`} onDragOver={(event) => { event.preventDefault(); setDragging(true); }} onDragLeave={() => setDragging(false)} onDrop={(event) => { event.preventDefault(); setDragging(false); void openFile(event.dataTransfer.files[0]); }}>
    {picker}
    {!provider ? bootstrapping ? <div className="viewer-boot" role="status" aria-label="Loading RLViz"><span /><span /><span /></div> : <main className={`landing ${dragging ? "dragging" : ""}`}>
      <nav><a className="wordmark" href="/">RLViz</a><div className="landing-links"><a href="/docs.html">Docs</a><a href="https://benchmarks.rlviz.dev">Benchmarks</a><a href="https://github.com/TheSnakeFang/rlviz">GitHub</a></div></nav>
      <div className="open-shell">
        {publicBundle.kind !== "none" && <section className="public-bundle" aria-label="Shared bundle confirmation">
          <h1>{publicBundle.kind === "ready" ? "Open shared trajectory" : "This link cannot be opened"}</h1>
          {publicBundle.kind === "invalid" ? <p>{publicBundle.message}</p> : <>
            <p>Fetch this public <code>.rlviz</code> file and verify its SHA-256 before opening it. Nothing is uploaded.</p>
            <dl><dt>Source</dt><dd>{publicBundle.request.url.origin}{publicBundle.request.url.pathname}</dd><dt>SHA-256</dt><dd><code>{publicBundle.request.digest}</code></dd></dl>
            <div className="bundle-actions"><button className="primary" disabled={busy} onClick={() => void confirmPublicBundle()}>{busy ? "Verifying…" : "Verify and open"}</button><button disabled={busy} onClick={dismissPublicBundle}>Cancel</button></div>
          </>}
          {publicBundle.kind === "invalid" && <button disabled={busy} onClick={dismissPublicBundle}>Dismiss</button>}
        </section>}
        <section className="local-open" aria-labelledby="local-open-title">
          <h2 id="local-open-title">Open a local trace</h2>
          <p>Choose a supported file from this device. It stays in this tab.</p>
          <div className="local-actions"><button disabled={busy} onClick={() => traceInput.current?.click()}>{busy ? "Parsing…" : "Choose file"}</button><span>or drop it on this page</span></div>
          <a href="/supported-formats.html">Supported formats</a>
        </section>
        {status !== initialStatus && <p className="status" role="status">{status}</p>}
        {source && <div className="adapter-callout"><p>This format needs an adapter. The module runs in the browser sandbox after you confirm its digest.</p><button onClick={() => adapterInput.current?.click()}>upload WASM adapter</button><button onClick={() => setHelp(true)}>show adapter prompt</button></div>}
      </div>
    </main> : <>
      <Suspense fallback={<div className="viewer-loading" role="status">loading viewer…</div>}><Viewer key={viewerGeneration} provider={provider} setup={{ mode: "browser", status, samples: [...(activeSample ? [] : [{ label: "local trace", value: "" }]), ...examples.map(([label, value]) => ({ label, value }))], selectedSample: activeSample, onSample: (value) => { const sample = examples.find(([, name]) => name === value); if (sample) void openExample(sample[2], sample[1]); }, onOpenTrace: () => traceInput.current?.click(), onOpenDirectory: () => directoryInput.current?.click(), onOpenAdapter: () => adapterInput.current?.click(), onAdapterHelp: () => setHelp(true) }} /></Suspense>
    </>}
    {help && <div className="browser-dialog" role="dialog" aria-modal="true" aria-labelledby="adapter-help-title"><section><header><h2 id="adapter-help-title">Ask your local coding agent</h2><button onClick={() => setHelp(false)}>close</button></header><p>This prompt defines the complete browser adapter contract. The app does not send it or your trace anywhere.</p><pre>{adapterPrompt}</pre><button onClick={() => void navigator.clipboard.writeText(adapterPrompt)}>copy prompt</button></section></div>}
    {pendingAdapter && <div className="browser-dialog" role="dialog" aria-modal="true" aria-labelledby="adapter-confirm-title"><section><header><h2 id="adapter-confirm-title">Confirm browser adapter</h2><button onClick={() => setPendingAdapter(undefined)}>cancel</button></header><p>This module can compute inside the browser sandbox. It receives the current trace bytes and is not persisted.</p><dl><dt>module</dt><dd>{pendingAdapter.name}</dd><dt>size</dt><dd>{pendingAdapter.size.toLocaleString()} bytes</dd><dt>SHA-256</dt><dd><code>{pendingAdapter.digest}</code></dd></dl><button className="primary" disabled={!source} onClick={() => void confirmAdapter()}>{source ? "confirm and run once" : "open a trace first"}</button></section></div>}
  </div>;
}
