import type { CompetitorCategory } from "./types";

/**
 * Category-level comparison only. No unverifiable product checkmarks.
 * Capability ratings are directional synthesis from official product pages.
 */
export const competitorCategories: CompetitorCategory[] = [
  {
    id: "general-ai-study",
    category: "General AI study tools",
    examples: ["ChatGPT Study Mode", "Gemini Guided Learning"],
    whatCustomersBuy: "Near-free explanations and study help",
    primerResponse:
      "Do not compete on conversation alone; win on adult governance, curriculum, evidence, and continuity.",
    capabilities: {
      adultDirected: "no",
      longitudinalPlanning: "no",
      crossSubjectHabits: "no",
      masteryEvidence: "no",
      projectsAsInstruction: "no",
      offScreenWork: "partial",
      householdFirst: "yes",
    },
    sourceIds: [],
    notes: "Model commoditization is a core competitive risk.",
  },
  {
    id: "ai-tutors",
    category: "AI tutors",
    examples: ["Khanmigo", "Synthesis Tutor", "SchoolAI", "Amira"],
    whatCustomersBuy: "Guided help or subject tutoring",
    primerResponse:
      "Differentiate with grades 5–8 breadth, cross-subject habits, projects, and evidence per standard.",
    capabilities: {
      adultDirected: "partial",
      longitudinalPlanning: "partial",
      crossSubjectHabits: "no",
      masteryEvidence: "partial",
      projectsAsInstruction: "no",
      offScreenWork: "no",
      householdFirst: "yes",
    },
    sourceIds: ["khanmigo-pricing", "synthesis-tutor"],
  },
  {
    id: "mastery-systems",
    category: "Mastery systems",
    examples: ["Math Academy", "ALEKS", "MATHia"],
    whatCustomersBuy: "Rigorous adaptive progression, usually one subject",
    primerResponse:
      "Match rigor where offered; add cross-subject transfer and real-work evidence.",
    capabilities: {
      adultDirected: "partial",
      longitudinalPlanning: "yes",
      crossSubjectHabits: "no",
      masteryEvidence: "yes",
      projectsAsInstruction: "no",
      offScreenWork: "no",
      householdFirst: "partial",
    },
    sourceIds: ["math-academy"],
  },
  {
    id: "homeschool-platforms",
    category: "Homeschool platforms",
    examples: ["Time4Learning", "Power Homeschool", "Miacademy", "Acellus"],
    whatCustomersBuy: "Broad, self-paced curriculum and automatic reports",
    primerResponse:
      "Offer deeper diagnosis, correction, and adult control with less screen dependence.",
    capabilities: {
      adultDirected: "yes",
      longitudinalPlanning: "partial",
      crossSubjectHabits: "no",
      masteryEvidence: "partial",
      projectsAsInstruction: "partial",
      offScreenWork: "partial",
      householdFirst: "yes",
    },
    sourceIds: [],
  },
  {
    id: "off-screen-curriculum",
    category: "Off-screen curriculum",
    examples: ["The Good and the Beautiful", "literature/project curricula"],
    whatCustomersBuy: "Parent-led books, print work, projects",
    primerResponse:
      "Complement or automate the planning/feedback burden rather than dismissing trusted curricula.",
    capabilities: {
      adultDirected: "yes",
      longitudinalPlanning: "partial",
      crossSubjectHabits: "partial",
      masteryEvidence: "no",
      projectsAsInstruction: "yes",
      offScreenWork: "yes",
      householdFirst: "yes",
    },
    sourceIds: [],
  },
  {
    id: "supplemental-school",
    category: "Supplemental school systems",
    examples: ["i-Ready", "Exact Path", "IXL"],
    whatCustomersBuy: "Diagnostics, practice, intervention, standards reports",
    primerResponse:
      "Produce evidence during learning and include work outside the platform.",
    capabilities: {
      adultDirected: "partial",
      longitudinalPlanning: "yes",
      crossSubjectHabits: "no",
      masteryEvidence: "yes",
      projectsAsInstruction: "no",
      offScreenWork: "no",
      householdFirst: "no",
    },
    sourceIds: [],
  },
  {
    id: "conventional-lms",
    category: "Conventional LMS platforms",
    examples: ["Canvas", "Schoology", "Google Classroom", "Moodle"],
    whatCustomersBuy: "Course delivery, assignments, gradebook, identity, integrations",
    primerResponse:
      "Primer is also an LMS, differentiated by native tutoring and mastery; enter schools supplementarily and integrate before asking buyers to consolidate platforms.",
    capabilities: {
      adultDirected: "yes",
      longitudinalPlanning: "partial",
      crossSubjectHabits: "no",
      masteryEvidence: "partial",
      projectsAsInstruction: "partial",
      offScreenWork: "partial",
      householdFirst: "no",
    },
    sourceIds: ["canvas-instructure"],
    notes:
      "Canvas can remain the school's system of record while Primer runs a supplementary programme.",
  },
  {
    id: "alt-schools",
    category: "High-touch alternative schools",
    examples: ["Alpha School / 2 Hour Learning", "microschools", "TimeBack"],
    whatCustomersBuy: "A whole-school model around personalized learning",
    primerResponse:
      "Offer the instructional system without the school tuition or screen-maximalist model.",
    capabilities: {
      adultDirected: "yes",
      longitudinalPlanning: "yes",
      crossSubjectHabits: "partial",
      masteryEvidence: "partial",
      projectsAsInstruction: "partial",
      offScreenWork: "partial",
      householdFirst: "no",
    },
    sourceIds: [],
    notes:
      "Name collision risk: primer.com is already a funded K–8 microschool company.",
  },
  {
    id: "primer",
    category: "Primer",
    examples: ["Primer instructional LMS"],
    whatCustomersBuy:
      "Adult-directed tutoring, longitudinal mastery evidence, projects, and parent-owned records",
    primerResponse:
      "Integrate tutoring, LMS memory, cross-subject habits, projects, and evidence in one longitudinal system.",
    capabilities: {
      adultDirected: "yes",
      longitudinalPlanning: "yes",
      crossSubjectHabits: "yes",
      masteryEvidence: "yes",
      projectsAsInstruction: "yes",
      offScreenWork: "yes",
      householdFirst: "yes",
    },
    sourceIds: ["product-state-internal"],
    notes:
      "Target operating model. Many elements are IN DEVELOPMENT or PLANNED — not all LIVE. Category row describes intended differentiation, not shipped completeness.",
  },
];
