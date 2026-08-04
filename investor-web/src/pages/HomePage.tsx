import {
  InvestorCTA,
  MetricBlock,
  MetricRow,
  RuledCard,
  RuledGrid,
  SectionFrame,
  SystemLabel,
} from "@/components";
import { investorData, sections, sources } from "@/data";

function sourcesFor(citationIds: string[]) {
  return sources.filter((s) => citationIds.includes(s.id));
}

/**
 * Primary investor page — familiar section order with placeholder frames.
 * Full narrative content lands in Phase 2; this proves the shell.
 */
export function HomePage() {
  const total = sections.length;

  return (
    <>
      {sections.map((section, i) => (
        <SectionFrame
          key={section.id}
          section={section}
          index={i + 1}
          total={total}
          sources={sourcesFor(section.citationIds)}
        >
          {section.id === "thesis" ? <ThesisExtras /> : null}
          {section.id === "proof" ? <EvidenceExtras /> : null}
          {section.id === "market" ? <MarketExtras /> : null}
          {section.id === "current-state" ? <StateExtras /> : null}
          {section.id === "round" ? <RoundExtras /> : null}
        </SectionFrame>
      ))}

      <div className="site-container" style={{ paddingBottom: "var(--primer-space-12)" }}>
        <InvestorCTA
          headline="Ready to discuss the round."
          body="Working target: $3.5M pre-seed for 18 months of seed-readiness work. No student data in demos — synthetic artifacts only."
          primaryHref="#contact"
          secondaryLabel="View product state"
          secondaryHref="/demo"
        />
      </div>
    </>
  );
}

function ThesisExtras() {
  return (
    <MetricRow summary="Positioning summary for Primer investor thesis">
      <MetricBlock label="Product class" value="LMS" hint="Instructional, not administrative" />
      <MetricBlock label="Grades" value="5–8" hint="Family first, school ready" />
      <MetricBlock label="Entry" value="Family" hint="Supplementary school path" />
    </MetricRow>
  );
}

function EvidenceExtras() {
  return (
    <RuledGrid columns={3}>
      <RuledCard
        footer={<SystemLabel>Research basis</SystemLabel>}
      >
        <SystemLabel tone="accent">Tier 01</SystemLabel>
        <p className="type-h3" style={{ margin: 0 }}>
          Prior evidence
        </p>
        <p className="type-small text-muted" style={{ margin: 0 }}>
          Tutoring and mastery research with explicit caveats. Not a Primer outcome.
        </p>
      </RuledCard>
      <RuledCard footer={<SystemLabel>Learner evidence</SystemLabel>}>
        <SystemLabel tone="accent">Tier 02</SystemLabel>
        <p className="type-h3" style={{ margin: 0 }}>
          What Primer records
        </p>
        <p className="type-small text-muted" style={{ margin: 0 }}>
          Per-standard evidence model — implementation in progress.
        </p>
      </RuledCard>
      <RuledCard attention footer={<SystemLabel tone="attention">Company efficacy</SystemLabel>}>
        <SystemLabel tone="attention">Tier 03</SystemLabel>
        <p className="type-h3" style={{ margin: 0 }}>
          Not yet measured
        </p>
        <p className="type-small text-muted" style={{ margin: 0 }}>
          Primer learning outcomes remain evidence needed. This card stays empty of results.
        </p>
      </RuledCard>
    </RuledGrid>
  );
}

function MarketExtras() {
  const layers = investorData.marketLayers.slice(0, 4);
  return (
    <RuledGrid columns={2}>
      {layers.map((layer) => (
        <RuledCard
          key={layer.id}
          footer={<SystemLabel>{layer.sourceYear} · {layer.observedOrModeled}</SystemLabel>}
        >
          <SystemLabel>{layer.displayLabel}</SystemLabel>
          <p className="type-h3" style={{ margin: 0 }}>
            ${(layer.modeledCeiling / 1_000_000_000).toFixed(2)}B
          </p>
          <p className="type-small text-muted" style={{ margin: 0 }}>
            {layer.learners.toLocaleString()} learners · ${layer.annualRevenuePerLearner}/yr · not additive
          </p>
        </RuledCard>
      ))}
    </RuledGrid>
  );
}

function StateExtras() {
  const live = investorData.productState.filter((p) => p.status === "LIVE");
  const building = investorData.productState.filter((p) => p.status === "IN_DEVELOPMENT");
  const planned = investorData.productState.filter((p) => p.status === "PLANNED");

  return (
    <RuledGrid columns={3}>
      <RuledCard footer={<SystemLabel tone="accent">{live.length} live</SystemLabel>}>
        <SystemLabel tone="accent">Live</SystemLabel>
        <ul style={{ margin: 0, paddingLeft: "1.1rem" }}>
          {live.slice(0, 4).map((p) => (
            <li key={p.id} className="type-small">
              {p.name}
            </li>
          ))}
        </ul>
      </RuledCard>
      <RuledCard footer={<SystemLabel>{building.length} in development</SystemLabel>}>
        <SystemLabel>In development</SystemLabel>
        <ul style={{ margin: 0, paddingLeft: "1.1rem" }}>
          {(building.length ? building : planned).slice(0, 4).map((p) => (
            <li key={p.id} className="type-small">
              {p.name}
            </li>
          ))}
        </ul>
      </RuledCard>
      <RuledCard footer={<SystemLabel>{planned.length} planned</SystemLabel>}>
        <SystemLabel>Planned</SystemLabel>
        <ul style={{ margin: 0, paddingLeft: "1.1rem" }}>
          {planned.slice(0, 4).map((p) => (
            <li key={p.id} className="type-small">
              {p.name}
            </li>
          ))}
        </ul>
      </RuledCard>
    </RuledGrid>
  );
}

function RoundExtras() {
  const plan = investorData.fundingPlan;
  return (
    <MetricRow summary="Working pre-seed target summary">
      <MetricBlock
        label="Ask"
        value={`$${(plan.amountUsd / 1_000_000).toFixed(1)}M`}
        hint={plan.roundName}
      />
      <MetricBlock label="Runway" value={`${plan.runwayMonths} mo`} hint="Seed-readiness window" />
      <MetricBlock
        label="Team"
        value={plan.teamSize}
        hint={`$${(plan.annualCashSalaryUsd / 1000).toFixed(0)}k cash each`}
      />
    </MetricRow>
  );
}
