import { useCallback, useEffect, useRef, useState } from "react";
import type { MutableRefObject } from "react";
import type { LoadResult } from "./api";
import type { ViewerProvider } from "./provider";
import type { AnalysisResponse, PresentationConfig, Trajectory } from "./types";
import type { WorkspaceLane, WorkspaceState } from "./workspace";
import { laneId } from "./workspace";
import { firstAnomaly } from "./instrument";
import { sampleTrajectory } from "./sample";

export type LaneData = {
  trajectory: Trajectory;
  analysis: AnalysisResponse | null;
  presentation?: PresentationConfig;
};

type RequestEntry = { generation: number; controller: AbortController };

function aborted(reason: unknown): boolean {
  return reason instanceof Error && reason.name === "AbortError";
}

export function useLaneDataLoader({
  provider,
  initialTrajectory,
  workspace,
  workspaceRef,
  change,
  onError,
  onPresentation,
}: {
  provider: ViewerProvider;
  initialTrajectory?: Trajectory;
  workspace: WorkspaceState;
  workspaceRef: MutableRefObject<WorkspaceState>;
  change: (update: (current: WorkspaceState) => WorkspaceState, snapshot?: boolean) => void;
  onError: (message: string) => void;
  onPresentation: (presentation?: PresentationConfig) => void;
}) {
  const [laneData, setLaneData] = useState<Map<string, LaneData>>(() => new Map(
    initialTrajectory ? [[laneId("sample", initialTrajectory.id), { trajectory: initialTrajectory, analysis: null }]] : [],
  ));
  const laneDataRef = useRef(laneData);
  laneDataRef.current = laneData;
  const laneDataLRU = useRef<string[]>([]);
  const laneRequests = useRef(new Map<string, RequestEntry>());
  const analysisRequests = useRef(new Map<string, RequestEntry>());
  const slotRequests = useRef(new Map<string, RequestEntry>());
  const nextGeneration = useRef(0);

  const rememberLaneData = useCallback((id: string) => {
    laneDataLRU.current = [...laneDataLRU.current.filter((item) => item !== id), id];
  }, []);

  const putLaneData = useCallback((id: string, data: LaneData) => {
    rememberLaneData(id);
    setLaneData((current) => {
      const next = new Map(current).set(id, data);
      laneDataRef.current = next;
      return next;
    });
  }, [rememberLaneData]);

  const begin = useCallback((requests: MutableRefObject<Map<string, RequestEntry>>, key: string): RequestEntry => {
    requests.current.get(key)?.controller.abort();
    const entry = { generation: ++nextGeneration.current, controller: new AbortController() };
    requests.current.set(key, entry);
    return entry;
  }, []);

  const current = useCallback((requests: MutableRefObject<Map<string, RequestEntry>>, key: string, entry: RequestEntry): boolean => {
    return requests.current.get(key) === entry && !entry.controller.signal.aborted;
  }, []);

  const loadAnalysisForLane = useCallback(async (id: string, sourceId: string, trajectoryId: string) => {
    if (sourceId === "sample") return;
    const entry = begin(analysisRequests, id);
    try {
      const analysis = await provider.loadAnalysis(sourceId, trajectoryId, entry.controller.signal);
      if (!current(analysisRequests, id, entry) || !workspaceRef.current.lanes.some((lane) => lane.id === id)) return;
      setLaneData((values) => {
        const existing = values.get(id);
        if (!existing) return values;
        const next = new Map(values).set(id, { ...existing, analysis });
        laneDataRef.current = next;
        return next;
      });
    } catch (reason) {
      if (!aborted(reason)) return;
    } finally {
      if (analysisRequests.current.get(id) === entry) analysisRequests.current.delete(id);
    }
  }, [begin, current, provider, workspaceRef]);

  const ensureLaneData = useCallback(async (lane: WorkspaceLane) => {
    if (laneDataRef.current.has(lane.id) || laneRequests.current.has(lane.id)) return;
    const entry = begin(laneRequests, lane.id);
    try {
      const loaded = lane.sourceId === "sample"
        ? { trajectory: initialTrajectory ?? sampleTrajectory, presentation: undefined }
        : await provider.loadTrajectory(lane.sourceId, lane.trajectoryId, entry.controller.signal);
      if (!loaded.trajectory || !current(laneRequests, lane.id, entry) || !workspaceRef.current.lanes.some((item) => item.id === lane.id)) return;
      const data: LaneData = { trajectory: loaded.trajectory, analysis: null, presentation: loaded.presentation };
      if (lane.id === workspaceRef.current.active) onPresentation(loaded.presentation);
      putLaneData(lane.id, data);
      change((state) => ({
        ...state,
        lanes: state.lanes.map((item) => item.id === lane.id && item.axis.end <= item.axis.start + 1 ? {
          ...item,
          selected: firstAnomaly(loaded.trajectory),
          axis: { start: loaded.trajectory.events[0]?.sequence ?? 0, end: loaded.trajectory.events.at(-1)?.sequence ?? 1 },
        } : item),
      }), false);
      void loadAnalysisForLane(lane.id, lane.sourceId, lane.trajectoryId);
    } catch (reason) {
      if (!aborted(reason)) onError(reason instanceof Error ? reason.message : "Could not load trajectory");
    } finally {
      if (laneRequests.current.get(lane.id) === entry) laneRequests.current.delete(lane.id);
    }
  }, [begin, change, current, initialTrajectory, loadAnalysisForLane, onError, onPresentation, provider, putLaneData, workspaceRef]);

  const loadForSlot = useCallback(async (slot: string, sourceId: string, trajectoryId: string): Promise<LoadResult | undefined> => {
    const entry = begin(slotRequests, slot);
    try {
      const loaded = sourceId === "sample"
        ? { trajectory: initialTrajectory ?? sampleTrajectory, presentation: undefined, isSample: true }
        : await provider.loadTrajectory(sourceId, trajectoryId, entry.controller.signal);
      return current(slotRequests, slot, entry) ? loaded : undefined;
    } catch (reason) {
      if (aborted(reason)) return undefined;
      throw reason;
    } finally {
      if (slotRequests.current.get(slot) === entry) slotRequests.current.delete(slot);
    }
  }, [begin, current, initialTrajectory, provider]);

  const deleteLaneData = useCallback((id: string) => {
    laneRequests.current.get(id)?.controller.abort();
    analysisRequests.current.get(id)?.controller.abort();
    slotRequests.current.get(id)?.controller.abort();
    laneRequests.current.delete(id);
    analysisRequests.current.delete(id);
    slotRequests.current.delete(id);
    laneDataLRU.current = laneDataLRU.current.filter((item) => item !== id);
    setLaneData((values) => {
      if (!values.has(id)) return values;
      const next = new Map(values);
      next.delete(id);
      laneDataRef.current = next;
      return next;
    });
  }, []);

  const pruneOffLaneData = useCallback(() => {
    const active = new Set(workspaceRef.current.lanes.map((lane) => lane.id));
    const offLane = laneDataLRU.current.filter((id) => laneDataRef.current.has(id) && !active.has(id));
    const evict = new Set(offLane.slice(0, Math.max(0, offLane.length - 8)));
    if (!evict.size) return;
    laneDataLRU.current = laneDataLRU.current.filter((id) => !evict.has(id));
    setLaneData((values) => {
      const next = new Map(values);
      evict.forEach((id) => next.delete(id));
      laneDataRef.current = next;
      return next;
    });
  }, [workspaceRef]);

  useEffect(() => {
    const active = new Set(workspace.lanes.map((lane) => lane.id));
    laneRequests.current.forEach((entry, id) => { if (!active.has(id)) { entry.controller.abort(); laneRequests.current.delete(id); } });
    analysisRequests.current.forEach((entry, id) => { if (!active.has(id)) { entry.controller.abort(); analysisRequests.current.delete(id); } });
    workspace.lanes.forEach((lane) => void ensureLaneData(lane));
  }, [ensureLaneData, workspace.lanes]);

  useEffect(() => () => {
    [laneRequests, analysisRequests, slotRequests].forEach((requests) => requests.current.forEach((entry) => entry.controller.abort()));
  }, []);

  return {
    laneData,
    laneDataRef,
    putLaneData,
    deleteLaneData,
    pruneOffLaneData,
    loadForSlot,
    loadAnalysisForLane,
  };
}
