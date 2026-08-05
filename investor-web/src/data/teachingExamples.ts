import type { TeachingExample } from "./types";

/**
 * Concrete teaching-method examples for the How it teaches section.
 * Mechanism description only — not efficacy claims.
 */
export type { TeachingExample };

export const teachingExamples: TeachingExample[] = [
  {
    id: "asks-before-tells",
    order: 1,
    title: "Asks before it tells",
    principle: "Prompt, hint, decompose, then model only when needed.",
    example:
      "A sixth-grader misapplies the distributive property. The tutor asks what the expression means, requests a smaller analogous case, and only then shows a worked model if the student remains stuck.",
    status: "IN_DEVELOPMENT",
  },
  {
    id: "correction-before-progression",
    order: 2,
    title: "Correction before progression",
    principle: "A correct final answer with broken reasoning does not clear the standard.",
    example:
      "The student reaches the right fraction answer by canceling digits. Primer rejects the method, requires a rewritten explanation with common factors named, and schedules a delayed check.",
    status: "IN_DEVELOPMENT",
  },
  {
    id: "cross-subject-habits",
    order: 3,
    title: "Reasoning, units, evidence, grammar, vocabulary",
    principle: "Quality expectations travel with the learner across subjects.",
    example:
      "A science lab write-up without units, a history claim without a source, and a math answer without shown work all open the same correction loop until the habit is repaired.",
    status: "PLANNED",
  },
  {
    id: "transfer-across-subjects",
    order: 4,
    title: "Transfer across subjects",
    principle: "Mastery is tested when the skill appears in a new context.",
    example:
      "After ratios clear in mathematics, the same skill reappears in a recipe scale-up, a map distance conversion, and a budget line — the student must name the connection.",
    status: "PLANNED",
  },
  {
    id: "off-screen-projects",
    order: 5,
    title: "Off-screen projects",
    principle: "Real work supplies consequences a worksheet cannot.",
    example:
      "A garden experiment or CAD build is planned in phases. Primer checks calculations and explanations; the physical result — out-of-square joints, exceeded budget, failed prediction — is part of the evidence.",
    status: "PLANNED",
  },
  {
    id: "evidence-behind-mastery",
    order: 6,
    title: "Evidence behind mastery",
    principle: "Every mastery claim points to an artifact and an evaluation rule.",
    example:
      "Clearing a standard stores the corrected explanation, assessment item, project photo, or oral defense — with date, conditions, and who or what evaluated it — not only a course percentage.",
    status: "IN_DEVELOPMENT",
  },
];
