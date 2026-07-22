import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode, RefObject } from "react";
import { DockviewReact } from "dockview-react";
import type { DockviewApi, DockviewReadyEvent, GroupNavigationDirection, IDockviewPanelHeaderProps, IDockviewPanelProps, Position } from "dockview-react";
import { daemonProvider, ViewerProviderContext } from "./provider";
import type { ViewerProvider } from "./provider";
import { commandDefinition, commandIds, commands, dispatchCommand, firstBindingLabel, useCommands, useKeymapRevision } from "./commands";
import type { CommandId } from "./commands";
import { axisX, episodeIndexForEvent, episodesFor, episodeWindow, firstAnomaly, glyphForKind, layoutStrip, panWindowToInclude, stripX, verdictGlyph, zoomWindow } from "./instrument";
import type { Episode, StripMark } from "./instrument";
import type { AnalysisResponse, BrowseResponse, BrowseTrajectory, Trajectory, TrajectoryEvent } from "./types";
import { preview, title } from "./format";
import { sampleTrajectory } from "./sample";
import { applyPresentationTheme } from "./presentation";
import type { PresentationConfig } from "./types";
import { effectiveDepth, emptyWorkspace, laneId, legacyWorkspace, normalizeWorkspace, snapshotLabel, workspaceFromSearch, workspaceStorageKey, workspaceURL } from "./workspace";
import type { WorkspaceLane, WorkspaceState } from "./workspace";

const fidelityNames = ["hairline", "glyphs", "detail"];
type LaneData = { trajectory: Trajectory; analysis: AnalysisResponse | null; presentation?: PresentationConfig };

function metric(row: BrowseTrajectory, name: string): unknown {
  const metrics = row.metrics.metrics ?? row.metrics.normalized_metrics ?? row.metrics;
  return metrics[name] ?? row.metrics[name];
}

function rowKey(row: BrowseTrajectory): string { return laneId(row.source_id, row.trajectory.id); }
function eventDetail(event: TrajectoryEvent): unknown { return event.output ?? event.input ?? event.content ?? event.data ?? event.raw ?? event; }
function eventText(event: TrajectoryEvent): string { return title(event) || `${event.kind} event`; }
function eventReward(event: TrajectoryEvent): number | undefined {
  if (typeof event.reward === "number") return event.reward;
  if (event.kind === "reward" && event.data && typeof event.data === "object" && "total" in event.data && typeof event.data.total === "number") return event.data.total;
  return undefined;
}

function fakeBrowse(trajectory: Trajectory): BrowseResponse {
  return { sources: [{ id: "sample" }], count: 1, trajectories: [{
    source_id: "sample", source_name: "sample", case_name: trajectory.name, group_name: trajectory.group_id,
    trajectory: { ...trajectory, events: undefined } as Omit<Trajectory, "events">,
    metrics: { trajectory: { ...trajectory, events: undefined }, event_count: trajectory.events.length, error_count: trajectory.events.filter((event) => event.kind === "error").length, reward: trajectory.total_reward },
  }] };
}

function savedWorkspace(): WorkspaceState | undefined {
  try {
    const value = localStorage.getItem(workspaceStorageKey);
    return value ? normalizeWorkspace(JSON.parse(value)) : undefined;
  } catch { return undefined; }
}

function initialWorkspace(): WorkspaceState {
  return workspaceFromSearch(window.location.search) ?? legacyWorkspace(window.location.search) ?? savedWorkspace() ?? emptyWorkspace();
}

function HelpOverlay({ onClose }: { onClose: () => void }) {
  useCommands("overlay", { [commandIds.trajectory.dismiss]: onClose, [commandIds.trajectory.toggleHelp]: onClose });
  const active = commands.filter((command) => (command.scope === "workspace" || command.scope === "trajectory" || command.scope === "all") && command.defaultBindings.length);
  return <div className="instrument-overlay" role="dialog" aria-label="Active keyboard shortcuts">
    <div className="help-card"><header><h2>workspace keys</h2><button onClick={onClose}>close Esc</button></header>
      <dl>{active.map((command) => <div key={command.id}><dt>{command.defaultBindings.join(" / ")}</dt><dd>{command.label}</dd></div>)}</dl>
    </div>
  </div>;
}

function CollectionStrip({ row, fidelity }: { row: BrowseTrajectory; fidelity: number }) {
  const count = Math.max(1, Number(metric(row, "event_count") ?? 1));
  const errors = Number(metric(row, "error_count") ?? 0);
  const width = Math.min(100, 18 + Math.log2(count + 1) * 13);
  const shape = row.shape;
  if (!shape) {
    // No shape summary from this provider yet: render only what is true —
    // length and counts. Never synthesize texture or positions.
    return <span className="cat-line" style={{ width: `${width}%` }}>{errors > 0 && <em className="strip-badge" title={`${errors} errors`}>{errors}✕</em>}</span>;
  }
  if (fidelity === 0) {
    return <span className="cat-line" style={{ width: `${width}%` }}>
      {shape.slots.map((slot, index) => slot.landmark ? <i key={index} className={`strip-landmark ${slot.landmark}`} style={{ left: `${((index + 0.5) / shape.slots.length) * 100}%` }} /> : null)}
    </span>;
  }
  const glyphSlots = shape.slots.map((slot, index) => {
    if (slot.landmark === "error") return <b key={index} className="g-error">✕</b>;
    if (slot.landmark === "context") return <b key={index} className="g-context">◇</b>;
    if (slot.landmark === "evidence") return <b key={index} className="g-evidence">◆</b>;
    if (!slot.count) return <span key={index} className="g-gap"> </span>;
    return <span key={index}>{slot.tools * 2 > slot.count ? "▮" : "·"}</span>;
  });
  return <span className="cat-glyphs" style={{ width: `${width}%` }}>{glyphSlots}{fidelity >= 2 && <small>{shape.events} events{errors ? ` · ${errors} errors` : ""}</small>}</span>;
}

function Rail({ root, rows, workspace, fidelity, onActivate, onSelect, onOpen, onAdd, onQuery }: {
  root: RefObject<HTMLElement | null>; rows: BrowseTrajectory[]; workspace: WorkspaceState; fidelity: number;
  onActivate: () => void; onSelect: (index: number) => void; onOpen: () => void; onAdd: () => void; onQuery: (query: string) => void;
}) {
  const selected = Math.min(workspace.railSelected, Math.max(0, rows.length - 1));
  return <main ref={root} tabIndex={0} className={`workspace-rail ${workspace.active === "rail" ? "active-zone" : ""}`} aria-label="Browse trajectories" data-filter={workspace.railQuery} data-fidelity={fidelityNames[fidelity]} onFocus={onActivate}>
    <header><div><h1>Trajectories</h1><p>{rows.length === 1 ? "1 trajectory" : `${rows.length} trajectories`}</p></div></header>
    <div className="rail-controls"><label>Filter <input id="browse-filter" value={workspace.railQuery} onChange={(event) => onQuery(event.target.value)} onKeyDown={(event) => { if (event.key === "Escape") { event.preventDefault(); root.current?.focus(); } }} /></label></div>
    <div className="fidelity-readout">fidelity <b>{fidelityNames[fidelity]}</b> · [ ]</div>
    <section className={`browse-list rail-fidelity-${fidelity}`} role="listbox" aria-label="Trajectory collection" data-fidelity-level={`L${fidelity}`}>
      {rows.map((row, index) => <button key={rowKey(row)} role="option" aria-selected={index === selected} data-fidelity-level={`L${fidelity}`} data-columns={fidelity >= 2 ? "true" : "false"} className={`browse-row ${index === selected ? "selected" : ""}`} onClick={() => onSelect(index)} onDoubleClick={onOpen}>
        {fidelity >= 1 && <span className="verdict">{verdictGlyph(row)}</span>}<span className="identity"><b>{row.trajectory.id}</b>{fidelity >= 2 && <small>{row.case_name ?? row.group_name ?? row.source_name}</small>}</span><CollectionStrip row={row} fidelity={fidelity} />
        {fidelity >= 2 && <><span className="numeric events-column">{String(metric(row, "event_count") ?? "—")} ev</span><span className="numeric reward-column">{metric(row, "reward") === undefined ? "" : `r ${String(metric(row, "reward"))}`}</span></>}
        {fidelity >= 2 && <span className="row-state">{row.source_name}{row.group_name ? ` · ${row.group_name}` : ""}</span>}
      </button>)}
      {!rows.length && <p className="empty-state">No trajectories match this filter.</p>}
    </section>
    <span className="rail-actions"><button onClick={onAdd}>add lane</button></span>
  </main>;
}

function useMeasuredWidth(ref: RefObject<HTMLElement | null>, fallback = 800): number {
  const [width, setWidth] = useState(fallback);
  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    const measure = () => setWidth((current) => {
      const next = Math.round(node.getBoundingClientRect().width);
      return next >= 120 ? next : current;
    });
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, [ref]);
  return width;
}

function ShapeStrip({ trajectory, selected, hover, axis, compact = false, label, onSelect, onHover, onAscend }: {
  trajectory: Trajectory; selected: number; hover?: number; axis: { start: number; end: number }; compact?: boolean; label?: string; onSelect: (index: number) => void; onHover: (index?: number) => void; onAscend?: () => void;
}) {
  // Pixel-space rendering (workspace-spec v3 §0.1): marks have fixed sizes at
  // true positions; the SVG viewBox tracks the measured width so nothing
  // stretches. Density is handled by layoutStrip's binning, never by scaling.
  const containerRef = useRef<HTMLElement>(null);
  const width = useMeasuredWidth(containerRef);
  const events = trajectory.events;
  const min = events[0]?.sequence ?? 0;
  const height = compact ? 56 : 200;
  const x = (sequence: number) => stripX(sequence, axis, width);
  const layout = layoutStrip(events, axis, width);
  const visibleCount = layout.mode === "marks" ? layout.marks.length : layout.bins.reduce((total, bin) => total + bin.count, 0) + layout.landmarks.length;
  const bands = compact ? [] : episodesFor(events);
  const baseline = compact ? 44 : 105;
  const markFor = (mark: StripMark) => {
    if (mark.kind === "error") return <path data-event-index={mark.index} key={`m${mark.index}`} className="event-shape error" d={`M${mark.x - 5},${baseline} L${mark.x},${baseline - 15} L${mark.x + 5},${baseline} Z`} />;
    if (mark.kind === "context") return <path data-event-index={mark.index} key={`m${mark.index}`} className="event-shape context" d={`M${mark.x},${baseline - 18} l6,8 -6,8 -6,-8 Z`} />;
    if (mark.kind === "evidence") return <circle data-event-index={mark.index} key={`m${mark.index}`} className="event-shape evidence" cx={mark.x} cy={baseline - 12} r="5" />;
    if (mark.kind === "tool") return <rect data-event-index={mark.index} key={`m${mark.index}`} className="event-shape tool" x={mark.x - 2} y={baseline - 26} width="4" height="26" />;
    return <line data-event-index={mark.index} key={`m${mark.index}`} className="event-shape nominal" x1={mark.x} x2={mark.x} y1={baseline - 10} y2={baseline} />;
  };
  const rewards: Array<{ x: number; value: number }> = [];
  if (!compact) {
    let reward = 0;
    events.forEach((event) => {
      if (event.sequence < axis.start || event.sequence > axis.end) return;
      reward = eventReward(event) ?? reward;
      rewards.push({ x: x(event.sequence), value: reward });
    });
  }
  const rewardMin = Math.min(0, ...rewards.map((point) => point.value)), rewardMax = Math.max(1, ...rewards.map((point) => point.value));
  const rewardPath = rewards.map((point, index) => `${index ? "L" : "M"}${point.x},${182 - ((point.value - rewardMin) / Math.max(1, rewardMax - rewardMin)) * 26}`).join(" ");
  const selectedX = x(events[selected]?.sequence ?? min);
  return <section ref={containerRef} className={`shape-strip ${compact ? "compact" : ""}`} aria-label={label ?? "Trajectory shape"} data-selected-x={selectedX.toFixed(4)} data-visible-events={visibleCount} data-strip-mode={layout.mode}>
    <svg width="100%" height={height} viewBox={`0 0 ${width} ${height}`} onMouseLeave={() => onHover(undefined)} onMouseMove={(pointer) => {
      const rect = pointer.currentTarget.getBoundingClientRect();
      const px = pointer.clientX - rect.left;
      let nearest: number | undefined; let best = Infinity;
      events.forEach((event, index) => {
        if (event.sequence < axis.start || event.sequence > axis.end) return;
        const distance = Math.abs(x(event.sequence) - px);
        if (distance < best) { best = distance; nearest = index; }
      });
      if (nearest !== undefined) onHover(nearest);
    }} onClick={() => onAscend ? onAscend() : hover !== undefined && onSelect(hover)}>
      {!compact && bands.filter((band) => band.end >= axis.start && band.start <= axis.end).map((band) => {
        const x0 = x(Math.max(axis.start, band.start)), x1 = x(Math.min(axis.end, band.end));
        return <g key={band.key} data-episode-key={band.key} data-episode-start={band.start} data-episode-end={band.end}><rect className="episode-band" x={x0} y="18" width={Math.max(1, x1 - x0)} height="23" /><text className="episode-label" x={x0 + 4} y="34">{x1 - x0 > 44 ? band.label : ""}</text></g>;
      })}
      {layout.mode === "binned" && layout.bins.map((bin, index) => bin.count ? <rect key={`b${index}`} className={`density-bin ${bin.tools * 2 > bin.count ? "tool-heavy" : ""}`} x={bin.x0} y={baseline - 4 - Math.round((bin.count / layout.peak) * 20)} width={Math.max(1, bin.x1 - bin.x0 - 1)} height={4 + Math.round((bin.count / layout.peak) * 20)} /> : null)}
      {layout.mode === "binned" ? layout.landmarks.map(markFor) : layout.marks.map(markFor)}
      {!compact && <>{events.map((event) => event.context?.input_tokens !== undefined && event.context.capacity && event.sequence >= axis.start && event.sequence <= axis.end ? <rect key={`ctx:${event.id}`} className="context-pressure" x={x(event.sequence) - 2} y={153 - 25 * event.context.input_tokens / event.context.capacity} width="4" height={25 * event.context.input_tokens / event.context.capacity} /> : null)}<path className="reward-curve" d={rewardPath} /></>}
      {hover !== undefined && events[hover] && <line className="skimmer-line" x1={x(events[hover].sequence)} x2={x(events[hover].sequence)} y1="5" y2={height - 5} />}
      <line data-testid="playhead" className="playhead" x1={selectedX} x2={selectedX} y1="5" y2={height - 5} />
    </svg>
  </section>;
}

function EpisodeStrip({ trajectory, lane, episodes, selectedEpisode, onDescend }: { trajectory: Trajectory; lane: WorkspaceLane; episodes: Episode[]; selectedEpisode: number; onDescend: (episode: Episode) => void }) {
  const containerRef = useRef<HTMLElement>(null);
  const width = useMeasuredWidth(containerRef);
  const x = (sequence: number) => stripX(sequence, lane.axis, width);
  const percent = (px: number) => (px / width) * 100;
  const selectedSequence = trajectory.events[lane.selected]?.sequence ?? trajectory.events[0]?.sequence ?? 0;
  return <section ref={containerRef} className="episode-strip" aria-label="Trajectory shape" data-selected-x={x(selectedSequence).toFixed(4)} data-selected-episode={episodes[selectedEpisode]?.key ?? ""}>
    <div className="episode-axis">
      {episodes.filter((episode) => episode.end >= lane.axis.start && episode.start <= lane.axis.end).map((episode, index) => {
        const actualIndex = episodes.indexOf(episode);
        const nextStart = episodes[actualIndex + 1]?.start ?? Math.max(episode.end + 1, lane.axis.end);
        const left = percent(x(Math.max(lane.axis.start, episode.start)));
        const right = percent(x(Math.min(lane.axis.end, nextStart)));
        return <button key={episode.key} className={`episode-button ${actualIndex === selectedEpisode ? "selected" : ""}`} data-episode-index={actualIndex} data-episode-key={episode.key} data-episode-start={episode.start} data-episode-end={episode.end} style={{ left: `${left}%`, width: `${Math.max(0.8, right - left)}%` }} onClick={(event) => { event.stopPropagation(); onDescend(episode); }}>
          <b>{episode.label}</b><small>{episode.inferred ? "inferred" : "adapter"} · {episode.endIndex - episode.startIndex + 1} events</small>
        </button>;
      })}
      <i className="episode-playhead" style={{ left: `${percent(x(selectedSequence))}%` }} />
    </div>
    {episodes[selectedEpisode] && <EpisodeSummary trajectory={trajectory} episode={episodes[selectedEpisode]} />}
  </section>;
}

function EpisodeSummary({ trajectory, episode }: { trajectory: Trajectory; episode: Episode }) {
  const events = trajectory.events.slice(episode.startIndex, episode.endIndex + 1);
  const counts = new Map<string, number>(); events.forEach((event) => counts.set(event.kind, (counts.get(event.kind) ?? 0) + 1));
  const errors = events.filter((event) => event.kind === "error").length;
  return <div className="episode-summary" aria-label="Selected episode summary"><span><b>{episode.label}</b><small>{episode.inferred ? "deterministic fallback" : "alignment key"}</small></span><span><small>kinds</small><b>{[...counts].map(([kind, count]) => `${kind} ${count}`).join(" · ")}</b></span><span><small>errors</small><b>{errors}</b></span><span><small>span</small><b>#{episode.start}–#{episode.end}</b></span></div>;
}

function ScopedEvents({ trajectory, episode, selected, onSelect }: { trajectory: Trajectory; episode: Episode; selected: number; onSelect: (index: number) => void }) {
  return <section className="lane-events" aria-label="Episode events" data-episode-key={episode.key}>
    {trajectory.events.slice(episode.startIndex, episode.endIndex + 1).map((event, offset) => { const index = episode.startIndex + offset; return <button key={event.id} className={`lane-event ${index === selected ? "selected" : ""}`} onClick={(pointer) => { pointer.stopPropagation(); onSelect(index); }}><span className="address">#{event.sequence}</span><span className="kind-glyph">{glyphForKind(event.kind)}</span><span><small>{event.kind}</small><b>{eventText(event)}</b></span></button>; })}
  </section>;
}

function SourceRecord({ event }: { event: TrajectoryEvent }) {
  return <section className="lane-source" aria-label="Event source"><div className="source-provenance"><h3>Provenance</h3><dl><dt>event</dt><dd>{event.id}</dd><dt>address</dt><dd>#{event.sequence}</dd><dt>source</dt><dd>{event.source?.path ?? "canonical record"}{event.source?.line ? `:${event.source.line}` : ""}</dd>{event.source?.byte_start !== undefined && <><dt>bytes</dt><dd>{event.source.byte_start}–{event.source.byte_end ?? "?"}</dd></>}</dl></div><div><h3>Raw normalized record</h3><pre>{JSON.stringify(event.raw ?? event, null, 2)}</pre></div></section>;
}

function LaneTrack({ lane, data, active, reference, hover, onActivate, onSelect, onHover, onDescend, onAscend }: {
  lane: WorkspaceLane; data?: LaneData; active: boolean; reference: boolean; hover?: number; onActivate: () => void; onSelect: (index: number) => void; onHover: (index?: number) => void; onDescend: (episode?: Episode) => void; onAscend: () => void;
}) {
  const trajectory = data?.trajectory;
  const depth = effectiveDepth(lane);
  const selected = trajectory ? Math.min(lane.selected, trajectory.events.length - 1) : 0;
  const episodes = trajectory ? episodesFor(trajectory.events) : [];
  const selectedEpisode = episodeIndexForEvent(episodes, selected);
  const episode = episodes[selectedEpisode];
  return <main tabIndex={0} aria-label={lane.band === "focus" ? "Read trajectory" : `Context lane ${lane.trajectoryId}`} className={`lane-track depth-${depth} ${lane.band}-lane ${active ? "active-zone" : ""} ${reference ? "reference-lane" : ""}`} data-lane-id={lane.id} data-trajectory={lane.trajectoryId} data-depth={depth} data-stored-depth={lane.depth} data-episode={episode?.key ?? ""} data-axis-start={lane.axis.start.toFixed(4)} data-axis-end={lane.axis.end.toFixed(4)} onFocus={onActivate} onClick={onActivate}>
    <header><span><b>{lane.trajectoryId}</b>{reference && <small>reference</small>}</span><span className="lane-state">{["", "overview", "episodes", "events", "raw"][depth]}</span></header>
    {!trajectory ? <div className="lane-loading">loading trajectory…</div> : depth === 1 ? <ShapeStrip trajectory={trajectory} selected={selected} hover={hover} axis={lane.axis} compact={lane.band === "context"} label={lane.band === "focus" ? "Trajectory shape" : `Trajectory shape ${lane.trajectoryId}`} onSelect={onSelect} onHover={onHover} /> : depth === 2 ? <EpisodeStrip trajectory={trajectory} lane={lane} episodes={episodes} selectedEpisode={selectedEpisode} onDescend={(target) => onDescend(target)} /> : <><ShapeStrip trajectory={trajectory} selected={selected} hover={hover} axis={lane.axis} compact label="Compressed trajectory shape" onSelect={onSelect} onHover={onHover} onAscend={onAscend} />{depth === 3 && episode && <ScopedEvents trajectory={trajectory} episode={episode} selected={selected} onSelect={onSelect} />}{depth === 4 && trajectory.events[selected] && <SourceRecord event={trajectory.events[selected]} />}</>}
    {trajectory && hover !== undefined && depth === 1 && lane.band === "focus" && <aside className="skim-preview" role="status"><b>#{trajectory.events[hover].sequence} · {trajectory.events[hover].kind}</b><span>{eventText(trajectory.events[hover])}</span></aside>}
  </main>;
}

function judgesFor(trajectory: Trajectory): Array<{ label: string; value: string; eventId?: string }> {
  const judges: Array<{ label: string; value: string; eventId?: string }> = [];
  for (const event of trajectory.events.filter((item) => item.kind === "grader")) {
    const output = event.output && typeof event.output === "object" ? event.output as Record<string, unknown> : {};
    judges.push({ label: String(event.metadata?.grader ?? "grader"), value: String(output.verdict ?? output.score ?? "recorded"), eventId: event.id });
  }
  const reward = trajectory.signals?.find((signal) => signal.name === "reward"); if (reward) judges.push({ label: "reward", value: String(reward.value), eventId: reward.event_id });
  const pass = trajectory.signals?.find((signal) => signal.name === "pass"); if (pass) judges.push({ label: "verifier", value: String(pass.value), eventId: pass.event_id });
  return judges;
}

function Console({ workspace, lane, data, breadcrumb, resizeMode, dockPosition, onSelect, onHelp, onActivate }: {
  workspace: WorkspaceState; lane?: WorkspaceLane; data?: LaneData; breadcrumb: string; resizeMode: boolean; dockPosition: "right" | "bottom"; onSelect: (index: number) => void; onHelp: () => void; onActivate: () => void;
}) {
  const trajectory = data?.trajectory; const current = trajectory?.events[Math.min(lane?.selected ?? 0, Math.max(0, trajectory.events.length - 1))];
  const around = 2;
  const detailRows = trajectory && current ? trajectory.events.slice(Math.max(0, trajectory.events.indexOf(current) - around), Math.min(trajectory.events.length, trajectory.events.indexOf(current) + around + 1)) : [];
  return <section className="workspace-console" tabIndex={0} onFocus={onActivate} aria-label="Workspace console" data-resize-mode={resizeMode ? "true" : "false"} data-dock-position={dockPosition}>
    <header className="console-header"><div><h2>{trajectory?.name ?? trajectory?.id ?? "No lane selected"}</h2><p className="workspace-breadcrumb">{breadcrumb}</p></div>
      {trajectory && <div className="judge-list">{judgesFor(trajectory).map((judge, index) => <button key={`${judge.label}:${index}`} className={/false|fail/i.test(judge.value) ? "failure" : judge.label === "verifier" && /true|pass/i.test(judge.value) ? "verifier-pass" : ""} onClick={() => { const found = trajectory.events.findIndex((event) => event.id === judge.eventId); if (found >= 0) onSelect(found); }}><small>{judge.label}</small><b>{judge.value}</b></button>)}</div>}
      <div className="console-meta"><span>reference: <b data-testid="reference-name">{workspace.reference ? workspace.lanes.find((item) => item.id === workspace.reference)?.trajectoryId ?? "none" : "none"}</b></span>{resizeMode && <strong>resize mode · arrows · Esc</strong>}<button onClick={onHelp}>?</button></div>
    </header>
    <section className="detail-region" aria-label="Selected moment">{detailRows.map((event) => <button key={event.id} className={`moment ${event.id === current?.id ? "selected" : ""}`} onClick={() => onSelect(trajectory!.events.indexOf(event))}><span className="address">{event.sequence}</span><span className="kind-glyph">{glyphForKind(event.kind)}</span><span className="moment-copy"><small>{event.kind}</small><b>{eventText(event)}</b>{event.id === current?.id && <pre>{preview(eventDetail(event), 700)}</pre>}{event.id === current?.id && <em>source · {event.source?.path ?? "canonical record"}{event.source?.line ? `:${event.source.line}` : ""}</em>}</span></button>)}</section>
  </section>;
}

type DockContent = { collection: ReactNode; detail: ReactNode; lane: (id: string) => ReactNode };
const DockContentContext = createContext<DockContent | null>(null);

function WorkspacePanel({ params }: IDockviewPanelProps<{ kind: "collection" | "detail" | "lane"; laneId?: string }>) {
  const content = useContext(DockContentContext);
  if (!content) return null;
  if (params.kind === "collection") return content.collection;
  if (params.kind === "detail") return content.detail;
  return <div className="focus-slot">{content.lane(params.laneId ?? "")}</div>;
}

function MinimalTab({ params }: IDockviewPanelHeaderProps<{ label?: string }>) {
  return <span className="workspace-tab">{params.label ?? "module"}</span>;
}

const dockComponents = { workspace: WorkspacePanel };
const dockTabComponents = { minimal: MinimalTab };
const lanePanelId = (id: string) => `lane:${id}`;
const laneIdFromPanel = (id: string) => id.startsWith("lane:") ? id.slice(5) : undefined;


const KEYBAR_COLLECTION: CommandId[] = [commandIds.workspace.descend, commandIds.workspace.addLane, commandIds.view.fidelityUp, commandIds.group.search, commandIds.workspace.toggleRail, commandIds.workspace.cycleNext, commandIds.view.toggleHelp];
const KEYBAR_LANE: CommandId[] = [commandIds.trajectory.next, commandIds.trajectory.nextError, commandIds.workspace.descend, commandIds.workspace.ascend, commandIds.view.zoomIn, commandIds.workspace.nextRollout, commandIds.workspace.closeLane, commandIds.workspace.cycleNext, commandIds.view.toggleHelp];

function KeyBar({ module, selection }: { module: "collection" | "lane"; selection?: string }) {
  const ids = module === "collection" ? KEYBAR_COLLECTION : KEYBAR_LANE;
  return <footer className="keybar" aria-label="Active module keys">
    {ids.map((id) => { const command = commandDefinition(id); return <button key={id} className="keybar-chip" tabIndex={-1} onMouseDown={(event) => event.preventDefault()} onClick={() => dispatchCommand(id)}><kbd>{firstBindingLabel(id)}</kbd><span>{command.label}</span></button>; })}
    {selection && <span className="selection-address">{selection}</span>}
  </footer>;
}

export function App({ initialTrajectory, provider = daemonProvider }: { initialTrajectory?: Trajectory; provider?: ViewerProvider }) {
  useKeymapRevision();
  const [workspace, setWorkspace] = useState<WorkspaceState>(initialWorkspace);
  const workspaceRef = useRef(workspace); workspaceRef.current = workspace;
  const [browse, setBrowse] = useState<BrowseResponse>(() => fakeBrowse(initialTrajectory ?? sampleTrajectory));
  const [laneData, setLaneData] = useState<Map<string, LaneData>>(() => new Map(initialTrajectory ? [[laneId("sample", initialTrajectory.id), { trajectory: initialTrajectory, analysis: null }]] : []));
  const laneDataRef = useRef(laneData); laneDataRef.current = laneData;
  const [railFidelity, setRailFidelity] = useState(1);
  const [hover, setHover] = useState<Record<string, number | undefined>>({});
  const [help, setHelp] = useState(false); const [resizeMode, setResizeMode] = useState(false); const [moveMode, setMoveMode] = useState(false); const [error, setError] = useState("");
  const [dockReady, setDockReady] = useState(false);
  const [dockRevision, setDockRevision] = useState(0);
  const [detailPosition, setDetailPosition] = useState<"right" | "bottom">(workspace.direction === "columns" ? "bottom" : "right");
  const [presentation, setPresentation] = useState<PresentationConfig>();
  const [theme, setTheme] = useState<"light" | "dark">(() => document.documentElement.getAttribute("data-theme") === "dark" || (!document.documentElement.getAttribute("data-theme") && window.matchMedia?.("(prefers-color-scheme: dark)").matches) ? "dark" : "light");
  const [breadcrumb, setBreadcrumb] = useState(() => snapshotLabel(workspace));
  const railRef = useRef<HTMLElement>(null); const dockApiRef = useRef<DockviewApi | null>(null); const syncingDock = useRef(false);
  const lastFocus = useRef<string | undefined>(undefined);
  const tabPointerAt = useRef(0);
  const jumpList = useRef<WorkspaceState[]>([workspace]); const jumpIndex = useRef(0); const restoring = useRef(false); const openRevision = useRef(0);
  const laneDataLRU = useRef<string[]>([]);
  const pendingReplace = useRef<WorkspaceState | undefined>(undefined); const replaceFrame = useRef<number | undefined>(undefined);
  const legacyReadIntent = useRef((() => { const params = new URLSearchParams(window.location.search); return (params.get("mode") === "read" || params.get("view") === "read") && !params.get("trajectory_id"); })());

  const ordered = browse.trajectories;
  const filtered = useMemo(() => ordered.filter((row) => !workspace.railQuery || `${row.trajectory.id} ${row.source_name} ${row.case_name ?? ""} ${row.group_name ?? ""}`.toLowerCase().includes(workspace.railQuery.toLowerCase())), [ordered, workspace.railQuery]);
  const boundedRail = Math.min(workspace.railSelected, Math.max(0, filtered.length - 1)); const selectedRow = filtered[boundedRail];
  const activeLane = workspace.lanes.find((lane) => lane.id === workspace.active) ?? (workspace.active === "detail" ? workspace.lanes.find((lane) => lane.id === lastFocus.current) ?? workspace.lanes.find((lane) => lane.band === "focus") : undefined);

  const writeURL = useCallback((next: WorkspaceState, push: boolean) => {
    try { localStorage.setItem(workspaceStorageKey, JSON.stringify(next)); } catch { /* storage is optional */ }
    const state = { rlvizWorkspace: next };
    if (push) {
      if (replaceFrame.current !== undefined) cancelAnimationFrame(replaceFrame.current);
      replaceFrame.current = undefined; pendingReplace.current = undefined;
      window.history.pushState(state, "", workspaceURL(next));
      return;
    }
    pendingReplace.current = next;
    if (replaceFrame.current !== undefined) return;
    replaceFrame.current = requestAnimationFrame(() => {
      replaceFrame.current = undefined;
      const latest = pendingReplace.current; pendingReplace.current = undefined;
      if (latest) window.history.replaceState({ rlvizWorkspace: latest }, "", workspaceURL(latest));
    });
  }, []);
  const applyWorkspace = useCallback((next: WorkspaceState, snapshot = true) => {
    const normalized = normalizeWorkspace(next); if (!normalized) return;
    if (JSON.stringify(normalized) === JSON.stringify(workspaceRef.current)) return;
    workspaceRef.current = normalized; setWorkspace(normalized); setBreadcrumb(snapshotLabel(normalized)); writeURL(normalized, snapshot && !restoring.current);
    if (snapshot && !restoring.current) {
      const serialized = JSON.stringify(normalized), current = JSON.stringify(jumpList.current[jumpIndex.current]);
      if (serialized !== current) { jumpList.current = [...jumpList.current.slice(0, jumpIndex.current + 1), normalized]; jumpIndex.current = jumpList.current.length - 1; }
    }
  }, [writeURL]);
  const change = useCallback((update: (current: WorkspaceState) => WorkspaceState, snapshot = true) => applyWorkspace(update(workspaceRef.current), snapshot), [applyWorkspace]);

  const rememberLaneData = useCallback((id: string) => {
    laneDataLRU.current = [...laneDataLRU.current.filter((item) => item !== id), id];
  }, []);
  const putLaneData = useCallback((id: string, data: LaneData) => {
    rememberLaneData(id);
    setLaneData((current) => { const next = new Map(current).set(id, data); laneDataRef.current = next; return next; });
  }, [rememberLaneData]);
  const deleteLaneData = useCallback((id: string) => {
    laneDataLRU.current = laneDataLRU.current.filter((item) => item !== id);
    setLaneData((current) => { if (!current.has(id)) return current; const next = new Map(current); next.delete(id); laneDataRef.current = next; return next; });
  }, []);
  const pruneOffLaneData = useCallback(() => {
    const active = new Set(workspaceRef.current.lanes.map((lane) => lane.id));
    const offLane = laneDataLRU.current.filter((id) => laneDataRef.current.has(id) && !active.has(id));
    const evict = new Set(offLane.slice(0, Math.max(0, offLane.length - 8)));
    if (!evict.size) return;
    laneDataLRU.current = laneDataLRU.current.filter((id) => !evict.has(id));
    setLaneData((current) => { const next = new Map(current); evict.forEach((id) => next.delete(id)); laneDataRef.current = next; return next; });
  }, []);

  const ensureLaneData = useCallback(async (lane: WorkspaceLane) => {
    if (laneDataRef.current.has(lane.id)) return;
    const revision = openRevision.current;
    try {
      const loaded = lane.sourceId === "sample" ? { trajectory: initialTrajectory ?? sampleTrajectory, presentation: undefined } : await provider.loadTrajectory(lane.sourceId, lane.trajectoryId);
      if (revision !== openRevision.current || !workspaceRef.current.lanes.some((item) => item.id === lane.id)) return;
      const data: LaneData = { trajectory: loaded.trajectory, analysis: null, presentation: loaded.presentation };
      if (lane.id === workspaceRef.current.active) setPresentation(loaded.presentation);
      putLaneData(lane.id, data);
      change((current) => ({ ...current, lanes: current.lanes.map((item) => item.id === lane.id && item.axis.end <= item.axis.start + 1 ? { ...item, selected: firstAnomaly(loaded.trajectory), axis: { start: loaded.trajectory.events[0]?.sequence ?? 0, end: loaded.trajectory.events.at(-1)?.sequence ?? 1 } } : item) }), false);
      if (lane.sourceId !== "sample") provider.loadAnalysis(lane.sourceId, lane.trajectoryId).then((analysis) => setLaneData((current) => { const existing = current.get(lane.id); if (!existing) return current; const next = new Map(current).set(lane.id, { ...existing, analysis }); laneDataRef.current = next; return next; })).catch(() => undefined);
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Could not load trajectory"); }
  }, [change, initialTrajectory, provider, putLaneData]);

  useEffect(() => {
    const controller = new AbortController();
    if (initialTrajectory) { setBrowse(fakeBrowse(initialTrajectory)); workspace.lanes.forEach((lane) => void ensureLaneData(lane)); return () => controller.abort(); }
    if (workspaceRef.current.lanes.length) {
      provider.loadBrowse(controller.signal).then((collection) => { setBrowse(collection); workspaceRef.current.lanes.forEach((lane) => void ensureLaneData(lane)); }).catch((reason) => { if (!controller.signal.aborted && !(reason instanceof Error && reason.name === "AbortError")) setError(reason instanceof Error ? reason.message : "Could not load viewer"); });
      return () => controller.abort();
    }
    Promise.all([provider.loadInitial(controller.signal), provider.loadBrowse(controller.signal)]).then(([loaded, collection]) => {
      setBrowse(collection); setPresentation(loaded.presentation);
      const sourceId = collection.trajectories.find((row) => row.trajectory.id === loaded.trajectory.id)?.source_id;
      if (sourceId) putLaneData(laneId(sourceId, loaded.trajectory.id), { trajectory: loaded.trajectory, analysis: null, presentation: loaded.presentation });
      if (sourceId && legacyReadIntent.current && !workspaceRef.current.lanes.length) {
        const id = laneId(sourceId, loaded.trajectory.id);
        applyWorkspace({ ...workspaceRef.current, railExpanded: false, active: id, lanes: [{ id, sourceId, trajectoryId: loaded.trajectory.id, band: "focus", selected: firstAnomaly(loaded.trajectory), depth: 1, fidelity: 3, axis: { start: loaded.trajectory.events[0]?.sequence ?? 0, end: loaded.trajectory.events.at(-1)?.sequence ?? 1 }, descentStack: [] }] }, false);
      }
      workspaceRef.current.lanes.forEach((lane) => void ensureLaneData(lane));
    }).catch((reason) => { if (!controller.signal.aborted && !(reason instanceof Error && reason.name === "AbortError")) setError(reason instanceof Error ? reason.message : "Could not load viewer"); });
    return () => controller.abort();
  }, [applyWorkspace, ensureLaneData, initialTrajectory, provider, putLaneData]);

  useEffect(() => applyPresentationTheme(presentation), [presentation]);
  useEffect(() => { if (activeLane && laneData.has(activeLane.id)) setPresentation(laneData.get(activeLane.id)?.presentation); }, [activeLane, laneData]);
  useEffect(() => { document.documentElement.setAttribute("data-theme", theme); }, [theme]);
  useEffect(() => { writeURL(workspaceRef.current, false); }, [writeURL]);
  useEffect(() => {
    const onPop = (event: PopStateEvent) => {
      const next = normalizeWorkspace((event.state as { rlvizWorkspace?: unknown } | null)?.rlvizWorkspace) ?? workspaceFromSearch(location.search) ?? legacyWorkspace(location.search);
      if (!next) return;
      const serialized = JSON.stringify(next); let found = -1; for (let index = jumpList.current.length - 1; index >= 0; index--) { if (JSON.stringify(jumpList.current[index]) === serialized) { found = index; break; } } if (found >= 0) jumpIndex.current = found;
      restoring.current = true; workspaceRef.current = next; setWorkspace(next); setBreadcrumb(snapshotLabel(next)); openRevision.current++; next.lanes.forEach((lane) => void ensureLaneData(lane)); restoring.current = false;
    };
    window.addEventListener("popstate", onPop); return () => window.removeEventListener("popstate", onPop);
  }, [ensureLaneData]);
  useEffect(() => {
    const lane = workspace.lanes.find((item) => item.id === workspace.active); if (lane?.band === "focus") lastFocus.current = lane.id;
    const panelId = workspace.active === "rail" ? "collection" : workspace.active === "detail" ? "detail" : lanePanelId(workspace.active);
    const panel = dockApiRef.current?.getPanel(panelId);
    if (panel && dockApiRef.current?.activePanel?.id !== panelId) panel.api.setActive();
    // Panel content mounts a frame or two after dockReady; retry briefly so
    // boot/reload always lands focus on the active module.
    let cancelled = false;
    const place = (attempt: number) => {
      if (cancelled) return;
      const target = workspace.active === "rail" ? railRef.current : workspace.active === "detail" ? document.querySelector<HTMLElement>(".workspace-console") : document.querySelector<HTMLElement>(`[data-lane-id="${CSS.escape(workspace.active)}"]`);
      if (target) {
        if (document.activeElement !== target && !(document.activeElement instanceof HTMLInputElement)) target.focus({ preventScroll: true });
        return;
      }
      if (attempt < 90) requestAnimationFrame(() => place(attempt + 1));
    };
    place(0);
    return () => { cancelled = true; };
  }, [dockReady, dockRevision, workspace.active, workspace.lanes.length]);

  const loadRowIntoLane = useCallback(async (row: BrowseTrajectory, add: boolean, preserve?: WorkspaceLane) => {
    const id = rowKey(row); const existing = workspaceRef.current.lanes.find((lane) => lane.id === id);
    if (existing) { change((current) => ({ ...current, active: existing.id })); return; }
    const loaded = row.source_id === "sample" ? { trajectory: initialTrajectory ?? sampleTrajectory, presentation: undefined } : await provider.loadTrajectory(row.source_id, row.trajectory.id);
    const focus = workspaceRef.current.lanes.filter((lane) => lane.band === "focus");
    const band = add && focus.length >= 2 ? "context" : "focus";
    const base: WorkspaceLane = { id, sourceId: row.source_id, trajectoryId: row.trajectory.id, band, selected: preserve?.selected ?? firstAnomaly(loaded.trajectory), depth: preserve?.depth ?? 1, fidelity: preserve?.fidelity ?? 3, axis: preserve?.axis ?? { start: loaded.trajectory.events[0]?.sequence ?? 0, end: loaded.trajectory.events.at(-1)?.sequence ?? 1 }, descentStack: preserve?.descentStack ?? [] };
    putLaneData(id, { trajectory: loaded.trajectory, analysis: null, presentation: loaded.presentation });
    change((current) => {
      // `a` piles lanes in without stealing focus from the collection;
      // opening (Enter) is the action that moves focus.
      if (add) return { ...current, lanes: [...current.lanes, base], railExpanded: current.railExpanded };
      if (!current.lanes.length) return { ...current, lanes: [base], active: id };
      if (preserve) return { ...current, lanes: current.lanes.map((lane) => lane.id === preserve.id ? { ...base, band: preserve.band } : lane), active: id, reference: current.reference === preserve.id ? undefined : current.reference };
      const replaceId = current.lanes.find((lane) => lane.id === current.active && lane.band === "focus")?.id ?? current.lanes.find((lane) => lane.id === lastFocus.current && lane.band === "focus")?.id ?? current.lanes.find((lane) => lane.band === "focus")?.id;
      if (!replaceId) return { ...current, lanes: [...current.lanes, base], active: id };
      return { ...current, lanes: current.lanes.map((lane) => lane.id === replaceId ? { ...base, band: "focus" } : lane), active: id, reference: current.reference === replaceId ? undefined : current.reference };
    });
    pruneOffLaneData();
    if (row.source_id !== "sample") provider.loadAnalysis(row.source_id, row.trajectory.id).then((analysis) => setLaneData((current) => { const data = current.get(id); if (!data) return current; const next = new Map(current).set(id, { ...data, analysis }); laneDataRef.current = next; return next; })).catch(() => undefined);
  }, [change, initialTrajectory, provider, pruneOffLaneData, putLaneData]);

  const openSelected = (add: boolean) => { if (selectedRow) void loadRowIntoLane(selectedRow, add).catch((reason) => setError(reason instanceof Error ? reason.message : "Could not load trajectory")); };
  const updateLane = useCallback((id: string, update: (lane: WorkspaceLane, data?: LaneData) => WorkspaceLane, snapshot = true) => change((current) => ({ ...current, lanes: current.lanes.map((lane) => lane.id === id ? update(lane, laneDataRef.current.get(id)) : lane) }), snapshot), [change]);
  const selectEvent = useCallback((id: string, index: number) => updateLane(id, (lane, data) => {
    if (!data) return { ...lane, selected: index };
    const min = data.trajectory.events[0]?.sequence ?? 0, max = data.trajectory.events.at(-1)?.sequence ?? 1, sequence = data.trajectory.events[index]?.sequence ?? min;
    return { ...lane, selected: index, axis: panWindowToInclude(lane.axis, sequence, min, max) };
  }, false), [updateLane]);
  const descendLane = useCallback((id: string, target?: Episode) => updateLane(id, (lane, data) => {
    if (lane.band === "context" || !data) return lane;
    const depth = effectiveDepth(lane);
    if (depth >= 4) return lane;
    const events = data.trajectory.events, episodes = episodesFor(events);
    const current = target ?? episodes[episodeIndexForEvent(episodes, lane.selected)];
    const selected = current && (lane.selected < current.startIndex || lane.selected > current.endIndex) ? current.startIndex : lane.selected;
    const sequence = events[selected]?.sequence ?? events[0]?.sequence ?? 0;
    const min = events[0]?.sequence ?? 0, max = events.at(-1)?.sequence ?? 1;
    const panned = panWindowToInclude(lane.axis, sequence, min, max);
    const axis = depth === 2 && current ? episodeWindow(panned, current, sequence) : lane.axis;
    return { ...lane, selected, depth: depth + 1, axis, descentStack: [...lane.descentStack, { depth, axis: lane.axis }] };
  }), [updateLane]);
  const ascendLane = useCallback((id: string) => updateLane(id, (lane) => {
    const depth = effectiveDepth(lane);
    if (depth <= 1) return lane;
    const snapshot = lane.descentStack.at(-1);
    return { ...lane, depth: depth - 1, axis: snapshot?.axis ?? lane.axis, descentStack: lane.descentStack.slice(0, -1) };
  }), [updateLane]);
  const moveEvent = (delta: number) => {
    if (!activeLane) return;
    const data = laneData.get(activeLane.id); if (!data) return;
    const events = data.trajectory.events, episodes = episodesFor(events), episodeIndex = episodeIndexForEvent(episodes, activeLane.selected);
    const depth = effectiveDepth(activeLane);
    if (depth === 2) {
      const target = episodes[Math.max(0, Math.min(episodes.length - 1, episodeIndex + delta))];
      if (target) selectEvent(activeLane.id, target.startIndex);
      return;
    }
    if (depth >= 3) {
      const episode = episodes[episodeIndex]; if (!episode) return;
      selectEvent(activeLane.id, Math.max(episode.startIndex, Math.min(episode.endIndex, activeLane.selected + delta)));
      return;
    }
    selectEvent(activeLane.id, Math.max(0, Math.min(events.length - 1, activeLane.selected + delta)));
  };
  const jumpEvent = (predicate: (event: TrajectoryEvent) => boolean) => { if (!activeLane) return; const events = laneData.get(activeLane.id)?.trajectory.events; if (!events) return; const next = events.findIndex((event, index) => index > activeLane.selected && predicate(event)), wrapped = events.findIndex(predicate); if (next >= 0 || wrapped >= 0) selectEvent(activeLane.id, next >= 0 ? next : wrapped); };
  const cycleZone = (delta: number) => { const zones = [...(workspaceRef.current.railExpanded ? ["rail"] : []), ...workspaceRef.current.lanes.map((lane) => lane.id), "detail"]; if (!zones.length) return; const index = zones.indexOf(workspaceRef.current.active); const active = zones[((index < 0 ? 0 : index) + delta + zones.length) % zones.length]; const panel = dockApiRef.current?.getPanel(active === "rail" ? "collection" : active === "detail" ? "detail" : lanePanelId(active)); panel?.api.setActive(); change((current) => ({ ...current, active })); };
  const sweep = (delta: number) => { if (!activeLane || !filtered.length) return; const occupied = new Set(workspaceRef.current.lanes.filter((lane) => lane.id !== activeLane.id).map((lane) => lane.id)); const candidates = filtered.filter((row) => !occupied.has(rowKey(row))); if (!candidates.length) return; const index = candidates.findIndex((row) => rowKey(row) === activeLane.id); const row = candidates[((index < 0 ? 0 : index) + delta + candidates.length) % candidates.length]; change((current) => ({ ...current, railSelected: filtered.indexOf(row) }), false); void loadRowIntoLane(row, false, activeLane); };
  const closeLane = () => { if (!activeLane) return; deleteLaneData(activeLane.id); change((current) => { const lanes = current.lanes.filter((lane) => lane.id !== activeLane.id); return { ...current, lanes, railExpanded: lanes.length ? current.railExpanded : true, active: lanes[0]?.id ?? "rail", reference: current.reference === activeLane.id ? undefined : current.reference }; }); };
  const promoteDemote = () => { if (!activeLane) return; const counterpart = workspaceRef.current.lanes.find((item) => item.id !== activeLane.id && item.band !== activeLane.band); if (!counterpart) return; const activePanel = dockApiRef.current?.getPanel(lanePanelId(activeLane.id)); const counterpartPanel = dockApiRef.current?.getPanel(lanePanelId(counterpart.id)); const promoting = activeLane.band === "context"; change((current) => ({ ...current, lanes: current.lanes.map((item) => item.id === activeLane.id ? { ...item, band: counterpart.band } : item.id === counterpart.id ? { ...item, band: activeLane.band } : item) })); requestAnimationFrame(() => { const api = dockApiRef.current; if (!api || !activePanel || !counterpartPanel) return; const promoted = promoting ? activePanel : counterpartPanel, demoted = promoting ? counterpartPanel : activePanel; const focusAnchor = workspaceRef.current.lanes.filter((lane) => lane.band === "focus" && lane.id !== (promoting ? activeLane.id : counterpart.id)).map((lane) => api.getPanel(lanePanelId(lane.id))).find(Boolean) ?? api.getPanel("collection"); if (focusAnchor) promoted.api.moveTo({ group: focusAnchor.api.group, position: "right" }); demoted.api.moveTo({ group: promoted.api.group, position: "bottom" }); persistDockLayout(api); }); };
  const jump = (delta: number) => { const nextIndex = jumpIndex.current + delta; if (nextIndex < 0 || nextIndex >= jumpList.current.length) return; jumpIndex.current = nextIndex; restoring.current = true; const next = jumpList.current[nextIndex]; applyWorkspace(next, false); next.lanes.forEach((lane) => void ensureLaneData(lane)); restoring.current = false; };
  const adjustFidelity = (delta: number) => setRailFidelity((value) => Math.max(0, Math.min(2, value + delta)));
  const adjustZoom = (factor: number | "fit", all: boolean) => change((current) => ({ ...current, lanes: current.lanes.map((lane) => { if (!all && lane.id !== current.active) return lane; const data = laneDataRef.current.get(lane.id); if (!data) return lane; const min = data.trajectory.events[0]?.sequence ?? 0, max = data.trajectory.events.at(-1)?.sequence ?? 1, sequence = data.trajectory.events[lane.selected]?.sequence ?? min; return { ...lane, axis: factor === "fit" ? { start: min, end: max } : zoomWindow(lane.axis, sequence, factor, min, max) }; }) }), false);

  const resizeNearest = (key: string) => {
    const api = dockApiRef.current;
    const singleLane = workspaceRef.current.lanes.length === 1 && workspaceRef.current.lanes[0].id === workspaceRef.current.active;
    const panel = singleLane ? api?.getPanel("detail") : api?.activePanel; const box = panel?.api.group.api.boundingBox;
    if (!panel || !box) return;
    const delta = key === "ArrowRight" || key === "ArrowDown" ? 24 : -24;
    if (singleLane || key === "ArrowLeft" || key === "ArrowRight") panel.api.group.api.setSize({ width: Math.max(120, box.width + delta) });
    else panel.api.group.api.setSize({ height: Math.max(72, box.height + delta) });
  };

  const annotateDockGeometry = useCallback((api = dockApiRef.current) => {
    if (!api) return;
    const modules = {
      rail: document.querySelector<HTMLElement>(".workspace-rail"),
      console: document.querySelector<HTMLElement>(".workspace-console"),
      focusLane: document.querySelectorAll<HTMLElement>(".lane-track.focus-lane")[1],
      focusContext: document.querySelector<HTMLElement>(".lane-track.context-lane"),
    };
    const sashes = [...document.querySelectorAll<HTMLElement>(".rlviz-dockview .dv-sash:not(.dv-disabled)")];
    sashes.forEach((sash) => sash.removeAttribute("data-seam"));
    const used = new Set<HTMLElement>();
    for (const [name, module] of Object.entries(modules)) {
      if (!module) continue; const box = module.getBoundingClientRect();
      let best: HTMLElement | undefined, distance = Infinity;
      for (const sash of sashes) {
        if (used.has(sash)) continue; const candidate = sash.getBoundingClientRect();
        const vertical = candidate.height > candidate.width;
        const next = vertical ? Math.min(Math.abs(candidate.left - box.left), Math.abs(candidate.left - box.right)) : Math.min(Math.abs(candidate.top - box.top), Math.abs(candidate.top - box.bottom));
        if (next < distance) { best = sash; distance = next; }
      }
      if (best) { best.dataset.seam = name; used.add(best); }
    }
    const detail = api.getPanel("detail")?.api.group.api.boundingBox;
    const center = workspaceRef.current.lanes.map((lane) => api.getPanel(lanePanelId(lane.id))?.api.group.api.boundingBox).find(Boolean);
    if (detail && center) setDetailPosition(detail.top >= center.top + center.height * 0.5 ? "bottom" : "right");
  }, []);

  const persistDockLayout = useCallback((api = dockApiRef.current) => {
    if (!api || syncingDock.current || !api.totalPanels) return;
    const layout = api.toJSON();
    change((current) => ({ ...current, layout }), false);
  }, [change]);

  const addDefaultPanel = useCallback((api: DockviewApi, id: string, kind: "collection" | "detail" | "lane", lane?: WorkspaceLane) => {
    const panels = api.panels;
    const firstLane = workspaceRef.current.lanes.map((item) => api.getPanel(lanePanelId(item.id))).find(Boolean);
    const center = firstLane ?? panels.find((item) => item.id !== "collection" && item.id !== "detail") ?? panels[0];
    if (kind === "collection") {
      api.addPanel({ id, component: "workspace", tabComponent: "minimal", renderer: "always", title: "Collection", params: { kind, label: "collection" }, initialWidth: Math.max(220, api.width * 0.24), ...(center ? { position: { referencePanel: center, direction: "left" as const } } : {}) });
      return;
    }
    if (kind === "detail") {
      const bottom = workspaceRef.current.direction === "columns";
      api.addPanel({ id, component: "workspace", tabComponent: "minimal", renderer: "always", title: "Detail", params: { kind, label: "detail" }, initialWidth: Math.max(260, api.width * 0.28), initialHeight: Math.max(150, api.height * 0.28), ...(center ? { position: { referencePanel: center, direction: bottom ? "below" as const : "right" as const } } : {}) });
      setDetailPosition(bottom ? "bottom" : "right");
      return;
    }
    const focusPanels = workspaceRef.current.lanes.filter((item) => item.band === "focus" && item.id !== lane?.id).map((item) => api.getPanel(lanePanelId(item.id))).filter(Boolean);
    const contextPanel = workspaceRef.current.lanes.filter((item) => item.band === "context" && item.id !== lane?.id).map((item) => api.getPanel(lanePanelId(item.id))).find(Boolean);
    const reference = lane?.band === "context" ? contextPanel ?? focusPanels[0] ?? center : focusPanels.at(-1) ?? center;
    const direction = lane?.band === "context" ? (contextPanel ? "within" : "below") : "right";
    api.addPanel({ id, component: "workspace", tabComponent: "minimal", renderer: "always", title: lane?.trajectoryId ?? "Lane", params: { kind, laneId: lane?.id, label: lane?.trajectoryId ?? "lane" }, initialHeight: lane?.band === "context" ? 92 : undefined, ...(reference ? { position: { referencePanel: reference, direction } } : {}) });
  }, []);

  const reconcileDock = useCallback((api: DockviewApi) => {
    let changed = false;
    syncingDock.current = true;

    try {
      const desired = new Set(["detail", ...(workspaceRef.current.railExpanded ? ["collection"] : []), ...workspaceRef.current.lanes.map((lane) => lanePanelId(lane.id))]);
      api.panels.filter((panel) => !desired.has(panel.id)).forEach((panel) => { changed = true; api.removePanel(panel); });
      workspaceRef.current.lanes.forEach((lane) => { if (!api.getPanel(lanePanelId(lane.id))) { changed = true; addDefaultPanel(api, lanePanelId(lane.id), "lane", lane); } });
      if (workspaceRef.current.railExpanded && !api.getPanel("collection")) { changed = true; addDefaultPanel(api, "collection", "collection"); }
      if (!api.getPanel("detail")) { changed = true; addDefaultPanel(api, "detail", "detail"); }
    } finally {
      syncingDock.current = false;
    }
    if (changed) setDockRevision((value) => value + 1);
    requestAnimationFrame(() => { annotateDockGeometry(api); persistDockLayout(api); });
  }, [addDefaultPanel, annotateDockGeometry, persistDockLayout]);

  const onDockReady = useCallback((event: DockviewReadyEvent) => {
    const api = event.api; dockApiRef.current = api; syncingDock.current = true;
    if (workspaceRef.current.layout) {
      try { api.fromJSON(workspaceRef.current.layout); }
      catch { api.clear(); }
    }
    syncingDock.current = false; reconcileDock(api);
    // Deterministic boot/reload focus: place it the moment the dock exists,
    // not after a frame-count guess.
    requestAnimationFrame(() => requestAnimationFrame(() => {
      if (document.activeElement && document.activeElement !== document.body) return;
      const active = workspaceRef.current.active;
      const target = active === "rail" ? railRef.current : active === "detail" ? document.querySelector<HTMLElement>(".workspace-console") : document.querySelector<HTMLElement>(`[data-lane-id="${CSS.escape(active)}"]`);
      target?.focus({ preventScroll: true });
    }));
    api.onDidActivePanelChange(({ panel }) => {
      // Modules already activate themselves on focus/click; the only signal
      // this event uniquely carries is a TAB-HEADER interaction. Programmatic
      // setActive/addPanel echoes must not write state — they fight keyboard
      // cycling (Tab to detail was being bounced back by the echo).
      if (syncingDock.current || !panel) return;
      if (Date.now() - tabPointerAt.current > 400) return;
      const id = laneIdFromPanel(panel.id);
      const active = panel.id === "collection" ? "rail" : panel.id === "detail" ? "detail" : id;
      if (active) change((current) => ({ ...current, active }), false);
    });
    const onTabPointer = (event: PointerEvent) => {
      if (event.target instanceof Element && event.target.closest(".dv-tabs-and-actions-container, .dv-tab")) tabPointerAt.current = Date.now();
    };
    document.addEventListener("pointerdown", onTabPointer, true);
    api.onDidLayoutChange(() => {
      annotateDockGeometry(api); persistDockLayout(api);
      // Dockview re-parents module DOM on moves/mounts, which detaches the
      // focused element and drops keyboard focus to <body>. Restore it to the
      // active module so keyboard users never lose their place.
      requestAnimationFrame(() => {
        if (document.activeElement && document.activeElement !== document.body) return;
        const active = workspaceRef.current.active;
        const target = active === "rail" ? railRef.current : active === "detail" ? document.querySelector<HTMLElement>(".workspace-console") : document.querySelector<HTMLElement>(`[data-lane-id="${CSS.escape(active)}"]`);
        target?.focus({ preventScroll: true });
      });
    });
    document.querySelectorAll(".dv-live-region, .dv-live-region-assertive").forEach((node) => node.removeAttribute("role"));
    setDockReady(true);
  }, [annotateDockGeometry, change, persistDockLayout, reconcileDock]);

  useEffect(() => { if (dockApiRef.current) reconcileDock(dockApiRef.current); }, [reconcileDock, workspace.lanes, workspace.railExpanded]);

  const dockDetail = useCallback((position: "right" | "bottom") => {
    const api = dockApiRef.current, detail = api?.getPanel("detail");
    const center = workspaceRef.current.lanes.map((lane) => api?.getPanel(lanePanelId(lane.id))).find(Boolean) ?? api?.getPanel("collection");
    if (!detail || !center || detail.id === center.id) return;
    detail.api.moveTo({ group: center.api.group, position });
    setDetailPosition(position);
    requestAnimationFrame(() => persistDockLayout(api));
  }, [persistDockLayout]);

  const moveActiveModule = useCallback((key: string) => {
    const api = dockApiRef.current, panel = api?.activePanel; if (!api || !panel) return;
    const center = workspaceRef.current.lanes.map((lane) => api.getPanel(lanePanelId(lane.id))).find((candidate) => candidate && candidate.id !== panel.id) ?? api.panels.find((candidate) => candidate.id !== panel.id && candidate.id !== "collection" && candidate.id !== "detail") ?? api.panels.find((candidate) => candidate.id !== panel.id);
    if (!center) return;
    const position: Position = key === "ArrowLeft" ? "left" : key === "ArrowRight" ? "right" : key === "ArrowDown" ? "bottom" : "center";
    panel.api.moveTo({ group: center.api.group, position });
    if (panel.id === "detail") setDetailPosition(position === "bottom" ? "bottom" : "right");
    requestAnimationFrame(() => persistDockLayout(api));
  }, [persistDockLayout]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (help || event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement) return;
      if (event.ctrlKey && !event.metaKey && event.key.toLowerCase() === "m") { event.preventDefault(); setMoveMode(true); return; }
      if (moveMode) {
        if (event.key === "Escape") { event.preventDefault(); event.stopImmediatePropagation(); setMoveMode(false); return; }
        if (["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)) { event.preventDefault(); moveActiveModule(event.key); }
        return;
      }
      if (event.altKey && ["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)) {
        event.preventDefault();
        const direction = event.key.slice(5).toLowerCase() as GroupNavigationDirection;
        const api = dockApiRef.current, group = api?.activePanel?.api.group;
        const adjacent = api && group ? api.adjacentGroupInDirection(group, direction) : undefined;
        adjacent?.activePanel?.api.setActive();
      }
    };
    window.addEventListener("keydown", onKey, true); return () => window.removeEventListener("keydown", onKey, true);
  }, [help, moveActiveModule, moveMode]);

  useCommands("workspace", {
    [commandIds.workspace.toggleRail]: () => change((current) => { const railExpanded = !current.railExpanded; return { ...current, railExpanded, active: !railExpanded && current.active === "rail" && current.lanes.length ? current.lanes[0].id : current.active }; }),
    [commandIds.workspace.addLane]: () => workspaceRef.current.active === "rail" ? openSelected(true) : false,
    [commandIds.workspace.closeLane]: () => activeLane ? closeLane() : false,
    [commandIds.workspace.cycleNext]: () => cycleZone(1), [commandIds.workspace.cyclePrevious]: () => cycleZone(-1),
    [commandIds.workspace.nextRollout]: () => activeLane ? sweep(1) : false, [commandIds.workspace.previousRollout]: () => activeLane ? sweep(-1) : false,
    [commandIds.workspace.promoteDemote]: () => activeLane ? promoteDemote() : false,
    [commandIds.workspace.pinReference]: () => activeLane ? change((current) => ({ ...current, reference: current.reference === activeLane.id ? undefined : activeLane.id })) : false,
    [commandIds.workspace.directionRows]: () => { change((current) => ({ ...current, direction: "rows" })); dockDetail("right"); }, [commandIds.workspace.directionColumns]: () => { change((current) => ({ ...current, direction: "columns" })); dockDetail("bottom"); },
    [commandIds.workspace.descend]: () => { if (!activeLane) { openSelected(false); return; } if (activeLane.band === "context") return false; descendLane(activeLane.id); },
    // Esc is structural (ascend, then close the lane, keeping current rail
    // state); history rewind is exclusively Ctrl+o, so backing out of a lane
    // never restores a stale rail selection.
    [commandIds.workspace.ascend]: () => { if (resizeMode) { setResizeMode(false); return; } if (!activeLane) return false; if (effectiveDepth(activeLane) > 1) ascendLane(activeLane.id); else closeLane(); },
    [commandIds.workspace.jumpBack]: () => jump(-1), [commandIds.workspace.jumpForward]: () => jump(1), [commandIds.workspace.resizeMode]: () => setResizeMode(true),
    [commandIds.view.fidelityUp]: () => adjustFidelity(1), [commandIds.view.fidelityDown]: () => adjustFidelity(-1),
    [commandIds.view.zoomIn]: () => activeLane ? adjustZoom(2, false) : false, [commandIds.view.zoomOut]: () => activeLane ? adjustZoom(0.5, false) : false, [commandIds.view.zoomFit]: () => activeLane ? adjustZoom("fit", false) : false,
    [commandIds.view.zoomInAll]: () => activeLane ? adjustZoom(2, true) : false, [commandIds.view.zoomOutAll]: () => activeLane ? adjustZoom(0.5, true) : false, [commandIds.view.zoomFitAll]: () => activeLane ? adjustZoom("fit", true) : false,
    [commandIds.view.toggleHelp]: () => setHelp(true),
  }, !help);
  useCommands("trajectory", {
    [commandIds.trajectory.next]: () => activeLane ? moveEvent(1) : workspaceRef.current.active === "rail" ? change((current) => ({ ...current, railSelected: Math.min(filtered.length - 1, current.railSelected + 1) }), false) : false,
    [commandIds.trajectory.previous]: () => activeLane ? moveEvent(-1) : workspaceRef.current.active === "rail" ? change((current) => ({ ...current, railSelected: Math.max(0, current.railSelected - 1) }), false) : false,
    [commandIds.trajectory.nextError]: () => jumpEvent((event) => event.kind === "error"), [commandIds.trajectory.nextContext]: () => jumpEvent((event) => !!event.context || !!event.alignment_key?.startsWith("context:")),
    [commandIds.trajectory.nextReward]: () => jumpEvent((event) => event.kind === "reward" || event.kind === "grader"), [commandIds.trajectory.nextFinding]: () => { if (!activeLane) return false; const ids = new Set((laneData.get(activeLane.id)?.analysis?.analysis.findings ?? []).flatMap((finding) => finding.event_ids ?? [])); jumpEvent((event) => ids.has(event.id)); },
  }, !help);

  useEffect(() => {
    if (!resizeMode) return;
    const onKey = (event: KeyboardEvent) => {
      if (help || event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement) return;
      if (["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)) { event.preventDefault(); resizeNearest(event.key); }
    };
    window.addEventListener("keydown", onKey); return () => window.removeEventListener("keydown", onKey);
  }, [help, resizeMode]);

  const dockContent: DockContent = {
    collection: <Rail root={railRef} rows={filtered} workspace={{ ...workspace, railSelected: boundedRail }} fidelity={railFidelity} onActivate={() => change((current) => ({ ...current, active: "rail" }))} onSelect={(index) => change((current) => ({ ...current, railSelected: index, active: "rail" }))} onOpen={() => openSelected(false)} onAdd={() => openSelected(true)} onQuery={(railQuery) => change((current) => { const next = ordered.filter((row) => !railQuery || `${row.trajectory.id} ${row.source_name} ${row.case_name ?? ""} ${row.group_name ?? ""}`.toLowerCase().includes(railQuery.toLowerCase())); const kept = selectedRow ? next.findIndex((row) => rowKey(row) === rowKey(selectedRow)) : -1; return { ...current, railQuery, railSelected: kept >= 0 ? kept : 0 }; })} />,
    lane: (id) => { const lane = workspace.lanes.find((item) => item.id === id); return lane ? <LaneTrack lane={lane} data={laneData.get(lane.id)} active={workspace.active === lane.id} reference={workspace.reference === lane.id} hover={hover[lane.id]} onActivate={() => change((current) => ({ ...current, active: lane.id }))} onSelect={(value) => selectEvent(lane.id, value)} onHover={(value) => setHover((current) => ({ ...current, [lane.id]: value }))} onDescend={(episode) => descendLane(lane.id, episode)} onAscend={() => ascendLane(lane.id)} /> : null; },
    detail: <Console workspace={workspace} lane={activeLane} data={activeLane ? laneData.get(activeLane.id) : undefined} breadcrumb={breadcrumb} resizeMode={resizeMode} dockPosition={detailPosition} onSelect={(index) => activeLane && selectEvent(activeLane.id, index)} onHelp={() => setHelp(true)} onActivate={() => change((current) => ({ ...current, active: "detail" }))} />,
  };
  return <ViewerProviderContext.Provider value={provider}><DockContentContext.Provider value={dockContent}><div className={`instrument-shell workspace-rack rail-${workspace.railExpanded ? "open" : "closed"}`} data-filter={workspace.railQuery} data-direction={workspace.direction} data-active-zone={workspace.active} data-move-mode={moveMode ? "true" : "false"}>
    <button className="theme-toggle" aria-label={`Switch to ${theme === "light" ? "dark" : "light"} theme`} onClick={() => setTheme((current) => current === "light" ? "dark" : "light")}>{theme}</button>
    {error && <div className="instrument-error" role="alert">{error}</div>}{presentation?.notices?.map((notice) => <div className="presentation-notice" role="status" key={notice}>{notice}</div>)}
    <section className="workspace-stage" aria-label="Trajectory stage"><DockviewReact className="rlviz-dockview" components={dockComponents} tabComponents={dockTabComponents} defaultTabComponent={MinimalTab} onReady={onDockReady} disableFloatingGroups noPanelsOverlay="emptyGroup" keyboardNavigation={false} announcements={false} />{!workspace.lanes.length && <div className="empty-stage"><b>Open a rollout from the rail.</b><small>Enter replaces · A adds · t toggles the rail</small></div>}</section>
    <KeyBar module={workspace.active === "rail" ? "collection" : "lane"} selection={activeLane && laneData.get(activeLane.id)?.trajectory.events[activeLane.selected] ? `#${laneData.get(activeLane.id)!.trajectory.events[Math.min(activeLane.selected, laneData.get(activeLane.id)!.trajectory.events.length - 1)].sequence}` : undefined} />
    {help && <HelpOverlay onClose={() => setHelp(false)} />}
  </div></DockContentContext.Provider></ViewerProviderContext.Provider>;
}
