import { CaveatNote } from "@/components/CaveatNote";
import { MetricBlock, MetricRow } from "@/components/MetricBlock";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { SourceLinkList } from "@/components/SourceLink";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { researchClaims, sources } from "@/data";
import type { ResearchClaim, Source } from "@/data/types";

function resolveSources(ids: string[]): Source[] {
  return sources.filter((s) => ids.includes(s.id));
}

function primaryUrl(claim: ResearchClaim): string | undefined {
  const resolved = resolveSources(claim.sourceIds);
  return resolved[0]?.url;
}

function ClaimDisclosure({ claim }: { claim: ResearchClaim }) {
  const resolved = resolveSources(claim.sourceIds);
  const url = primaryUrl(claim);
  const effectLabel =
    claim.effectSizeSd != null ? `+${claim.effectSizeSd.toFixed(3)} SD · ${claim.effect}` : claim.effect;

  return (
    <details className="evidence-disclosure">
      <summary className="evidence-disclosure__summary">
        <span className="evidence-disclosure__title">
          <span className="type-small" style={{ fontWeight: 500 }}>
            {claim.study}
          </span>
          <span className="type-small text-muted">{effectLabel}</span>
        </span>
        <StateBadge status={claim.status} />
      </summary>
      <div className="evidence-disclosure__body">
        <dl className="detail-list">
          <div>
            <dt>
              <SystemLabel>Paper / year</SystemLabel>
            </dt>
            <dd className="type-small">
              {claim.study}
              {claim.year ? ` · ${claim.year}` : ""}
            </dd>
          </div>
          <div>
            <dt>
              <SystemLabel>Population</SystemLabel>
            </dt>
            <dd className="type-small">{claim.population}</dd>
          </div>
          <div>
            <dt>
              <SystemLabel>Design</SystemLabel>
            </dt>
            <dd className="type-small">{claim.design ?? "See source"}</dd>
          </div>
          <div>
            <dt>
              <SystemLabel>Effect</SystemLabel>
            </dt>
            <dd className="type-small">{effectLabel}</dd>
          </div>
          <div>
            <dt>
              <SystemLabel>Safe claim</SystemLabel>
            </dt>
            <dd className="type-small">{claim.safeClaim}</dd>
          </div>
          <div>
            <dt>
              <SystemLabel>Limitation</SystemLabel>
            </dt>
            <dd className="type-small">{claim.caveat}</dd>
          </div>
          {url ? (
            <div>
              <dt>
                <SystemLabel>URL</SystemLabel>
              </dt>
              <dd className="type-small">
                <a
                  className="source-link"
                  href={url}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <span aria-hidden="true">↗</span>
                  <span>{url}</span>
                </a>
              </dd>
            </div>
          ) : null}
        </dl>
        <SourceLinkList sources={resolved} />
      </div>
    </details>
  );
}

/**
 * Expandable research claims plus a visually distinct company-evidence panel.
 * Static safe claims remain readable without opening disclosures.
 */
export function EvidenceExplorer() {
  const research = researchClaims.filter((c) => !c.companyEvidence && c.id !== "primer-efficacy");
  const company =
    researchClaims.find((c) => c.companyEvidence || c.id === "primer-efficacy") ?? null;
  const headline = research.find((c) => c.id === "tutoring-meta-2024");

  return (
    <div className="explorer" data-explorer="evidence">
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
          <p className="type-small text-muted" style={{ margin: 0 }}>
            Expand any claim for paper, year, population, design, effect, safe claim, limitation, and
            URL. Source text stays visible without interaction.
          </p>
          <div className="evidence-list">
            {research.map((claim) => (
              <div key={claim.id} className="evidence-list__item">
                <p className="type-small" style={{ margin: 0 }}>
                  {claim.safeClaim}
                </p>
                <CaveatNote label="Note">{claim.caveat}</CaveatNote>
                <ClaimDisclosure claim={claim} />
              </div>
            ))}
          </div>
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
          className="evidence-company-panel"
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
          {company ? (
            <>
              <CaveatNote blocker>{company.caveat}</CaveatNote>
              <ClaimDisclosure claim={company} />
            </>
          ) : null}
          <StateBadge status="EVIDENCE_NEEDED" />
        </RuledCard>
      </RuledGrid>

      <p className="chart-summary">
        Research disclosures · company panel distinct · gaps marked EVIDENCE NEEDED
      </p>
    </div>
  );
}
