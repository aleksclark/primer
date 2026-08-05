import type { ProductCapability } from "./types";

/**
 * Current product-state inventory.
 * LIVE items must include an artifact link for validation.
 */
export const productState: ProductCapability[] = [
  {
    id: "command-teacher",
    name: "command-teacher",
    description:
      "Linux command tutor that diagnoses mistakes, requires reasoning, and records session evidence.",
    status: "LIVE",
    categories: ["learner", "lms"],
    artifactUrl: "/demo#command-teacher",
    artifactLabel: "Live command-teacher demo",
    owner: "Aleks Clark",
    notes: "In early family use with a sixth-grade learner; discovery only, not outcome evidence.",
  },
  {
    id: "primer-tv",
    name: "PrimerTV",
    description:
      "Curated instructional media channel with schedule, pairing, and family viewing controls.",
    status: "LIVE",
    categories: ["media", "learner", "parent"],
    artifactUrl: "/demo#primer-tv",
    artifactLabel: "PrimerTV surface",
    owner: "Aleks Clark",
  },
  {
    id: "lms-core",
    name: "LMS core infrastructure",
    description:
      "Basic learner, resource, and instruction-log infrastructure that underpins the longitudinal record.",
    status: "LIVE",
    categories: ["lms"],
    artifactUrl: "/demo#lms-core",
    artifactLabel: "LMS core infrastructure",
    owner: "Aleks Clark",
  },
  {
    id: "ultralogical",
    name: "Ultralogical agent execution platform",
    description:
      "Multi-agent execution platform used to run specialist tutors and instructional workflows.",
    status: "LIVE",
    categories: ["lms"],
    artifactUrl: "/demo#ultralogical",
    artifactLabel: "Ultralogical platform",
    owner: "Aleks Clark",
  },
  {
    id: "subject-tutoring",
    name: "Core-subject specialist tutoring",
    description:
      "Specialist tutors across mathematics, ELA, science, and social studies for grades 5–8.",
    status: "IN_DEVELOPMENT",
    categories: ["learner", "lms"],
    owner: "Aleks Clark",
    nearTermMilestone: "Paid remediation workflow for at least one core subject",
  },
  {
    id: "curriculum-planning",
    name: "Curriculum planning",
    description:
      "Adult-directed planner that chooses the next standards-linked task from the longitudinal record.",
    status: "IN_DEVELOPMENT",
    categories: ["parent", "lms"],
    owner: "Aleks Clark",
    nearTermMilestone: "Syllabus ingestion through next-task recommendation",
  },
  {
    id: "assessment-evaluation",
    name: "Assessment evaluation",
    description:
      "Evaluation of learner work against standards with revision loops rather than passive correction.",
    status: "IN_DEVELOPMENT",
    categories: ["learner", "lms", "compliance"],
    owner: "Aleks Clark",
    nearTermMilestone: "Blind expert agreement pilot on launch standards",
  },
  {
    id: "standards-mastery-record",
    name: "Standards and mastery records",
    description:
      "Longitudinal standards map storing the artifact and evidence behind every mastery claim.",
    status: "IN_DEVELOPMENT",
    categories: ["lms", "parent", "compliance"],
    owner: "Aleks Clark",
    nearTermMilestone: "Per-standard evidence event schema in production",
  },
  {
    id: "habit-guards",
    name: "Cross-subject habit guards",
    description:
      "Guards that require reasoning, evidence, units, grammar, and precise language across subjects.",
    status: "PLANNED",
    categories: ["learner", "lms"],
    nearTermMilestone: "First habit pack enforced in two subjects",
  },
  {
    id: "project-orchestration",
    name: "Project orchestration",
    description:
      "Real projects treated as standards-bearing instruction with transfer checks.",
    status: "PLANNED",
    categories: ["learner", "media", "parent"],
    nearTermMilestone: "One end-to-end project template with transfer assessment",
  },
  {
    id: "family-admin",
    name: "Family administration and data controls",
    description:
      "Parent authority over curriculum, worldview guardrails, privacy, and progress review.",
    status: "IN_DEVELOPMENT",
    categories: ["parent", "compliance"],
    owner: "Aleks Clark",
    nearTermMilestone: "Parent progress view with exception review",
  },
  {
    id: "lti-clever-pilot",
    name: "LTI and Clever SSO pilot capability",
    description:
      "Supplementary school entry via LTI launch and Clever SSO after family proof.",
    status: "PLANNED",
    categories: ["school", "compliance"],
    nearTermMilestone: "Design-partner LTI launch path after family retention evidence",
  },
  {
    id: "mobile-experience",
    name: "Selected mobile experience",
    description: "Mobile learner and parent surfaces for core tutoring workflows.",
    status: "PLANNED",
    categories: ["learner", "parent"],
    nearTermMilestone: "Priority mobile paths identified from pilot usage",
  },
];
