import type { PricingTier } from "./types";

/**
 * Package ladder hypotheses. Prices are working assumptions, not current revenue.
 */
export const pricingTiers: PricingTier[] = [
  {
    id: "base",
    name: "Base",
    monthlyPriceUsd: 50,
    annualRevenuePerLearnerUsd: 600,
    scope: [
      "Two or three selected subjects",
      "Adaptive tutoring and practice",
      "State-standards mapping",
      "Assessment and mastery record",
      "Parent progress view",
      "Defined included inference allowance",
      "Projects and content limited to selected subjects",
    ],
    primaryBuyers: [
      "Homeschool families assembling an à-la-carte stack",
      "Public/private school families remediating one or more subjects",
      "Advanced students seeking deeper work",
      "ESA families buying a focused service",
    ],
    includedSubjects: "Two or three selected subjects",
    planningAssessment: "Parent selects subjects; Primer owns tutoring loops and mastery records",
    projectsContentReporting: "Limited to selected subjects; basic parent progress view",
    supportLevel: "Near-zero human onboarding; self-serve exception review",
    cogsTarget: "≤$15/learner/month target · ≤$20 floor (Base seed economics)",
    expansionValidation: "Seed-readiness measured on Base — not dependent on upsell",
    includedUsage: "Included model usage plus credit cap or metered overage",
    cogsState: "EVIDENCE_NEEDED",
    status: "HYPOTHESIS",
    competitiveFrame:
      "Full-month adaptive attention for less than one hour of many human tutoring services.",
    notes:
      "Seed-readiness economics are measured on Base. Near-zero human onboarding intended.",
  },
  {
    id: "core",
    name: "Core",
    monthlyPriceUsd: 100,
    annualRevenuePerLearnerUsd: 1200,
    scope: [
      "Full core academics: mathematics, ELA, science, and social studies",
      "Cross-subject habits and reinforcement",
      "Curriculum and weekly planning",
      "Assessments, mastery records, and portfolio",
      "Syllabus/content ingestion",
      "Parent-directed projects",
      "School-grade and state-assessment remediation views",
      "Broader included usage",
    ],
    primaryBuyers: [
      "Homeschool families seeking a complete academic spine",
      "Public-school families wanting a comprehensive supplement",
      "Microschools and hybrid programmes with lean instructional staffing",
      "Funded tutoring/microgrant families",
    ],
    includedSubjects: "Mathematics, ELA, science, and social studies",
    planningAssessment: "Primer owns weekly planning, assessments, mastery, and portfolio",
    projectsContentReporting: "Parent-directed projects; syllabus ingestion; remediation views",
    supportLevel: "Low-touch family support; broader included usage than Base",
    cogsTarget: "Must preserve Base-class unit economics after broader usage",
    expansionValidation:
      "Controlled test target: ≥10% of eligible active families accept $100/mo (not yet run)",
    includedUsage: "Broader included usage than Base; metered overage by tier",
    cogsState: "EVIDENCE_NEEDED",
    status: "HYPOTHESIS",
    competitiveFrame:
      "$1,200/year sits within the $1,200–$2,500 high-impact tutoring budget range districts are advised to expect, while remaining far below full online/private school tuition.",
    notes:
      "Controlled Core conversion tests are a pre-seed milestone; Base economics must not depend on Core upsell.",
  },
  {
    id: "premier",
    name: "Premier",
    monthlyPriceUsd: 300,
    annualRevenuePerLearnerUsd: 3600,
    scope: [
      "Everything in Core",
      "Customized scope and sequence across academic and practical domains",
      "School-specific pedagogy and content guardrails",
      "Advanced courses, test preparation, and executive-function support",
      "Higher-touch parent/educator reporting",
      "Bring-your-own-content ingestion and managed library",
      "PrimerTV/media curation and project orchestration",
      "Larger usage envelope and priority support",
      "Optional scheduled human expert review or specialist marketplace access",
    ],
    primaryBuyers: [
      "Families stacking tutors, test prep, coaching, and consulting",
      "Elite independent and boarding-school families",
      "Disability-funded families needing coordinated learning support",
      "Concierge homeschool families",
      "Elite schools embedding customized tutoring into tuition or learning support",
    ],
    includedSubjects: "Full academic and practical domains with custom scope and sequence",
    planningAssessment:
      "School- or family-authored guardrails; higher-touch reporting and long-term planning",
    projectsContentReporting:
      "Managed library, PrimerTV/media, project orchestration, optional expert review",
    supportLevel: "Priority support; optional scheduled human expert or specialist access",
    cogsTarget: "Higher usage envelope; human review optional and metered — evidence needed",
    expansionValidation:
      "Validation target: ≥20 paid families or one elite-school design partner (not yet run)",
    includedUsage: "Larger usage envelope with priority support",
    cogsState: "EVIDENCE_NEEDED",
    status: "HYPOTHESIS",
    competitiveFrame:
      "$300 buys roughly 4–8 hours of marketplace tutoring or fewer than two hours of premium test prep; Primer offers persistent coverage and a unified learner record.",
    notes:
      "Pre-seed target includes at least 20 paid Premier families or one elite-school design partner.",
  },
  {
    id: "institutional",
    name: "Institutional / targeted intervention",
    monthlyPriceUsd: 100,
    annualRevenuePerLearnerUsd: 1200,
    scope: [
      "Targeted tutoring or intervention for a defined cohort",
      "Dosage, active use, and standards-progress reporting",
      "LTI launch and Clever SSO entry",
      "Roster and grade synchronization over time",
      "Privacy, accessibility, and audit packages",
    ],
    primaryBuyers: [
      "Public schools buying targeted intervention",
      "Elite schools buying parent access, learning support, or tuition-embedded capacity",
      "Microschools and co-ops",
    ],
    includedSubjects: "Programme-defined cohort subjects",
    planningAssessment: "School owns cohort selection; Primer reports dosage and standards progress",
    projectsContentReporting: "Outcomes reporting; roster/grade sync over time",
    supportLevel: "Implementation support for bounded pilots; not whole-school helpdesk",
    cogsTarget: "Priced as instructional capacity, not commodity seats — unit economics unproven",
    expansionValidation: "Separate from family ladder; do not blend into consumer ARR",
    includedUsage: "Programme-defined dosage with outcomes reporting",
    cogsState: "EVIDENCE_NEEDED",
    status: "HYPOTHESIS",
    competitiveFrame:
      "Priced as instructional capacity (~$1,200 per supported learner annually), not commodity software seats.",
    notes:
      "Districts will not buy $1,200 software licenses for every student. Typical school-side cohort is 5–15% of enrollment.",
  },
];
