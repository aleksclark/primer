import type { StatusLabel } from "./types";

/** Primer measurement plan, thresholds, and claim boundaries. */

export interface EvidenceStage {
  id: string;
  order: number;
  name: string;
  goal: string;
  actions: string[];
  status: StatusLabel;
}

export interface LearningThreshold {
  id: string;
  name: string;
  floor: string;
  target: string;
  definition: string;
  status: StatusLabel;
}

export interface ClaimBoundary {
  id: string;
  allowed: boolean;
  claim: string;
}

export const EVIDENCE_UPDATED = "2026-08-04";

export const evidenceStages: EvidenceStage[] = [
  {
    id: "measurement-validity",
    order: 1,
    name: "Measurement validity",
    goal: "Establish that Primer measures accurately before claiming learning impact.",
    actions: [
      "Create externally scored tasks for the launch standards",
      "Blind human reviewers to Primer mastery decisions",
      "Measure agreement, false mastery, and missed mastery",
      "Test scoring consistency across model versions, demographics, writing styles, and accommodations",
      "Spot-check anomalies and implausible jumps",
      "Require stronger review for consequential or low-confidence decisions until reliability is known",
    ],
    status: "PLANNED",
  },
  {
    id: "feasibility-pilot",
    order: 2,
    name: "Feasibility pilot",
    goal: "Establish operational and early learning signals on a small cohort.",
    actions: [
      "Enrollment and completion",
      "Dosage achieved and correction-loop completion",
      "Parent burden and time saved vs founder baseline (>10 hours/week on a standard package)",
      "Safety and escalation rates",
      "Pre/post change on an independent state-aligned assessment",
      "Student and parent retention",
      "Pre-register primary outcomes and success thresholds before analysis",
    ],
    status: "PLANNED",
  },
  {
    id: "comparative-study",
    order: 3,
    name: "Comparative study",
    goal: "Compare Primer with a credible alternative using external assessment.",
    actions: [
      "Business-as-usual curriculum plus generic online practice as comparison",
      "Match or randomize where feasible",
      "External standardized or validated assessment — not only Primer items",
      "Report intent-to-treat and treatment-on-treated separately",
      "Report attrition by group, implementation cost per learner, model/version, and curriculum used",
      "Publish null and adverse results under the same policy as positive results",
    ],
    status: "PLANNED",
  },
  {
    id: "school-evidence",
    order: 4,
    name: "School evidence",
    goal: "Independent studies in the intended institutional setting before broad school sales.",
    actions: [
      "Homeschool efficacy does not automatically transfer to classrooms",
      "Implementation fidelity, accessibility, and privacy readiness required",
      "Align eventual school evidence with ESSA-tier expectations where relevant",
    ],
    status: "PLANNED",
  },
];

export const learningThresholds: LearningThreshold[] = [
  {
    id: "mastery-agreement",
    name: "Mastery / expert agreement",
    floor: "85%",
    target: "90%+",
    definition:
      "Agreement between Primer mastery decisions and blinded human experts on launch standards.",
    status: "EVIDENCE_NEEDED",
  },
  {
    id: "independent-learning",
    name: "Independent learning progression",
    floor: "Credible positive progression",
    target: "≥0.20 SD or pre-specified equivalent",
    definition:
      "Progression on independently scored, state-aligned assessments. Lack of progression is the explicit failure condition.",
    status: "EVIDENCE_NEEDED",
  },
  {
    id: "activation-48h",
    name: "Activation (first value loop in 48h)",
    floor: "50%",
    target: "70%+",
    definition: "Share of new accounts completing a first diagnose → attempt → revise loop within 48 hours.",
    status: "EVIDENCE_NEEDED",
  },
  {
    id: "retention-12-week",
    name: "12-week paid retention",
    floor: "75%",
    target: "85%+",
    definition: "Paid learners still active 12 weeks after paid activation (academic pauses reported separately).",
    status: "EVIDENCE_NEEDED",
  },
];

export const nullAdversePolicy = [
  "Primary outcomes and failure thresholds are pre-registered before analysis of a study cohort.",
  "Null and adverse findings are published in the diligence register with the same prominence as positive findings.",
  "If learners do not progress on independently scored, state-aligned standards, the system is not working — that is the company failure test.",
  "Product engagement metrics are never substituted for learning outcomes in external claims.",
  "Model/version, curriculum, dosage, and attrition are reported alongside any effect estimate.",
  "Silent mastery inflation (false mastery) is treated as a product defect, not a growth metric.",
];

export const claimBoundaries: ClaimBoundary[] = [
  {
    id: "allow-tutoring-meta",
    allowed: true,
    claim:
      "Across 89 randomized preK–12 trials, tutoring produced an average effect of about 0.29 SD (prior research, not Primer).",
  },
  {
    id: "allow-dosage",
    allowed: true,
    claim:
      "Tutoring effects are strongest with frequent, sustained, curriculum-aligned sessions and small groups.",
  },
  {
    id: "allow-mastery-base",
    allowed: true,
    claim:
      "Mastery learning and corrective feedback have a positive evidence base; broad standardized-test effects are smaller and disputed.",
  },
  {
    id: "allow-adaptive",
    allowed: true,
    claim:
      "Adaptive instruction can produce meaningful gains; implementation and usage determine results.",
  },
  {
    id: "allow-design",
    allowed: true,
    claim:
      "Primer is designed to reproduce key mechanisms: immediate feedback, individualized pacing, repeated correction, longitudinal context, and frequent practice.",
  },
  {
    id: "allow-unproven",
    allowed: true,
    claim: "Primer's own efficacy remains to be established through external assessments and well-designed pilots.",
  },
  {
    id: "deny-two-sigma",
    allowed: false,
    claim: "Primer gives every student a two-sigma improvement.",
  },
  {
    id: "deny-ai-equals-human",
    allowed: false,
    claim: "AI tutoring is equivalent to trained human tutoring.",
  },
  {
    id: "deny-always-beats-small-group",
    allowed: false,
    claim: "One-to-one instruction always beats small-group instruction.",
  },
  {
    id: "deny-mastery-one-sd",
    allowed: false,
    claim: "Mastery learning raises standardized scores by one standard deviation.",
  },
  {
    id: "deny-every-log-mastery",
    allowed: false,
    claim: "Every logged interaction is valid evidence of mastery.",
  },
  {
    id: "deny-engagement-is-learning",
    allowed: false,
    claim: "Product engagement is the same as learning.",
  },
];

export const bloomCaveat =
  "Bloom (1984) is historical motivation only. The original two-sigma illustration bundled tutoring with mastery quizzing, corrective feedback, retesting, trained tutors, and extra time on narrow researcher-made tests. Modern randomized evidence supports an average tutoring effect around 0.29 SD, not two sigma. Never use Bloom as a promised Primer outcome.";
