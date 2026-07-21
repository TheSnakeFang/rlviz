import { describe, expect, it } from "vitest";
import { defaultSeams, emptyWorkspace, laneId, legacyWorkspace, normalizeWorkspace, serializeWorkspace, workspaceFromSearch, workspaceURL } from "./workspace";

describe("workspace arrangements", () => {
  it("round-trips lanes, per-lane view state, and seam ratios", () => {
    const workspace = emptyWorkspace();
    workspace.lanes = [{ id: laneId("source", "one"), sourceId: "source", trajectoryId: "one", band: "focus", selected: 4, depth: 3, fidelity: 5, axis: { start: 10, end: 20 } }];
    workspace.active = workspace.lanes[0].id;
    workspace.seams.rail = 0.31;
    const encoded = serializeWorkspace(workspace);
    expect(workspaceFromSearch(`?workspace=${encodeURIComponent(encoded)}`)).toEqual(workspace);
    expect(workspaceURL(workspace, { pathname: "/view", search: "?trajectory=old&mode=read", hash: "#token=x" } as Location)).toContain("workspace=");
  });

  it("bounds untrusted ratios and keeps at most two focus lanes", () => {
    const normalized = normalizeWorkspace({ ...emptyWorkspace(), seams: { rail: 9, focusContext: -1, focusLane: 9, console: 0 }, lanes: ["a", "b", "c"].map((trajectoryId) => ({ sourceId: "s", trajectoryId, band: "focus", axis: { start: 0, end: 1 } })) })!;
    expect(normalized.lanes.filter((lane) => lane.band === "focus")).toHaveLength(2);
    expect(normalized.lanes[2].band).toBe("context");
    expect(normalized.seams).not.toEqual(defaultSeams);
    expect(normalized.seams.rail).toBe(0.42);
  });

  it("maps legacy read and compare URLs into arrangements", () => {
    const read = legacyWorkspace("?trajectory=source&trajectory_id=one&mode=read")!;
    expect(read.lanes.map((lane) => lane.trajectoryId)).toEqual(["one"]);
    const compare = legacyWorkspace("?trajectory=source&left=one&right=two&view=compare")!;
    expect(compare.lanes.map((lane) => lane.trajectoryId)).toEqual(["one", "two"]);
    expect(compare.reference).toBe(compare.lanes[0].id);
  });
});
