import { DiligencePage, DiligenceSection } from "@/components/DiligencePage";
import { EvidenceExplorer, SeedScorecardExplorer } from "@/components/explorers";
import { CaveatNote } from "@/components/CaveatNote";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { SourceLinkList } from "@/components/SourceLink";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { researchClaims, sources } from "@/data";
import {
  bloomCaveat,
  claimBoundaries,
  EVIDENCE_UPDATED,
  evidenceStages,
  learningThresholds,
  nullAdversePolicy,
} from "@/data/evidencePlan";

const nav = [
  { id: "research-basis", label: "Tutoring, mastery, adaptive research" },
  { id: "bloom", label: "Bloom caveat" },
  { id: "measurement-plan", label: "Primer measurement plan" },
  { id: "thresholds", label: "Seed learning thresholds" },
  { id: "null-policy", label: "Null / adverse result policy" },
  { id: "claim-boundaries", label: "Claims allowed and rejected" },
  { id: "scorecard", label: "Seed scorecard (learning rows)" },
  { id: "source-register", label: "Full source register" },
];

const researchSourceIds = [
  ...new Set(researchClaims.flatMap((c) => c.sourceIds)),
];

/**
 * Diligence-depth evidence register with printable layout.
 */
export function EvidencePage() {
  const allowed = claimBoundaries.filter((c) => c.allowed);
  const rejected = claimBoundaries.filter((c) => !c.allowed);

  return (
    <DiligencePage
      eyebrow="Evidence"
      title="Research basis, measurement plan, and source register"
      summary="Prior research supports tutoring, mastery, and adaptive mechanisms. It does not establish Primer efficacy. This page holds safe claims, the Bloom caveat, the multi-stage proof plan, seed learning thresholds, and the null-result policy."
      updated={EVIDENCE_UPDATED}
      nav={nav}
      sources={sources.filter((s) => researchSourceIds.includes(s.id) || s.qualityTier === "peer_reviewed" || s.qualityTier === "primary_official" || s.id.startsWith("seed") || s.id.includes("product-state"))}
      printable
      caveat="Company efficacy is NOT YET MEASURED. Research effect sizes are not Primer outcomes."
      secondaryAction={{ label: "Primary evidence section", href: "/#proof" }}
    >
      <DiligenceSection
        id="research-basis"
        title="Tutoring, mastery, and adaptive research"
        eyebrow="Prior evidence"
      >
        <EvidenceExplorer />
      </DiligenceSection>

      <DiligenceSection id="bloom" title="Bloom two-sigma caveat" eyebrow="Historical only">
        <RuledCard attention footer={<SystemLabel tone="attention">Not a product promise</SystemLabel>}>
          <SystemLabel tone="attention">Bloom 1984</SystemLabel>
          <p className="type-body prose-measure" style={{ margin: 0 }}>
            {bloomCaveat}
          </p>
          <SourceLinkList
            sources={sources.filter((s) =>
              ["bloom-1984", "von-hippel-2024", "nickow-oreopoulos-quan-2024"].includes(s.id),
            )}
          />
        </RuledCard>
      </DiligenceSection>

      <DiligenceSection
        id="measurement-plan"
        title="Primer measurement and study plan"
        eyebrow="Four stages"
      >
        <ol className="ladder-list">
          {evidenceStages.map((stage) => (
            <li key={stage.id} className="ladder-list__item">
              <RuledCard
                footer={
                  <div className="loop-diagram__footer">
                    <SystemLabel>Stage {String(stage.order).padStart(2, "0")}</SystemLabel>
                    <StateBadge status={stage.status} />
                  </div>
                }
              >
                <SystemLabel tone="accent">{stage.name}</SystemLabel>
                <p className="type-small" style={{ margin: 0 }}>
                  {stage.goal}
                </p>
                <ul className="section-list section-list--compact">
                  {stage.actions.map((action) => (
                    <li key={action} className="type-small text-muted">
                      {action}
                    </li>
                  ))}
                </ul>
              </RuledCard>
            </li>
          ))}
        </ol>
      </DiligenceSection>

      <DiligenceSection
        id="thresholds"
        title="Seed learning thresholds"
        eyebrow="Failure tests"
      >
        <div className="funds-table-wrap">
          <table className="funds-table">
            <caption className="sr-only">Learning-related seed thresholds</caption>
            <thead>
              <tr>
                <th scope="col">Metric</th>
                <th scope="col">Floor</th>
                <th scope="col">Target</th>
                <th scope="col">Definition</th>
                <th scope="col">Status</th>
              </tr>
            </thead>
            <tbody>
              {learningThresholds.map((row) => (
                <tr key={row.id}>
                  <th scope="row" className="type-small">
                    {row.name}
                  </th>
                  <td className="type-small">{row.floor}</td>
                  <td className="type-small">{row.target}</td>
                  <td className="type-small text-muted">{row.definition}</td>
                  <td>
                    <StateBadge status={row.status} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <CaveatNote blocker>
          If learners do not progress on independently scored, state-aligned standards, the system
          is not working.
        </CaveatNote>
      </DiligenceSection>

      <DiligenceSection
        id="null-policy"
        title="Null and adverse result policy"
        eyebrow="Publication rules"
      >
        <ul className="section-list">
          {nullAdversePolicy.map((line) => (
            <li key={line} className="type-small">
              {line}
            </li>
          ))}
        </ul>
      </DiligenceSection>

      <DiligenceSection
        id="claim-boundaries"
        title="Claims the pitch can and cannot make"
        eyebrow="Boundaries"
      >
        <RuledGrid columns={2}>
          <RuledCard footer={<SystemLabel>Allowed</SystemLabel>}>
            <SystemLabel tone="accent">Safe claims</SystemLabel>
            <ul className="section-list">
              {allowed.map((c) => (
                <li key={c.id} className="type-small">
                  {c.claim}
                </li>
              ))}
            </ul>
          </RuledCard>
          <RuledCard attention footer={<SystemLabel tone="attention">Rejected</SystemLabel>}>
            <SystemLabel tone="attention">Do not claim</SystemLabel>
            <ul className="section-list">
              {rejected.map((c) => (
                <li key={c.id} className="type-small">
                  {c.claim}
                </li>
              ))}
            </ul>
          </RuledCard>
        </RuledGrid>
      </DiligenceSection>

      <DiligenceSection id="scorecard" title="Seed-readiness scorecard" eyebrow="Month-18">
        <p className="type-small text-muted prose-measure" style={{ margin: 0 }}>
          Full commercial and learning scorecard. Currents are NOT YET MEASURED until observed.
        </p>
        <div style={{ marginTop: "var(--primer-space-5)" }}>
          <SeedScorecardExplorer />
        </div>
      </DiligenceSection>

      <DiligenceSection id="source-register" title="Full source register" eyebrow="Citations">
        <div className="source-register">
          {sources.map((source) => (
            <article key={source.id} className="source-register__item">
              <div className="source-register__head">
                <SystemLabel>{source.qualityTier.replace(/_/g, " ")}</SystemLabel>
                <span className="type-label text-muted">{source.publicationDate}</span>
              </div>
              <h3 className="type-small" style={{ margin: 0, fontWeight: 500 }}>
                {source.title}
              </h3>
              <p className="type-small text-muted" style={{ margin: 0 }}>
                {source.organization}
                {source.notes ? ` · ${source.notes}` : ""}
              </p>
              <a
                className="source-link"
                href={source.url}
                target="_blank"
                rel="noopener noreferrer"
              >
                <span aria-hidden="true">↗</span>
                <span>{source.url}</span>
              </a>
            </article>
          ))}
        </div>
      </DiligenceSection>
    </DiligencePage>
  );
}
