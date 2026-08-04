import type { TeamRole } from "./types";

/**
 * Compact 18-month team plan. Salary figures live in fundingPlan use-of-funds,
 * not as defensive founder copy.
 */
export type { TeamRole };

export const teamPlan: TeamRole[] = [
  {
    id: "founder",
    title: "Founder / CEO",
    ownership: [
      "Product direction and instructional architecture",
      "Curriculum safety, privacy, and accessibility ownership",
      "Platform and agent infrastructure",
      "Early customer discovery and design partners",
    ],
    timing: "Current — sole team member",
    presence: "current",
  },
  {
    id: "educator-product",
    title: "Educator / product owner",
    ownership: [
      "Grades 5–8 curriculum and assessment design",
      "Mastery criteria and habit packs",
      "Teacher/parent workflow quality",
      "Learning-outcome study design with research partners",
    ],
    timing: "Hire 1 of 2 high-leverage roles in the 18-month plan",
    presence: "planned",
  },
  {
    id: "product-platform",
    title: "Product / platform builder",
    ownership: [
      "Learner and parent product surfaces",
      "Reliability, observability, and usage metering",
      "LTI/Clever and roster integrations",
      "COGS instrumentation and pilot operations tooling",
    ],
    timing: "Hire 1 of 2 high-leverage roles in the 18-month plan",
    presence: "planned",
  },
];
