import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ArtifactPanel, InlineArtifacts } from "./ArtifactPanel";
import type { TrajectoryArtifact } from "./types";

const artifacts: TrajectoryArtifact[] = [
  { id: "json", trajectory_id: "trajectory", event_id: "event-1", name: "result.json", media_type: "application/json", json: { ok: true } },
  { id: "diff", trajectory_id: "trajectory", name: "change.diff", media_type: "text/x-diff", text: "- old\n+ new" },
];

describe("artifact rendering", () => {
  afterEach(() => { vi.unstubAllGlobals(); });

  it("renders inline JSON and diff as inert text", () => {
    const { container, rerender } = render(<ArtifactPanel artifacts={artifacts} sourceId="source" trajectoryId="trajectory" selectedId="json" onSelect={() => {}} />);
    expect(screen.getByText(/"ok": true/)).toBeInTheDocument();
    expect(container.querySelector("script")).toBeNull();
    rerender(<ArtifactPanel artifacts={artifacts} sourceId="source" trajectoryId="trajectory" selectedId="diff" onSelect={() => {}} />);
    expect(screen.getByText(/- old/)).toHaveClass("diff");
  });

  it("selects artifacts and loads authenticated path text", async () => {
    window.history.replaceState({}, "", "/#token=secret");
    const fetch = vi.fn(async () => new Response("log output", { status: 200, headers: { "Content-Type": "text/plain" } }));
    vi.stubGlobal("fetch", fetch);
    const pathArtifact: TrajectoryArtifact = { id: "log", trajectory_id: "trajectory", event_id: "event-2", name: "run.log", media_type: "text/x-log", path: "run.log" };
    const select = vi.fn();
    render(<ArtifactPanel artifacts={[...artifacts, pathArtifact]} sourceId="source" trajectoryId="trajectory" selectedId="log" onSelect={select} />);
    expect(fetch).not.toHaveBeenCalled();
    expect(screen.getByText("run.log", { selector: ".artifact-consent code" })).toBeInTheDocument();
    expect(screen.getByText(/only if you trust the trace/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Load preview" }));
    expect(await screen.findByText("log output")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /result.json/ }));
    expect(select).toHaveBeenCalledWith(artifacts[0]);
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining("artifact_id=log"), expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer secret" }) }));
  });

  it("displays safe fetched images without interpreting markup", async () => {
    const create = vi.fn(() => "blob:safe-image");
    const revoke = vi.fn();
    vi.stubGlobal("URL", { ...URL, createObjectURL: create, revokeObjectURL: revoke });
    vi.stubGlobal("fetch", vi.fn(async () => new Response(new Uint8Array([137, 80, 78, 71]), { status: 200, headers: { "Content-Type": "image/png" } })));
    const image: TrajectoryArtifact = { id: "image", trajectory_id: "trajectory", name: "frame.png", media_type: "image/png", path: "frame.png" };
    const { unmount } = render(<ArtifactPanel artifacts={[image]} sourceId="source" trajectoryId="trajectory" selectedId="image" onSelect={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: "Load preview" }));
    expect(await screen.findByRole("img", { name: "frame.png" })).toHaveAttribute("src", "blob:safe-image");
    unmount();
    await waitFor(() => expect(revoke).toHaveBeenCalledWith("blob:safe-image"));
  });

  it("resets path authorization when artifact, trajectory, or source changes", async () => {
    const fetch = vi.fn(async () => new Response("safe", { status: 200 }));
    vi.stubGlobal("fetch", fetch);
    const one: TrajectoryArtifact = { id: "one", trajectory_id: "trajectory", media_type: "text/plain", path: "one.txt" };
    const two: TrajectoryArtifact = { id: "two", trajectory_id: "trajectory", media_type: "text/plain", path: "two.txt" };
    const { rerender } = render(<ArtifactPanel artifacts={[one, two]} sourceId="source" trajectoryId="trajectory" selectedId="one" onSelect={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: "Load preview" }));
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    rerender(<ArtifactPanel artifacts={[one, two]} sourceId="source" trajectoryId="trajectory" selectedId="two" onSelect={() => {}} />);
    expect(screen.getByRole("button", { name: "Load preview" })).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(1);
    rerender(<ArtifactPanel artifacts={[one, two]} sourceId="source" trajectoryId="trajectory" selectedId="one" onSelect={() => {}} />);
    expect(screen.getByRole("button", { name: "Load preview" })).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(1);
    rerender(<ArtifactPanel artifacts={[one]} sourceId="other-source" trajectoryId="other-trajectory" selectedId="one" onSelect={() => {}} />);
    expect(screen.getByRole("button", { name: "Load preview" })).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("shows relevant inline artifacts in comparisons only", () => {
    render(<InlineArtifacts artifacts={artifacts} eventId="event-1" label="left" />);
    expect(screen.getByText("left artifacts")).toBeInTheDocument();
    expect(screen.getByText("result.json")).toBeInTheDocument();
    expect(screen.getByText("change.diff")).toBeInTheDocument();
  });
});
