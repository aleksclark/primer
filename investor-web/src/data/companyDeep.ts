import type { StatusLabel } from "./types";

/** Company page content beyond founderProof timeline events. */

export interface HiringPrinciple {
  id: string;
  title: string;
  body: string;
}

export interface ContactProcessStep {
  id: string;
  order: number;
  title: string;
  body: string;
}

export const COMPANY_UPDATED = "2026-08-04";

export const ebackpackProofPoints = [
  {
    id: "scale",
    label: "Scale",
    value: ">500,000 students · ~60,000 req/min · 99.9% uptime",
  },
  {
    id: "stack",
    label: "Platform work",
    value: "Go microservices (4–6 ms critical paths), MongoDB/PostgreSQL, petabyte-scale file storage, TypeScript/Angular frontend refactor",
  },
  {
    id: "integrations",
    label: "Integrations built",
    value:
      "Skyward, PowerSchool, Google Classroom (rosters/grades); OAuth 1/2 with Google and Microsoft; OneDrive for Business; Common Core assessment tagging; Common Cartridge",
  },
  {
    id: "implication",
    label: "Implication",
    value:
      "SIS, SSO, LMS, roster, gradebook, and content interchange are familiar engineering — not a new category. Each Primer connector still must be implemented and certified.",
  },
];

export const hiringPrinciples: HiringPrinciple[] = [
  {
    id: "high-leverage",
    title: "High-leverage owners only",
    body: "Pre-seed headcount is founder plus one or two senior hires. Prefer staff-level ownership of education or platform outcomes over a large junior team.",
  },
  {
    id: "educator-ownership",
    title: "Educator ownership is non-negotiable",
    body: "Curriculum, mastery criteria, and assessment design need an experienced grades 5–8 owner so founder proximity does not become founder monoculture.",
  },
  {
    id: "milestone-timing",
    title: "Milestone-tied timing",
    body: "Second employee timing can follow activation/retention evidence if the first hire plus founder can clear the first paid workflow. Do not hire for optics.",
  },
  {
    id: "evidence-over-preference",
    title: "Evidence over founder preference",
    body: "Product decisions that affect learning claims require measurement validity and pilot evidence. Founder-bias risk is explicit and mitigated by outside educator ownership.",
  },
  {
    id: "no-premature-gtm",
    title: "No premature GTM headcount",
    body: "Sales and marketing spend waits on retention evidence. Discovery and design-partner support are budgeted; a growth team is not.",
  },
];

export const contactProcess: ContactProcessStep[] = [
  {
    id: "intro",
    order: 1,
    title: "Intro conversation",
    body: "Use Discuss the round on the primary page or email the founder. Expect a product-state walkthrough with synthetic data only.",
  },
  {
    id: "public-diligence",
    order: 2,
    title: "Public diligence",
    body: "Market, evidence, schools, company, and diligence index pages answer core underwriting questions without a live walkthrough.",
  },
  {
    id: "references",
    order: 3,
    title: "References process",
    body: "On serious interest: eBackpack scale/integration references, Curri agent-platform leadership references, and employment verification where appropriate. Intros are coordinated by the founder — not posted publicly.",
  },
  {
    id: "gated-room",
    order: 4,
    title: "Gated materials",
    body: "Under NDA when appropriate: deeper financial model, architecture suitable for diligence, and IP/conflicts review. No family data, no cap table on the public site, no live production credentials.",
  },
];

export const founderRiskMitigation = [
  "Add educator/product owner so curriculum and assessment judgment are not sole-founder concentrated.",
  "Publish product-state labels so live vs target experience cannot be confused.",
  "Tie fundraising milestones to retention, learning progression, and unit economics — not narrative alone.",
  "Keep current family use labeled discovery (~40 founder hours, ~$1,000 inputs, ~1 week).",
  "Require measurement validity before any company efficacy claim.",
];

export const openRolesNote: { status: StatusLabel; body: string } = {
  status: "HYPOTHESIS",
  body: "Open roles in the 18-month plan: Educator/product owner and Product/platform builder. Advisors for learning research, child safety/privacy, and school GTM are expected as contracted specialists before full-time headcount in those lanes.",
};
