import { CaveatNote } from "@/components/CaveatNote";
import {
  PackageLadderExplorer,
  SeedScorecardExplorer,
} from "@/components/explorers";
import { MetricBlock, MetricRow } from "@/components/MetricBlock";
import { RuledCard } from "@/components/RuledCard";
import { SystemLabel } from "@/components/SystemLabel";
import { fundingPlan, pricingTiers } from "@/data";
import { formatUsd } from "@/lib/format";

/**
 * Business model ladder + ask + use-of-funds + seed scorecard.
 * Salary appears here in the model, not in founder defensive copy.
 */
export function RoundSection() {
  const plan = fundingPlan;
  const institutional = pricingTiers.find((t) => t.id === "institutional");

  return (
    <div className="section-extras">
      <MetricRow summary="Working pre-seed target summary">
        <MetricBlock
          label="Ask"
          value={formatUsd(plan.amountUsd, { compact: true })}
          hint={plan.roundName}
        />
        <MetricBlock label="Runway" value={`${plan.runwayMonths} mo`} hint="Seed-readiness window" />
        <MetricBlock
          label="Team"
          value={plan.teamSize}
          hint={`${formatUsd(plan.annualCashSalaryUsd)} cash each + benefits`}
        />
      </MetricRow>

      <PackageLadderExplorer />

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
                <td className="type-small">{formatUsd(line.leanUsd, { compact: true })}</td>
                <td className="type-small">{formatUsd(line.fullUsd, { compact: true })}</td>
                <td className="type-small text-muted">{line.driver}</td>
              </tr>
            ))}
            <tr data-total="true">
              <th scope="row" className="type-small">
                Contingency ({Math.round(plan.contingencyRate * 100)}%)
              </th>
              <td className="type-small" colSpan={2}>
                {formatUsd(plan.contingencyUsd, { compact: true })}
              </td>
              <td className="type-small text-muted">On people + non-payroll base</td>
            </tr>
            <tr data-total="true">
              <th scope="row" className="type-small">
                Working ask
              </th>
              <td className="type-small" colSpan={2}>
                {formatUsd(plan.totalUsd, { compact: true })}
              </td>
              <td className="type-small text-muted">{plan.instrument}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <SeedScorecardExplorer />

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
