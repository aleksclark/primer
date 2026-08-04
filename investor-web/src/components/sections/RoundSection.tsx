import { CaveatNote } from "@/components/CaveatNote";
import { MetricBlock, MetricRow } from "@/components/MetricBlock";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { fundingPlan, pricingTiers, seedScorecard } from "@/data";

function formatUsd(n: number): string {
  if (n >= 1_000_000) return `$${(n / 1_000_000).toFixed(n % 1_000_000 === 0 ? 0 : 1)}M`;
  if (n >= 1_000) return `$${(n / 1_000).toFixed(0)}k`;
  return `$${n}`;
}

function formatMetricValue(
  value: (typeof seedScorecard)[0]["current"],
  unit: string,
): string {
  if (value === "NOT_YET_MEASURED") return "NOT YET MEASURED";
  if (unit === "percent") return `${value}%`;
  if (unit.startsWith("usd")) return formatUsd(value);
  return String(value);
}

/**
 * Business model ladder + ask + use-of-funds + seed scorecard.
 * Salary appears here in the model, not in founder defensive copy.
 */
export function RoundSection() {
  const plan = fundingPlan;
  const familyTiers = pricingTiers.filter((t) => t.id !== "institutional");
  const institutional = pricingTiers.find((t) => t.id === "institutional");
  const scorecard = seedScorecard;

  return (
    <div className="section-extras">
      <MetricRow summary="Working pre-seed target summary">
        <MetricBlock
          label="Ask"
          value={formatUsd(plan.amountUsd)}
          hint={plan.roundName}
        />
        <MetricBlock label="Runway" value={`${plan.runwayMonths} mo`} hint="Seed-readiness window" />
        <MetricBlock
          label="Team"
          value={plan.teamSize}
          hint={`${formatUsd(plan.annualCashSalaryUsd)} cash each + benefits`}
        />
      </MetricRow>

      <SystemLabel tone="accent">Package ladder · working hypotheses</SystemLabel>
      <RuledGrid columns={3}>
        {familyTiers.map((tier) => (
          <RuledCard
            key={tier.id}
            footer={
              <div className="loop-diagram__footer">
                <SystemLabel>{formatUsd(tier.annualRevenuePerLearnerUsd)}/yr</SystemLabel>
                <StateBadge status={tier.status} />
              </div>
            }
          >
            <SystemLabel tone="accent">{tier.name}</SystemLabel>
            <p className="type-h3" style={{ margin: 0 }}>
              ${tier.monthlyPriceUsd}
              <span className="type-small text-muted">/mo</span>
            </p>
            <ul className="section-list section-list--compact">
              {tier.scope.slice(0, 5).map((item) => (
                <li key={item} className="type-small">
                  {item}
                </li>
              ))}
            </ul>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              {tier.competitiveFrame}
            </p>
          </RuledCard>
        ))}
      </RuledGrid>

      {institutional ? (
        <RuledCard footer={<SystemLabel>Institutional · separate from family ladder</SystemLabel>}>
          <SystemLabel tone="accent">{institutional.name}</SystemLabel>
          <p className="type-h3" style={{ margin: 0 }}>
            {formatUsd(institutional.annualRevenuePerLearnerUsd)}
            <span className="type-small text-muted"> / supported learner / year</span>
          </p>
          <p className="type-small text-muted" style={{ margin: 0 }}>
            {institutional.competitiveFrame}
          </p>
          <CaveatNote>{institutional.notes}</CaveatNote>
        </RuledCard>
      ) : null}

      <SystemLabel tone="accent">Use of funds · 18 months</SystemLabel>
      <p className="type-small text-muted prose-measure" style={{ margin: 0 }}>
        {plan.compensationAnnotation}
      </p>
      <div className="funds-table-wrap" role="region" aria-label="Use of funds">
        <table className="funds-table">
          <thead>
            <tr>
              <th scope="col">
                <SystemLabel>Category</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>Lean</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>Full</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>Driver</SystemLabel>
              </th>
            </tr>
          </thead>
          <tbody>
            {plan.lineItems.map((line) => (
              <tr key={line.id}>
                <th scope="row" className="type-small">
                  {line.category}
                </th>
                <td className="type-small">{formatUsd(line.leanUsd)}</td>
                <td className="type-small">{formatUsd(line.fullUsd)}</td>
                <td className="type-small text-muted">{line.driver}</td>
              </tr>
            ))}
            <tr data-total="true">
              <th scope="row" className="type-small">
                Contingency ({Math.round(plan.contingencyRate * 100)}%)
              </th>
              <td className="type-small" colSpan={2}>
                {formatUsd(plan.contingencyUsd)}
              </td>
              <td className="type-small text-muted">On people + non-payroll base</td>
            </tr>
            <tr data-total="true">
              <th scope="row" className="type-small">
                Working ask
              </th>
              <td className="type-small" colSpan={2}>
                {formatUsd(plan.totalUsd)}
              </td>
              <td className="type-small text-muted">{plan.instrument}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <SystemLabel tone="accent">Seed-readiness scorecard</SystemLabel>
      <div className="scorecard-wrap" role="region" aria-label="Seed readiness scorecard">
        <table className="scorecard-table">
          <thead>
            <tr>
              <th scope="col">
                <SystemLabel>Metric</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>Floor</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>Target</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>Current</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>Status</SystemLabel>
              </th>
            </tr>
          </thead>
          <tbody>
            {scorecard.map((metric) => (
              <tr key={metric.id}>
                <th scope="row" className="type-small">
                  {metric.name}
                  <span className="text-muted"> · {metric.definition}</span>
                </th>
                <td className="type-small">{formatMetricValue(metric.floor, metric.unit)}</td>
                <td className="type-small">{formatMetricValue(metric.target, metric.unit)}</td>
                <td className="type-small">
                  <span className={metric.current === "NOT_YET_MEASURED" ? "text-attention" : undefined}>
                    {formatMetricValue(metric.current, metric.unit)}
                  </span>
                </td>
                <td>
                  <StateBadge status={metric.status} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <ul className="scorecard-stack" aria-label="Seed readiness metrics, stacked">
        {scorecard.map((metric) => (
          <li key={metric.id} className="scorecard-stack__card">
            <div className="loop-diagram__footer">
              <SystemLabel>{metric.name}</SystemLabel>
              <StateBadge status={metric.status} />
            </div>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              {metric.definition}
            </p>
            <dl className="detail-list detail-list--inline">
              <div>
                <dt>
                  <SystemLabel>Floor</SystemLabel>
                </dt>
                <dd className="type-small">{formatMetricValue(metric.floor, metric.unit)}</dd>
              </div>
              <div>
                <dt>
                  <SystemLabel>Target</SystemLabel>
                </dt>
                <dd className="type-small">{formatMetricValue(metric.target, metric.unit)}</dd>
              </div>
              <div>
                <dt>
                  <SystemLabel>Current</SystemLabel>
                </dt>
                <dd className="type-small text-attention">
                  {formatMetricValue(metric.current, metric.unit)}
                </dd>
              </div>
            </dl>
            <CaveatNote>{metric.caveat}</CaveatNote>
          </li>
        ))}
      </ul>

      <SystemLabel tone="accent">Milestones the round should produce</SystemLabel>
      <ol className="section-list">
        {plan.milestones.map((m) => (
          <li key={m} className="type-small">
            {m}
          </li>
        ))}
      </ol>

      {plan.publicationBlockers.length > 0 ? (
        <div>
          {plan.publicationBlockers.map((b) => (
            <CaveatNote key={b} blocker>
              {b}
            </CaveatNote>
          ))}
        </div>
      ) : null}
    </div>
  );
}
