import type { StatusLabel } from "./types";

/**
 * Deterministic product demo content.
 * Live surfaces use synthetic artifacts; target flow is scripted, not a live model call.
 */

export interface LiveSurface {
  id: string;
  name: string;
  status: Extract<StatusLabel, "LIVE">;
  summary: string;
  capabilityId: string;
  /** Synthetic capture description — no real student data. */
  capture: string;
  /** What an investor should take away. */
  proves: string;
  anchor: string;
}

export interface DemoStep {
  id: string;
  order: number;
  title: string;
  actor: "Parent" | "Primer" | "Student" | "System";
  /** Visible scripted dialogue / UI copy. */
  script: string;
  /** What mechanism this step demonstrates. */
  mechanism: string;
  /** Status is always TARGET EXPERIENCE until live. */
  status: "TARGET_EXPERIENCE";
  /** Synthetic learner label only. */
  artifact?: string;
}

export const DEMO_UPDATED = "2026-08-04";
export const SYNTHETIC_LEARNER = "Alex M. (synthetic grade-6 learner)";

export const liveSurfaces: LiveSurface[] = [
  {
    id: "command-teacher",
    name: "command-teacher",
    status: "LIVE",
    summary:
      "Linux command tutor that diagnoses mistakes, requires reasoning, and records session evidence.",
    capabilityId: "command-teacher",
    capture:
      "Synthetic session: learner mistypes `ls -la /home`, tutor asks what the flags do before accepting a corrected command. No real student transcript is shown.",
    proves: "Specialist diagnosis, reasoning prompt, and session evidence logging exist today.",
    anchor: "command-teacher",
  },
  {
    id: "primer-tv",
    name: "PrimerTV",
    status: "LIVE",
    summary:
      "Curated instructional media channel with schedule, pairing, and family viewing controls.",
    capabilityId: "primer-tv",
    capture:
      "Synthetic schedule card: paired clip on ratios with parent-visible watch window and off-screen follow-up prompt. Media is curated, not open-web.",
    proves: "Media can be scheduled as instruction rather than passive consumption.",
    anchor: "primer-tv",
  },
  {
    id: "lms-core",
    name: "LMS core / admin",
    status: "LIVE",
    summary:
      "Learner, resource, standards, and instruction-log infrastructure under the longitudinal record.",
    capabilityId: "lms-core",
    capture:
      "Synthetic admin view: organization → learner → instruction log entries with timestamps. Fields use redacted placeholders only.",
    proves: "The longitudinal record and admin SPA foundation are running.",
    anchor: "lms-core",
  },
  {
    id: "ultralogical",
    name: "Ultralogical platform",
    status: "LIVE",
    summary:
      "Multi-agent execution platform used to run specialist tutors and instructional workflows.",
    capabilityId: "ultralogical",
    capture:
      "Platform diagram: planner → specialist tutor → habit check → evidence writer. Agents are labeled by role; no production credentials or internal topology are exposed.",
    proves: "Specialist workflows can be orchestrated as durable agent jobs, not one-off chat turns.",
    anchor: "ultralogical",
  },
];

export const targetExperienceSteps: DemoStep[] = [
  {
    id: "syllabus-ingestion",
    order: 1,
    title: "Syllabus ingestion",
    actor: "Parent",
    script:
      "Parent pastes a grade-6 unit outline: “Ratios and proportional relationships — unit rates, tables, and graphs. Target: independent practice by Friday.”",
    mechanism: "Adult direction and curriculum planning entry point",
    status: "TARGET_EXPERIENCE",
    artifact: "Synthetic syllabus paste · Unit 3 ratios",
  },
  {
    id: "prerequisite-diagnosis",
    order: 2,
    title: "Prerequisite diagnosis",
    actor: "Primer",
    script:
      "Primer maps the unit to standards and flags weak prerequisites: equivalent fractions and unit-rate language from prior evidence. Next task: 12-minute diagnostic, not a full re-teach.",
    mechanism: "Standards map + longitudinal record before new instruction",
    status: "TARGET_EXPERIENCE",
    artifact: "Standards: 6.RP.A.1, 6.RP.A.2 · prerequisites flagged",
  },
  {
    id: "student-attempt",
    order: 3,
    title: "Student attempt",
    actor: "Student",
    script:
      "Alex M. (synthetic) answers: “A recipe uses 3 cups flour for 4 cups milk. Flour per cup of milk is 3/4.” Work is partially correct but units are dropped on the follow-up table.",
    mechanism: "Visible student work before correction",
    status: "TARGET_EXPERIENCE",
    artifact: "Attempt #1 · synthetic response",
  },
  {
    id: "reasoning-prompt",
    order: 4,
    title: "Reasoning prompt",
    actor: "Primer",
    script:
      "Tutor: “Explain how you know 3/4 cup flour pairs with 1 cup milk. What stays constant when the table scales to 8 cups of milk?”",
    mechanism: "Specialist attention requires reasoning, not answer key matching",
    status: "TARGET_EXPERIENCE",
  },
  {
    id: "habit-correction",
    order: 5,
    title: "Habit correction",
    actor: "System",
    script:
      "Habit guard blocks progression: missing unit labels on the scaled table. Message: “State the unit with every quantity before this attempt can be accepted.”",
    mechanism: "Cross-subject habit enforcement (units / precision)",
    status: "TARGET_EXPERIENCE",
    artifact: "Guard: units-required · status blocked",
  },
  {
    id: "revision",
    order: 6,
    title: "Revision",
    actor: "Student",
    script:
      "Alex revises the table with units on every cell and restates the constant rate in a sentence. Tutor accepts the revision and asks one transfer item with different cover story.",
    mechanism: "Correction loop before credit",
    status: "TARGET_EXPERIENCE",
    artifact: "Attempt #2 · revised with units",
  },
  {
    id: "mastery-evidence",
    order: 7,
    title: "Mastery evidence",
    actor: "Primer",
    script:
      "Evidence event written: standard 6.RP.A.2, artifact = revised table + explanation, evaluator = specialist tutor v0 (target), confidence = provisional. Parent sees the artifact, not only a green check.",
    mechanism: "Auditable mastery record with artifact and conditions",
    status: "TARGET_EXPERIENCE",
    artifact: "Evidence event · provisional mastery",
  },
  {
    id: "transfer-check",
    order: 8,
    title: "Scheduled transfer check",
    actor: "System",
    script:
      "Planner schedules a transfer check in five days inside a science mixture task so the rate skill is reused off the original worksheet format.",
    mechanism: "Transfer / spaced revisit — mastery not tied to one item format",
    status: "TARGET_EXPERIENCE",
    artifact: "Transfer check · Day +5 · science mixture context",
  },
];

/** Full accessible transcript of the target experience, synchronized with steps. */
export function buildTargetTranscript(): string {
  return targetExperienceSteps
    .map(
      (step) =>
        `Step ${step.order} — ${step.title} (${step.actor})\n${step.script}\nMechanism: ${step.mechanism}`,
    )
    .join("\n\n");
}
