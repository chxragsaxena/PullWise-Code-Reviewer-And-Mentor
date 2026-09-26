import type { ReviewStatus } from "../api/types";
import { PROGRESS_STEPS, STATUS_LABEL } from "../lib/status";
import "./StatusStepper.css";

type StepState = "done" | "active" | "pending";

function stepState(step: (typeof PROGRESS_STEPS)[number], current: ReviewStatus): StepState {
  if (current === "completed") return "done";
  const currentIndex = PROGRESS_STEPS.findIndex((s) => s === current);
  const index = PROGRESS_STEPS.indexOf(step);
  if (index < currentIndex) return "done";
  return index === currentIndex ? "active" : "pending";
}

/** The review pipeline as a vertical list of steps. */
export function StatusStepper({ current }: { current: ReviewStatus }) {
  return (
    <ol className="stepper" aria-label="Review progress">
      {PROGRESS_STEPS.map((step) => {
        const state = stepState(step, current);
        return (
          <li
            key={step}
            className={`stepper__step stepper__step--${state}`}
            aria-current={state === "active" ? "step" : undefined}
          >
            <span className="stepper__marker" aria-hidden="true">
              {state === "done" ? "✓" : ""}
            </span>
            <span className="stepper__label">{STATUS_LABEL[step]}</span>
            <span className="visually-hidden">{state === "done" ? "(done)" : state === "active" ? "(in progress)" : "(waiting)"}</span>
          </li>
        );
      })}
    </ol>
  );
}
