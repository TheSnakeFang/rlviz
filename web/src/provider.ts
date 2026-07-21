import { createContext, useContext } from "react";
import {
  loadAnalysis,
  loadBrowse,
  loadComparison,
  loadIndexedTrajectory,
  loadArtifactContent as loadDaemonArtifactContent,
  loadTrajectory,
} from "./api";
import type {
  AnalysisResponse,
  BrowseResponse,
  ComparisonResponse,
} from "./types";
import type { LoadResult } from "./api";

/** Data boundary shared by the loopback daemon and the static browser app. */
export interface ViewerProvider {
  loadInitial(signal?: AbortSignal): Promise<LoadResult>;
  loadBrowse(signal?: AbortSignal): Promise<BrowseResponse>;
  loadTrajectory(sourceId: string, trajectoryId: string, signal?: AbortSignal): Promise<LoadResult>;
  loadAnalysis(sourceId: string, trajectoryId: string, signal?: AbortSignal): Promise<AnalysisResponse>;
  loadComparison(sourceId: string, left: string, right: string, signal?: AbortSignal): Promise<ComparisonResponse>;
  loadArtifactContent(sourceId: string, trajectoryId: string, artifactId: string, signal?: AbortSignal): Promise<Uint8Array>;
}

export const daemonProvider: ViewerProvider = {
  loadInitial: loadTrajectory,
  loadBrowse,
  loadTrajectory: loadIndexedTrajectory,
  loadAnalysis,
  loadComparison,
  async loadArtifactContent(sourceId, trajectoryId, artifactId, signal) {
    return new Uint8Array(await loadDaemonArtifactContent(sourceId, trajectoryId, artifactId, signal));
  },
};

export const ViewerProviderContext = createContext<ViewerProvider>(daemonProvider);
export function useViewerProvider(): ViewerProvider { return useContext(ViewerProviderContext); }
