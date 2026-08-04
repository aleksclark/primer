import type { AdoptionRung } from "./types";

/**
 * Overlapping GTM adoption ladder — not a rigid timeline.
 * Each rung carries buyer, proof burden, and integration notes.
 */
export type { AdoptionRung };

export const adoptionLadder: AdoptionRung[] = [
  {
    id: "family-base",
    order: 1,
    name: "Family Base",
    buyer: "Homeschool or school-family parent buying two or three subjects at $50/month.",
    proof: "Activation, correction loops completed, 12-week paid retention, parent time reduced.",
    integration: "Self-serve household account; no institutional connector required.",
    status: "HYPOTHESIS",
  },
  {
    id: "family-core",
    order: 2,
    name: "Family Core",
    buyer: "Same household expanding to full core academics at $100/month.",
    proof: "Controlled Core conversion without depending Base economics on upsell.",
    integration: "Same learner record; broader curriculum planning and portfolio.",
    status: "HYPOTHESIS",
  },
  {
    id: "esa-microschool",
    order: 3,
    name: "ESA / microschool",
    buyer: "Funded families, co-ops, and microschool founders aggregating 10–30 learners.",
    proof: "Vendor eligibility where required; multi-learner admin; reference sites.",
    integration: "Organization roles, CSV rostering; per-state ESA payment paths as needed.",
    status: "HYPOTHESIS",
  },
  {
    id: "premier-elite-support",
    order: 4,
    name: "Premier family / elite learning support",
    buyer:
      "High-intent families and elite independent/boarding learning-support programmes at $300/month.",
    proof: "At least 20 paid Premier families or one elite-school design partner.",
    integration: "Higher-touch reporting; optional human expert review; school-specific guardrails.",
    status: "HYPOTHESIS",
  },
  {
    id: "lti-clever-pilot",
    order: 5,
    name: "LTI / Clever pilot",
    buyer: "School academic leader running a bounded supplementary cohort.",
    proof: "Family retention and learning evidence first; then paid bounded school pilots.",
    integration: "LTI launch and Clever SSO; roster/grade sync deepen over time.",
    status: "PLANNED",
  },
  {
    id: "tuition-embedded",
    order: 6,
    name: "Tuition-embedded or academic-core deployment",
    buyer: "School embedding Primer as learning-support or primary instructional capacity.",
    proof: "Widespread de facto use, independent outcomes, institutional support model.",
    integration: "Primary-LMS status earned after supplementary proof — not demanded on day one.",
    status: "PLANNED",
  },
];
