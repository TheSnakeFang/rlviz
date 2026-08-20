import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { sampleTrajectory } from "./sample";
import { MobileTrajectoryReader } from "./MobileTrajectoryReader";
import { laneId } from "./workspace";

const lane = {
  id: laneId("sample", sampleTrajectory.id), sourceId: "sample", trajectoryId: sampleTrajectory.id,
  band: "focus" as const, selected: 0, depth: 1, fidelity: 1,
  axis: { start: 0, end: 10 }, descentStack: [],
};

describe("mobile trajectory reader", () => {
  it("starts with an outcome-first summary and exposes one primary story action", () => {
    const onSurface = vi.fn();
    render(<MobileTrajectoryReader lane={lane} trajectory={sampleTrajectory} surface="summary" details={<div>Raw detail</div>} onBrowse={vi.fn()} onSurface={onSurface} onSelect={vi.fn()} />);
    expect(screen.getByRole("main", { name: "Mobile trajectory reader" })).toHaveAttribute("data-surface", "summary");
    expect(screen.getByRole("region", { name: "Trajectory summary" })).toHaveTextContent("OutcomeFail");
    fireEvent.click(screen.getByRole("button", { name: "Read the story" }));
    expect(onSurface).toHaveBeenCalledWith("story");
  });

  it("renders semantic story and evidence surfaces and routes evidence into details", () => {
    const onSurface = vi.fn();
    const onSelect = vi.fn();
    const { rerender } = render(<MobileTrajectoryReader lane={lane} trajectory={sampleTrajectory} surface="story" details={<div>Raw detail</div>} onBrowse={vi.fn()} onSurface={onSurface} onSelect={onSelect} />);
    expect(screen.getByRole("region", { name: "Trajectory transcript" })).toBeInTheDocument();
    rerender(<MobileTrajectoryReader lane={lane} trajectory={sampleTrajectory} surface="evidence" details={<div>Raw detail</div>} onBrowse={vi.fn()} onSurface={onSurface} onSelect={onSelect} />);
    expect(screen.getByRole("region", { name: "Trajectory outcome" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /fail/i }));
    expect(onSelect).toHaveBeenCalled();
    expect(onSurface).toHaveBeenCalledWith("details");
  });
});
