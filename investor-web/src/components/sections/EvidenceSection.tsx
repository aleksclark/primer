import { CaveatNote } from "@/components/CaveatNote";
import { MetricBlock, MetricRow } from "@/components/MetricBlock";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { SourceLinkList } from "@/components/SourceLink";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { researchClaims, sources } from "@/data";

function resolveSources(ids: string[]) {
  return sources.filter((s) => ids.includes(s.id));
}

/**
 * Three-tier evidence: research basis, learner evidence model, company plan.
 * Company tier must show NOT YET MEASURED / EVIDENCE NEEDED.
 */
export function EvidenceSection() {
  const research = researchClaims.filter((c) => c.id !== "primer-efficacy");
  const company = researchClaims.find((c) => c.id === "primer-efficacy");
  const headline = research.find((c) => c.id === "tutoring-meta-2024");

  return (
    <div className="section-extras">
      {headline?.effectSizeSd != null ? (
        <MetricRow summary="Prior tutoring meta-analysis effect size">
          <MetricBlock
            label="Research effect"
            value={`+${headline.effectSizeSd.toFixed(3)} SD`}
            hint="Average across 89 RCTs — not a Primer outcome"
          />
          <MetricBlock
            label="Company efficacy"
            value="NOT YET MEASURED"
            hint="Independently scored, state-aligned progression required"
          />
        </MetricRow>
      ) : null}

      <RuledGrid columns={3}>
        <RuledCard footer={<SystemLabel>Tier 01 · Research basis</SystemLabel>}>
          <SystemLabel tone="accent">Prior evidence</SystemLabel>
          <p className="type-h3" style={{ margin: 0 }}>
            Mechanism support
          </p>
          <ul className="section-list">
            {research.slice(0, 4).map((claim) => (
              <li key={claim.id}>
                <p className="type-small" style={{ margin: 0 }}>
                  {claim.safeClaim}
                </p>
                <CaveatNote label="Note">{claim.caveat}</CaveatNote>
                <SourceLinkList sources={resolveSources(claim.sourceIds)} />
              </li>
            ))}
          </ul>
        </RuledCard>

        <RuledCard footer={<SystemLabel>Tier 02 · Learner evidence</SystemLabel>}>
          <SystemLabel tone="accent">What Primer records</SystemLabel>
          <p className="type-h3" style={{ margin: 0 }}>
            Per-standard evidence model
          </p>
          <ul className="section-list">
            <li className="type-small">Corrected tutoring explanations</li>
            <li className="type-small">Formal assessment items</li>
            <li className="type-small">Essays, journals, oral defenses</li>
            <li className="type-small">Project artifacts and transfer checks</li>
            <li className="type-small">Date, conditions, and evaluator on every status change</li>
          </ul>
          <StateBadge status="IN_DEVELOPMENT" />
        </RuledCard>

        <RuledCard
          attention
          footer={<SystemLabel tone="attention">Tier 03 · Company efficacy</SystemLabel>}
        >
          <SystemLabel tone="attention">Not yet measured</SystemLabel>
          <p className="type-h3" style={{ margin: 0 }}>
            Primer outcomes
          </p>
          <p className="type-small text-muted" style={{ margin: 0 }}>
            {company?.safeClaim ??
              "If learners do not progress on independently scored, state-aligned standards, the system is not working."}
          </p>
          {company ? <CaveatNote blocker>{company.caveat}</CaveatNote> : null}
          <StateBadge status="EVIDENCE_NEEDED" />
        </RuledCard>
      </RuledGrid>
    </div>
  );
}
