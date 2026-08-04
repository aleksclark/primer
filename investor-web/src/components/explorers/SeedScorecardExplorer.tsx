import { CaveatNote } from "@/components/CaveatNote";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { seedScorecard } from "@/data";
import { formatMetricValue } from "@/lib/format";

/**
 * Floor / target / current scorecard.
 * Unknown currents render NOT YET MEASURED — never zero.
 */
export function SeedScorecardExplorer() {
  return (
    <div className="explorer" data-explorer="seed-scorecard">
      <div className="explorer__toolbar">
        <div className="explorer__toolbar-text">
          <SystemLabel tone="accent">Seed-readiness scorecard</SystemLabel>
          <p className="type-small text-muted" style={{ margin: 0 }}>
            Month-18 floor and target. Current unknowns stay explicit — never coerced to zero.
          </p>
        </div>
      </div>

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
            {seedScorecard.map((metric) => {
              const currentLabel = formatMetricValue(metric.current, metric.unit);
              const isUnknown = metric.current === "NOT_YET_MEASURED";
              return (
                <tr key={metric.id}>
                  <th scope="row" className="type-small">
                    {metric.name}
                    <span className="text-muted"> · {metric.definition}</span>
                  </th>
                  <td className="type-small">
                    {formatMetricValue(metric.floor, metric.unit)}
                  </td>
                  <td className="type-small">
                    {formatMetricValue(metric.target, metric.unit)}
                  </td>
                  <td className="type-small">
                    <span className={isUnknown ? "text-attention" : undefined}>{currentLabel}</span>
                  </td>
                  <td>
                    <StateBadge status={metric.status} />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <ul className="scorecard-stack" aria-label="Seed readiness metrics, stacked">
        {seedScorecard.map((metric) => {
          const isUnknown = metric.current === "NOT_YET_MEASURED";
          return (
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
                  <dd className="type-small">
                    {formatMetricValue(metric.floor, metric.unit)}
                  </dd>
                </div>
                <div>
                  <dt>
                    <SystemLabel>Target</SystemLabel>
                  </dt>
                  <dd className="type-small">
                    {formatMetricValue(metric.target, metric.unit)}
                  </dd>
                </div>
                <div>
                  <dt>
                    <SystemLabel>Current</SystemLabel>
                  </dt>
                  <dd className={`type-small${isUnknown ? " text-attention" : ""}`}>
                    {formatMetricValue(metric.current, metric.unit)}
                  </dd>
                </div>
              </dl>
              <CaveatNote>{metric.caveat}</CaveatNote>
            </li>
          );
        })}
      </ul>

      <p className="chart-summary">
        Floor · target · current · unknowns labeled NOT YET MEASURED · includes Core/Premier
        expansion tests
      </p>
    </div>
  );
}
