import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ComparisonView } from "./ComparisonView";
import type { ComparisonResponse, TrajectoryEvent } from "./types";

const event = (id: string, sequence: number, kind = "tool", name = id): TrajectoryEvent => ({ id, sequence, kind, title: name, raw: { id, payload: `${id}-raw` } });

const comparison: ComparisonResponse = {
  left: { trajectory: { id: "run-left" }, events: [event("shared-0", 0), event("shared-1", 1), event("left-choice", 2), event("rejoin", 3)] },
  right: {
    trajectory: { id: "run-right" },
    events: [event("shared-0-r", 0), event("shared-1-r", 1), event("right-choice", 2, "environment_action"), event("retry", 3), event("rejoin-r", 4)],
    artifacts: [{ id: "right-log", trajectory_id: "run-right", event_id: "right-choice", name: "choice.log", media_type: "text/x-log", text: "right-side artifact output" }],
  },
  alignment: {
    common_behavioral_prefix: 2, first_meaningful_divergence: 2, later_realignment: 4,
    steps: [
      { operation: "match", left_index: 0, right_index: 0, meaningful: false },
      { operation: "match", left_index: 1, right_index: 1, meaningful: false },
      { operation: "replace", left_index: 2, right_index: 2, meaningful: true },
      { operation: "insert", right_index: 3, meaningful: true },
      { operation: "match", left_index: 3, right_index: 4, meaningful: false },
    ],
  },
  differences: {
    reward: { left: 0, right: 1, changed: true }, status: { left: "failed", right: "complete", changed: true },
    termination: { left: "error", right: "answer", changed: true }, event_count: { left: 4, right: 5, delta: 1 },
  },
};

describe("trajectory comparison", () => {
  it("renders divergence, later realignment, operations, metrics, and raw payloads", () => {
    render(<ComparisonView comparison={comparison} onClose={() => {}} />);
    expect(screen.getByText("First meaningful divergence")).toBeInTheDocument();
    expect(screen.getByText("Later behavioral realignment")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Alignment step 3: replace" })).toHaveClass("selected");
    expect(screen.getByText(/left-choice-raw/)).toBeInTheDocument();
    expect(screen.getByText(/right-choice-raw/)).toBeInTheDocument();
    expect(screen.getByText("right-side artifact output")).toBeInTheDocument();
    expect(screen.getByText("REWARD")).toBeInTheDocument();
  });

  it("navigates steps, changes, divergence, and returns to the group", () => {
    const close = vi.fn();
    render(<ComparisonView comparison={comparison} onClose={close} />);
    fireEvent.keyDown(window, { key: "n" });
    expect(screen.getByRole("button", { name: "Alignment step 4: insert" })).toHaveClass("selected");
    fireEvent.keyDown(window, { key: "j" });
    expect(screen.getByRole("button", { name: "Alignment step 5: match" })).toHaveClass("selected");
    fireEvent.keyDown(window, { key: "d" });
    expect(screen.getByRole("button", { name: "Alignment step 3: replace" })).toHaveClass("selected");
    fireEvent.keyDown(window, { key: "Escape" });
    expect(close).toHaveBeenCalledOnce();
  });

  it("restores a valid step and reports keyboard navigation", () => {
    const onStepChange = vi.fn();
    render(<ComparisonView comparison={comparison} initialStep={4} onStepChange={onStepChange} onClose={() => {}} />);
    expect(screen.getByRole("button", { name: "Alignment step 5: match" })).toHaveClass("selected");
    fireEvent.keyDown(window, { key: "k" });
    expect(onStepChange).toHaveBeenLastCalledWith(3);
  });

  it("compresses a long shared prefix", () => {
    const steps = Array.from({ length: 6 }, (_, index) => ({ operation: index === 5 ? "replace" as const : "match" as const, left_index: index, right_index: index, meaningful: index === 5 }));
    const events = steps.map((_, index) => event(`event-${index}`, index));
    render(<ComparisonView comparison={{ ...comparison, left: { ...comparison.left, events }, right: { ...comparison.right, events }, alignment: { steps, common_behavioral_prefix: 5, first_meaningful_divergence: 5 } }} onClose={() => {}} />);
    expect(screen.getByText("4 aligned prefix events compressed")).toBeInTheDocument();
    expect(screen.getByText("5 shared behavioral anchors")).toBeInTheDocument();
  });
});
