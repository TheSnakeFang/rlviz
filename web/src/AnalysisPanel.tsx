import type { AnalysisResponse } from "./types";

function valueLabel(value: unknown): string {
  if (typeof value === "string") return value;
  try { return JSON.stringify(value); } catch { return String(value); }
}

export function AnalysisPanel({ analysis, loading, error, onRetry, onJump }: {
  analysis: AnalysisResponse | null;
  loading: boolean;
  error: string;
  onRetry: () => void;
  onJump: (eventId: string) => void;
}) {
  const findings = analysis?.analysis.findings ?? [];
  const signals = analysis?.analysis.signals ?? [];
  const provenance = analysis?.analysis.provenance;
  return <section className="analysis-panel" aria-label="Analyzer findings">
    <header><div><span>Analyzer</span>{analysis && <b className={analysis.cached ? "cached" : "fresh"}>{analysis.cached ? "cache hit" : "fresh"}</b>}</div><small>{findings.length} findings · {signals.length} signals</small></header>
    {loading ? <div className="analysis-state"><i></i>Analyzing trajectory…</div> : error ? <div className="analysis-state error"><span>{error}</span><button onClick={onRetry}>Retry</button></div> : analysis ? <>
      <div className="analysis-provenance" title={`${provenance?.digest}\ninput ${provenance?.input_digest}`}><strong>{provenance?.name}</strong><span>v{provenance?.version} · {analysis.cached ? "cached" : "computed"} {analysis.analyzed_at ? new Date(analysis.analyzed_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : ""}</span></div>
      {findings.length ? <div className="analysis-findings">{findings.map((finding) => <article key={finding.id} className={`analysis-finding severity-${finding.severity}`}>
        <button className="finding-main" onClick={() => finding.event_ids?.[0] && onJump(finding.event_ids[0])} disabled={!finding.event_ids?.length}><span>{finding.kind}</span><strong>{finding.title}</strong>{finding.summary && <small>{finding.summary}</small>}</button>
        {!!finding.event_ids?.length && <div className="finding-events">{finding.event_ids.map((id, index) => <button key={id} onClick={() => onJump(id)} title={`Jump to ${id}`}>#{index + 1}</button>)}</div>}
      </article>)}</div> : <div className="analysis-state empty"><strong>No loops or retries</strong><span>No repeated action patterns detected.</span></div>}
      {!!signals.length && <div className="analysis-signals"><span>Derived signals</span>{signals.map((signal) => <button key={signal.id} onClick={() => signal.event_id && onJump(signal.event_id)} disabled={!signal.event_id}><strong>{signal.name.replace("analyzer.", "")}</strong><small>{valueLabel(signal.value)}{signal.unit ? ` ${signal.unit}` : ""}</small></button>)}</div>}
    </> : <div className="analysis-state empty"><span>Analysis is available for indexed trajectories.</span></div>}
  </section>;
}
