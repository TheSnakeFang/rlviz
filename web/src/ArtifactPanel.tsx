import { useEffect, useMemo, useState } from "react";
import { bindingLabel, commandIds, useKeymapRevision } from "./commands";
import { json } from "./format";
import { useViewerProvider } from "./provider";
import type { TrajectoryArtifact } from "./types";

const imageTypes = new Set(["image/png", "image/jpeg", "image/gif", "image/webp"]);
const textTypes = new Set(["text/plain", "text/x-log", "text/x-diff", "text/x-patch", "application/json", "application/x-ndjson"]);

function baseMediaType(value: string): string {
  return value.split(";", 1)[0].trim().toLowerCase();
}

function artifactName(artifact: TrajectoryArtifact): string {
  return artifact.name || artifact.path?.split(/[\\/]/).at(-1) || artifact.id;
}

function inlineContent(artifact: TrajectoryArtifact): string | null {
  if (artifact.json !== undefined) return json(artifact.json);
  if (artifact.text !== undefined) {
    if (baseMediaType(artifact.media_type) === "application/json") {
      try { return json(JSON.parse(artifact.text)); } catch { return artifact.text; }
    }
    return artifact.text;
  }
  return null;
}

function Preview({ artifact, content, imageURL }: { artifact: TrajectoryArtifact; content?: string; imageURL?: string }) {
  const inline = inlineContent(artifact);
  const displayed = content ?? inline;
  const diff = ["text/x-diff", "text/x-patch"].includes(baseMediaType(artifact.media_type)) || /\.(diff|patch)$/i.test(artifact.name ?? artifact.path ?? "");
  if (imageURL) return <img className="artifact-image" src={imageURL} alt={artifactName(artifact)} />;
  if (displayed !== null && displayed !== undefined) return <pre className={`artifact-content ${diff ? "diff" : ""}`}>{displayed}</pre>;
  return <div className="artifact-state">No inline preview</div>;
}

export function InlineArtifacts({ artifacts, eventId, label }: { artifacts?: TrajectoryArtifact[]; eventId?: string; label: string }) {
  const relevant = (artifacts ?? []).filter((artifact) => !artifact.path && (!artifact.event_id || artifact.event_id === eventId));
  if (!relevant.length) return null;
  return <section className="comparison-artifacts"><h3>{label} artifacts</h3>{relevant.map((artifact) => <article key={artifact.id}><header><strong>{artifactName(artifact)}</strong><span>{artifact.media_type}</span></header><Preview artifact={artifact} /></article>)}</section>;
}

function SelectedArtifactPreview({ artifact, sourceId, trajectoryId }: { artifact: TrajectoryArtifact; sourceId: string; trajectoryId: string }) {
  const provider = useViewerProvider();
  const [content, setContent] = useState<string>();
  const [imageURL, setImageURL] = useState<string>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [authorized, setAuthorized] = useState(false);

  useEffect(() => {
    if (!artifact.path || !sourceId || !authorized) return;
    const mediaType = baseMediaType(artifact.media_type);
    if (!imageTypes.has(mediaType) && !textTypes.has(mediaType)) {
      setError(`Preview blocked for ${artifact.media_type}`);
      return;
    }
    const controller = new AbortController();
    let objectURL = "";
    setLoading(true);
    provider.loadArtifactContent(sourceId, trajectoryId, artifact.id, controller.signal).then((bytes) => {
      if (imageTypes.has(mediaType)) {
        const blob = new Blob([bytes], { type: mediaType });
        objectURL = URL.createObjectURL(blob);
        setImageURL(objectURL);
      } else {
        const text = new TextDecoder().decode(bytes);
        if (mediaType === "application/json") {
          try { setContent(json(JSON.parse(text))); } catch { setContent(text); }
        } else setContent(text);
      }
    }).catch((reason) => { if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : "Artifact preview failed"); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => { controller.abort(); if (objectURL) URL.revokeObjectURL(objectURL); };
  }, [artifact, authorized, provider, sourceId, trajectoryId]);

  return <div className="artifact-preview"><div className="artifact-meta"><strong>{artifactName(artifact)}</strong><span>{artifact.path ? "path-backed" : "inline"}{artifact.sha256 ? " · sha256 verified on read" : ""}</span></div>
    {artifact.path && !authorized ? <div className="artifact-consent"><strong>Local file preview</strong><code>{artifact.path}</code><p>This artifact path is relative to the trace. Load it only if you trust the trace and its files.</p><button onClick={() => setAuthorized(true)}>Load preview</button></div>
      : loading ? <div className="artifact-state">Loading artifact…</div> : error ? <div className="artifact-state error">{error}</div> : <Preview artifact={artifact} content={content} imageURL={imageURL} />}
  </div>;
}

export function ArtifactPanel({ artifacts, sourceId, trajectoryId, selectedId, onSelect, label = "Artifacts" }: {
  artifacts: TrajectoryArtifact[];
  sourceId: string;
  trajectoryId: string;
  selectedId: string;
  onSelect: (artifact: TrajectoryArtifact) => void;
  label?: string;
}) {
  useKeymapRevision();
  const selected = useMemo(() => artifacts.find((artifact) => artifact.id === selectedId) ?? artifacts[0], [artifacts, selectedId]);
  if (!artifacts.length || !selected) return null;
  return <section className="artifact-panel" aria-label={label}>
    <header><span>{label}</span><small>{artifacts.length} · <kbd>{bindingLabel(commandIds.trajectory.nextArtifact)}</kbd> next</small></header>
    <div className="artifact-tabs">{artifacts.map((artifact) => <button key={artifact.id} className={artifact.id === selected.id ? "active" : ""} onClick={() => onSelect(artifact)} title={artifact.id}><strong>{artifactName(artifact)}</strong><small>{artifact.media_type}</small></button>)}</div>
    <SelectedArtifactPreview key={`${sourceId}\u0000${trajectoryId}\u0000${selected.id}`} artifact={selected} sourceId={sourceId} trajectoryId={trajectoryId} />
  </section>;
}
