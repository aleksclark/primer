import type { ResearchClaim } from "./types";

/**
 * Prior research supports the mechanism, not company outcomes.
 * Safe claims only — no Primer efficacy asserted.
 */
export const researchClaims: ResearchClaim[] = [
  {
    id: "tutoring-meta-2024",
    effect: "Pooled average effect of preK–12 tutoring",
    effectSizeSd: 0.288,
    population: "89 randomized controlled trials, preK–12",
    study: "Nickow, Oreopoulos, and Quan, AERJ 2024",
    year: "2024",
    design: "Meta-analysis of randomized controlled trials",
    safeClaim:
      "Across 89 randomized preK–12 trials, tutoring produced an average effect of about 0.29 SD. This is prior research, not a Primer outcome.",
    caveat:
      "Average across diverse programs, subjects, ages, tutors, doses, and outcome measures. Effects shrink at policy-relevant scale when dosage and fidelity weaken.",
    sourceIds: ["nickow-oreopoulos-quan-2024", "kraft-schueler-falken-scale"],
    status: "LIVE",
  },
  {
    id: "tutoring-dosage-drivers",
    effect: "Stronger results with sustained dosage and fidelity",
    population: "Modern tutoring reviews and design-principle syntheses",
    study: "EdResearch for Action / NSSA design principles; Kraft et al. scale analysis",
    year: "2021–2024",
    design: "Design-principle synthesis and scale analysis",
    safeClaim:
      "Tutoring effects are strongest with frequent, sustained, curriculum-aligned sessions, small groups, and implementation fidelity.",
    caveat:
      "These are product requirements Primer must meet, not evidence that Primer already meets them.",
    sourceIds: ["kraft-schueler-falken-scale", "nickow-oreopoulos-quan-2024"],
    status: "LIVE",
  },
  {
    id: "saga-chicago",
    effect: "Intensive human math tutoring in Chicago high schools",
    effectSizeSd: 0.18,
    population: "Roughly 5,300 high-school students across two RCTs",
    study: "Guryan et al., AER 2023 (Saga Education)",
    year: "2023",
    design: "Two randomized controlled trials",
    safeClaim:
      "Two Chicago randomized trials of intensive mathematics tutoring found effects ranging from about 0.18 to 0.40 SD with substantial reductions in course failure.",
    caveat: "Daily, intensive, usually 2:1 human tutoring — not an autonomous AI tutor.",
    sourceIds: ["guryan-aer-2023"],
    status: "LIVE",
  },
  {
    id: "bloom-historical",
    effect: "Historical two-sigma tutoring illustration",
    effectSizeSd: 2.0,
    population: "Two short dissertation studies with narrow researcher-made tests",
    study: "Bloom, 1984; von Hippel reconstruction, 2024",
    year: "1984 / 2024",
    design: "Historical illustration with modern reconstruction",
    safeClaim:
      "Bloom posed the question of how to approximate individual tutoring at scale. Modern randomized evidence supports an average tutoring effect around 0.29 SD, not two sigma.",
    caveat:
      "Historical motivation only. Must not be used as a promised Primer outcome. Original gap bundled mastery quizzing, feedback, retesting, and extra time.",
    sourceIds: ["bloom-1984", "von-hippel-2024"],
    status: "LIVE",
  },
  {
    id: "mastery-learning-meta",
    effect: "Mastery-learning program effects",
    effectSizeSd: 0.52,
    population: "108 controlled evaluations (Kulik); stricter best-evidence subset (Slavin)",
    study: "Kulik, Kulik, and Bangert-Drowns 1990; Slavin 1987",
    year: "1987 / 1990",
    design: "Meta-analysis and best-evidence synthesis",
    safeClaim:
      "Mastery learning and corrective feedback have a positive evidence base, though broad standardized-test effects are smaller and disputed (Kulik ~0.52 SD mean; Slavin median ~0.25 SD on stricter criteria).",
    caveat:
      "Effects stronger on closely aligned assessments; much of the literature is old. Primer must validate both aligned mastery and transfer to independent measures.",
    sourceIds: ["kulik-mastery-1990", "slavin-mastery-1987"],
    status: "LIVE",
  },
  {
    id: "adaptive-mindspark",
    effect: "Computer-adaptive instruction gains",
    effectSizeSd: 0.37,
    population: "Indian lottery-based RCT (Mindspark); later government-school scale-up",
    study: "Muralidharan, Singh, and Ganimian, AER 2019",
    year: "2019",
    design: "Lottery-based randomized controlled trial",
    safeClaim:
      "Adaptive, level-appropriate instruction can produce meaningful gains (about 0.37 SD math / 0.23 SD Hindi over 4.5 months in the original study), with results tied to usage and adult supervision.",
    caveat:
      "Does not validate a specific LLM architecture or U.S. homeschool implementation.",
    sourceIds: ["muralidharan-mindspark-2019"],
    status: "LIVE",
  },
  {
    id: "primer-efficacy",
    effect: "Primer learning outcomes on independent assessments",
    population: "Primer learners (none measured yet)",
    study: "Primer proof plan — not yet run",
    year: "planned",
    design: "Company proof plan (not yet executed)",
    safeClaim:
      "Primer's first failure test: if learners do not progress on independently scored, state-aligned standards, the system is not working. Company efficacy remains to be established.",
    caveat:
      "Current family use is discovery only (approximately 40 founder hours and $1,000 inputs). No outcome claim is supportable.",
    sourceIds: ["seed-readiness-internal", "product-state-internal"],
    status: "EVIDENCE_NEEDED",
    companyEvidence: true,
  },
];
