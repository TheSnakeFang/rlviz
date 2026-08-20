import { useRef } from "react";
import type { ReactNode } from "react";
import { OutcomeView, TranscriptView } from "./ResearchViews";
import { deriveOutcome, deriveTool } from "./research";
import { preview } from "./format";
import type { Trajectory } from "./types";
import type { MobileSurface, WorkspaceLane } from "./workspace";

const surfaces: Array<{ id: MobileSurface; label: string }> = [
  { id: "summary", label: "Summary" },
  { id: "story", label: "Story" },
  { id: "evidence", label: "Evidence" },
  { id: "details", label: "Details" },
];

function value(value: unknown) {
  return value === undefined || value === null || value === "" ? "—" : String(value);
}

function MobileSummary({ trajectory, onStory, onEvidence }: { trajectory: Trajectory; onStory: () => void; onEvidence: () => void }) {
  const outcome = deriveOutcome(trajectory, trajectory.signals);
  const pass = outcome.pass?.value;
  const toolCalls = trajectory.events.filter((event) => !!deriveTool(event)).length;
  const result = pass === undefined ? outcome.status ?? trajectory.status : pass ? "Pass" : "Fail";
  const verifierReason = outcome.graders.find((grader) => grader.reason !== undefined)?.reason;
  return <section className="mobile-summary" aria-label="Trajectory summary">
    <header>
      <span className="eyebrow">Trajectory</span>
      <h1>{trajectory.name ?? trajectory.case_name ?? trajectory.id}</h1>
      <p>{trajectory.model ? `${trajectory.model} · ` : ""}{trajectory.run_name ?? trajectory.group_name ?? "Recorded agent run"}</p>
    </header>
    <div className="mobile-summary-result" data-result={pass === true ? "pass" : pass === false ? "fail" : "unknown"}>
      <span>Outcome</span><strong>{value(result)}</strong>
      <small>{value(outcome.termination ?? trajectory.termination)}</small>
    </div>
    <dl className="mobile-summary-facts">
      <div><dt>Reward</dt><dd>{value(outcome.reward.total?.value ?? trajectory.total_reward)}</dd></div>
      <div><dt>Steps</dt><dd>{trajectory.events.length}</dd></div>
      <div><dt>Tool calls</dt><dd>{toolCalls}</dd></div>
      <div><dt>Errors</dt><dd>{outcome.errorEventIds.length}</dd></div>
    </dl>
    {verifierReason !== undefined && <section className="mobile-summary-reason"><span>Verifier reason</span><p>{typeof verifierReason === "string" ? verifierReason : preview(verifierReason, 800)}</p></section>}
    <button className="mobile-primary-action" type="button" onClick={onStory}>Read the story</button>
    <button className="mobile-secondary-action" type="button" onClick={onEvidence}>Inspect verifier evidence</button>
  </section>;
}

export function MobileTrajectoryReader({ lane, trajectory, surface, details, onBrowse, onSurface, onSelect }: {
  lane: WorkspaceLane;
  trajectory?: Trajectory;
  surface: MobileSurface;
  details: ReactNode;
  onBrowse: () => void;
  onSurface: (surface: MobileSurface) => void;
  onSelect: (index: number) => void;
}) {
  const scrollRef = useRef<HTMLElement>(null);
  if (!trajectory) return <main className="mobile-reader mobile-reader-loading" aria-label="Mobile trajectory reader">Loading trajectory…</main>;
  const selected = Math.min(lane.selected, Math.max(0, trajectory.events.length - 1));
  const selectID = (id: string, reveal = false) => {
    const index = trajectory.events.findIndex((event) => event.id === id);
    if (index >= 0) onSelect(index);
    if (reveal) onSurface("details");
  };
  return <main className="mobile-reader" aria-label="Mobile trajectory reader" data-surface={surface}>
    <section ref={scrollRef} className="mobile-reader-content">
      {surface === "summary" && <MobileSummary trajectory={trajectory} onStory={() => onSurface("story")} onEvidence={() => onSurface("evidence")} />}
      {surface === "story" && <TranscriptView events={trajectory.events} selectedId={trajectory.events[selected]?.id ?? ""} selectedIndex={selected} scrollRef={scrollRef} onSelect={selectID} />}
      {surface === "evidence" && <OutcomeView trajectory={trajectory} onSelect={(id) => selectID(id, true)} />}
      {surface === "details" && details}
    </section>
    <nav className="mobile-reader-nav" aria-label="Trajectory reading views">
      <button type="button" onClick={onBrowse}>Browse</button>
      {surfaces.map((item) => <button key={item.id} type="button" aria-current={surface === item.id ? "page" : undefined} onClick={() => onSurface(item.id)}>{item.label}</button>)}
    </nav>
  </main>;
}
