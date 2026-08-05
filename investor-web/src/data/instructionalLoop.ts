import type { InstructionalLoopNode } from "./types";

/**
 * Static instructional-loop diagram nodes.
 * Status mirrors product-state inventory; LLM is never the center node.
 */
export type { InstructionalLoopNode };

export const instructionalLoop: InstructionalLoopNode[] = [
  {
    id: "adult-direction",
    order: 1,
    name: "Adult direction",
    summary:
      "Parent or teacher sets curriculum, worldview guardrails, priorities, and non-negotiable expectations.",
    status: "IN_DEVELOPMENT",
    capabilityId: "family-admin",
  },
  {
    id: "planner",
    order: 2,
    name: "Planner",
    summary:
      "Chooses the next standards-linked task from prerequisites, weak evidence, projects, and adult overrides.",
    status: "IN_DEVELOPMENT",
    capabilityId: "curriculum-planning",
  },
  {
    id: "specialist-tutor",
    order: 3,
    name: "Specialist tutor",
    summary:
      "Diagnoses misconceptions and scaffolds within a subject using the learner's recent evidence.",
    status: "IN_DEVELOPMENT",
    capabilityId: "subject-tutoring",
  },
  {
    id: "habit-checks",
    order: 4,
    name: "Habit checks",
    summary:
      "Requires reasoning, units, evidence, grammar, and precise vocabulary before work is accepted.",
    status: "PLANNED",
    capabilityId: "habit-guards",
  },
  {
    id: "revision-loop",
    order: 5,
    name: "Revision loop",
    summary:
      "Student repairs the first consequential error rather than receiving passive correction or credit for incomplete work.",
    status: "IN_DEVELOPMENT",
    capabilityId: "assessment-evaluation",
  },
  {
    id: "evidence-record",
    order: 6,
    name: "Evidence record",
    summary:
      "Stores the artifact, conditions, and evaluator behind every mastery claim on the longitudinal standards map.",
    status: "IN_DEVELOPMENT",
    capabilityId: "standards-mastery-record",
  },
  {
    id: "transfer-project",
    order: 7,
    name: "Transfer / project",
    summary:
      "Reuses the skill in another subject or off-screen project so mastery is not tied to one item format.",
    status: "PLANNED",
    capabilityId: "project-orchestration",
  },
  {
    id: "adult-exception-review",
    order: 8,
    name: "Adult exception review",
    summary:
      "Adult reviews escalations, overrides plans or mastery decisions, and approves abandoned avenues.",
    status: "IN_DEVELOPMENT",
    capabilityId: "family-admin",
  },
];
