import { useCallback, useEffect, useMemo, useReducer, useRef } from "react";
import type { MutableRefObject } from "react";
import {
  emptyWorkspace,
  legacyWorkspace,
  normalizeWorkspace,
  snapshotLabel,
  workspaceFromSearch,
  workspaceStorageKey,
  workspaceTopologyKey,
  workspaceURL,
} from "./workspace";
import type { WorkspaceState } from "./workspace";

type WorkspaceAction = { type: "replace"; workspace: WorkspaceState };

function workspaceReducer(_state: WorkspaceState, action: WorkspaceAction): WorkspaceState {
  return action.type === "replace" ? action.workspace : _state;
}

function savedWorkspace(): WorkspaceState | undefined {
  try {
    const value = localStorage.getItem(workspaceStorageKey);
    return value ? normalizeWorkspace(JSON.parse(value)) : undefined;
  } catch { return undefined; }
}

function initialWorkspace(): WorkspaceState {
  const saved = savedWorkspace();
  const linked = workspaceFromSearch(window.location.search) ?? legacyWorkspace(window.location.search);
  if (!linked) return saved ?? emptyWorkspace();
  // Reuse exact geometry only for the same local panel topology. A shared link
  // from another arrangement or viewport receives a deterministic default.
  if (saved?.layout && workspaceTopologyKey(saved) === workspaceTopologyKey(linked)) return { ...linked, layout: saved.layout };
  return linked;
}

export type WorkspaceController = {
  workspace: WorkspaceState;
  workspaceRef: MutableRefObject<WorkspaceState>;
  breadcrumb: string;
  applyWorkspace: (next: WorkspaceState, snapshot?: boolean) => void;
  change: (update: (current: WorkspaceState) => WorkspaceState, snapshot?: boolean) => void;
  jump: (delta: number) => void;
};

export function useWorkspaceController(): WorkspaceController {
  const [workspace, dispatch] = useReducer(workspaceReducer, undefined, initialWorkspace);
  const workspaceRef = useRef(workspace);
  workspaceRef.current = workspace;
  const restoring = useRef(false);
  const jumpList = useRef<WorkspaceState[]>([workspace]);
  const jumpIndex = useRef(0);
  const pendingReplace = useRef<WorkspaceState | undefined>(undefined);
  const replaceFrame = useRef<number | undefined>(undefined);

  const writeURL = useCallback((next: WorkspaceState, push: boolean) => {
    try { localStorage.setItem(workspaceStorageKey, JSON.stringify(next)); } catch { /* persistence is optional */ }
    const state = { rlvizWorkspace: next };
    if (push) {
      if (replaceFrame.current !== undefined) cancelAnimationFrame(replaceFrame.current);
      replaceFrame.current = undefined;
      pendingReplace.current = undefined;
      window.history.pushState(state, "", workspaceURL(next));
      return;
    }
    pendingReplace.current = next;
    if (replaceFrame.current !== undefined) return;
    replaceFrame.current = requestAnimationFrame(() => {
      replaceFrame.current = undefined;
      const latest = pendingReplace.current;
      pendingReplace.current = undefined;
      if (latest) window.history.replaceState({ rlvizWorkspace: latest }, "", workspaceURL(latest));
    });
  }, []);

  const applyWorkspace = useCallback((next: WorkspaceState, snapshot = true) => {
    const normalized = normalizeWorkspace(next);
    if (!normalized || JSON.stringify(normalized) === JSON.stringify(workspaceRef.current)) return;
    workspaceRef.current = normalized;
    dispatch({ type: "replace", workspace: normalized });
    writeURL(normalized, snapshot && !restoring.current);
    if (snapshot && !restoring.current) {
      const serialized = JSON.stringify(normalized);
      const current = JSON.stringify(jumpList.current[jumpIndex.current]);
      if (serialized !== current) {
        jumpList.current = [...jumpList.current.slice(0, jumpIndex.current + 1), normalized];
        jumpIndex.current = jumpList.current.length - 1;
      }
    }
  }, [writeURL]);

  const change = useCallback((update: (current: WorkspaceState) => WorkspaceState, snapshot = true) => {
    applyWorkspace(update(workspaceRef.current), snapshot);
  }, [applyWorkspace]);

  const jump = useCallback((delta: number) => {
    const nextIndex = jumpIndex.current + delta;
    if (nextIndex < 0 || nextIndex >= jumpList.current.length) return;
    jumpIndex.current = nextIndex;
    restoring.current = true;
    applyWorkspace(jumpList.current[nextIndex], false);
    restoring.current = false;
  }, [applyWorkspace]);

  useEffect(() => {
    writeURL(workspaceRef.current, false);
    return () => {
      if (replaceFrame.current !== undefined) cancelAnimationFrame(replaceFrame.current);
    };
  }, [writeURL]);

  useEffect(() => {
    const onPop = (event: PopStateEvent) => {
      const next = normalizeWorkspace((event.state as { rlvizWorkspace?: unknown } | null)?.rlvizWorkspace)
        ?? workspaceFromSearch(location.search)
        ?? legacyWorkspace(location.search);
      if (!next) return;
      const serialized = JSON.stringify(next);
      for (let index = jumpList.current.length - 1; index >= 0; index--) {
        if (JSON.stringify(jumpList.current[index]) === serialized) {
          jumpIndex.current = index;
          break;
        }
      }
      restoring.current = true;
      workspaceRef.current = next;
      dispatch({ type: "replace", workspace: next });
      restoring.current = false;
    };
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);

  return {
    workspace,
    workspaceRef,
    breadcrumb: useMemo(() => snapshotLabel(workspace), [workspace]),
    applyWorkspace,
    change,
    jump,
  };
}
