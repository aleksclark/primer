import type { BeachheadJob, ProblemPoint } from "./types";

/**
 * Family/classroom comparison points for the Problem section.
 * Founder Abeka experience is labeled illustrative, not market proof.
 */
export type { BeachheadJob, ProblemPoint };

export const problemPoints: ProblemPoint[] = [
  {
    id: "package-schedules",
    side: "package",
    title: "Curriculum packages schedule",
    body: "A package can assign the next page and an LMS can collect it. Neither adapts when the first wrong step appears or when mastery is gamed by completion.",
  },
  {
    id: "classroom-attention",
    side: "classroom",
    title: "Classroom attention is discontinuous",
    body: "A skilled teacher knows how to diagnose one learner. Continuous diagnosis, insistence, plan change, and delayed check across every learner and subject does not scale to a full classroom.",
  },
  {
    id: "adult-burden",
    side: "adult-burden",
    title: "The adult carries the loop",
    body: "Parent or teacher must diagnose the mistake, enforce a complete explanation, change tomorrow's task, revisit the skill later, and keep the record — every day, across subjects.",
  },
  {
    id: "founder-abeka",
    side: "founder-example",
    title: "Founder example · Abeka (illustrative)",
    body: "Using a rigorous all-subject homeschool package still required more than ten parent hours each week and made mastery difficult to distinguish from completion. Labeled founder experience — not market proof.",
  },
];

export const beachheadJobs: BeachheadJob[] = [
  {
    id: "homeschool",
    title: "Homeschool",
    body: "Preserve parent authority while reducing routine planning, enforcement, grading, and record-keeping.",
  },
  {
    id: "remediation",
    title: "School supplement",
    body: "Ingest the school's syllabus, identify missing prerequisites, and focus instruction until progress appears on independent assessments and school work.",
  },
];
