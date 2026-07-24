import { describe, expect, it } from "vitest";
import { emptyWorkspace, laneId } from "./workspace";
import { responsivePrimaryTarget, responsiveWorkspaceTargets, workspaceViewportMode } from "./responsiveWorkspace";

describe("responsive workspace projection", () => {
  it("uses stable mobile, compact, and full breakpoints", () => {
    expect(workspaceViewportMode(390)).toBe("mobile");
    expect(workspaceViewportMode(719)).toBe("mobile");
    expect(workspaceViewportMode(720)).toBe("compact");
    expect(workspaceViewportMode(1199)).toBe("compact");
    expect(workspaceViewportMode(1200)).toBe("full");
  });

  it("starts a compact empty workspace in Guide", () => {
    expect(responsivePrimaryTarget(emptyWorkspace())).toBe("guide");
  });

  it("prefers the first rollout when Guide is closed", () => {
    const workspace = emptyWorkspace();
    const id = laneId("source", "rollout-01");
    workspace.guideOpen = false;
    workspace.lanes = [{ id, sourceId: "source", trajectoryId: "rollout-01", band: "focus", selected: 0, depth: 1, fidelity: 1, axis: { start: 0, end: 10 }, descentStack: [] }];
    expect(responsivePrimaryTarget(workspace)).toBe(id);
  });

  it("keeps every logical module reachable in a deterministic order", () => {
    const workspace = emptyWorkspace();
    const first = laneId("source", "rollout-01");
    const second = laneId("source", "rollout-02");
    workspace.lanes = [
      { id: first, sourceId: "source", trajectoryId: "rollout-01", band: "focus", selected: 0, depth: 1, fidelity: 1, axis: { start: 0, end: 10 }, descentStack: [] },
      { id: second, sourceId: "source", trajectoryId: "rollout-02", band: "focus", selected: 0, depth: 1, fidelity: 1, axis: { start: 0, end: 10 }, descentStack: [] },
    ];
    workspace.details = [second];
    expect(responsiveWorkspaceTargets(workspace)).toEqual(["rail", "guide", first, second, `detail:${second}`, "detail", "settings"]);
  });
});
