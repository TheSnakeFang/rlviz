import { useEffect, useState } from "react";
import type { WorkspaceState } from "./workspace";

export type WorkspaceViewportMode = "full" | "compact" | "mobile";

export const mobileWorkspaceBreakpoint = 720;
export const fullWorkspaceBreakpoint = 1200;

export function workspaceViewportMode(width: number): WorkspaceViewportMode {
  if (width < mobileWorkspaceBreakpoint) return "mobile";
  if (width < fullWorkspaceBreakpoint) return "compact";
  return "full";
}

export function responsiveWorkspaceTargets(workspace: WorkspaceState): string[] {
  return [
    "rail",
    "guide",
    ...workspace.lanes.map((lane) => lane.id),
    ...workspace.details.map((laneId) => `detail:${laneId}`),
    ...(workspace.lanes.length ? ["detail"] : []),
    "settings",
  ];
}

export function responsivePrimaryTarget(workspace: WorkspaceState): string {
  if (workspace.active !== "rail") return workspace.active;
  if (workspace.guideOpen) return "guide";
  return workspace.lanes[0]?.id ?? (workspace.settingsOpen ? "settings" : "rail");
}

export function useWorkspaceViewportMode(): WorkspaceViewportMode {
  const [mode, setMode] = useState<WorkspaceViewportMode>(() => workspaceViewportMode(window.innerWidth));
  useEffect(() => {
    let frame: number | undefined;
    const update = () => {
      if (frame !== undefined) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        frame = undefined;
        setMode(workspaceViewportMode(window.innerWidth));
      });
    };
    window.addEventListener("resize", update);
    return () => {
      window.removeEventListener("resize", update);
      if (frame !== undefined) cancelAnimationFrame(frame);
    };
  }, []);
  return mode;
}
