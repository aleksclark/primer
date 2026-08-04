import { useState } from "react";
import { CaveatNote } from "@/components/CaveatNote";
import { RuledCard } from "@/components/RuledCard";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { pricingTiers } from "@/data";
import type { PackageTier } from "@/data/types";
import { analytics } from "@/lib/analytics";
import { formatUsd } from "@/lib/format";
import { cn } from "@/lib/cn";

const FAMILY_ORDER: PackageTier[] = ["base", "core", "premier"];

const COMPARE_ROWS: {
  id: string;
  label: string;
  value: (tier: (typeof pricingTiers)[number]) => string;
}[] = [
  {
    id: "buyer",
    label: "Primary buyer",
    value: (t) => t.primaryBuyers[0] ?? "—",
  },
  {
    id: "scope",
    label: "Scope",
    value: (t) => t.scope[0] ?? "—",
  },
  {
    id: "subjects",
    label: "Included subjects",
    value: (t) => t.includedSubjects ?? t.scope[0] ?? "—",
  },
  {
    id: "monthly",
    label: "Monthly price",
    value: (t) => formatUsd(t.monthlyPriceUsd),
  },
  {
    id: "annual",
    label: "Annual ARPU",
    value: (t) => formatUsd(t.annualRevenuePerLearnerUsd),
  },
  {
    id: "planning",
    label: "Planning / assessment",
    value: (t) => t.planningAssessment ?? "—",
  },
  {
    id: "projects",
    label: "Projects / content / reporting",
    value: (t) => t.projectsContentReporting ?? "—",
  },
  {
    id: "support",
    label: "Support level",
    value: (t) => t.supportLevel ?? t.includedUsage,
  },
  {
    id: "cogs-target",
    label: "COGS target",
    value: (t) => t.cogsTarget ?? "Evidence needed",
  },
  {
    id: "cogs-state",
    label: "COGS current state",
    value: (t) => t.cogsState.replaceAll("_", " "),
  },
  {
    id: "expansion",
    label: "Expansion validation",
    value: (t) => t.expansionValidation ?? t.notes ?? "—",
  },
];

/**
 * Three-column ruled comparison of Base / Core / Premier.
 * No “most popular” consumer pricing chrome.
 */
export function PackageLadderExplorer() {
  const tiers = FAMILY_ORDER.map((id) => pricingTiers.find((t) => t.id === id)).filter(
    (t): t is (typeof pricingTiers)[number] => t != null,
  );
  const [selected, setSelected] = useState<PackageTier>("base");
  const active = tiers.find((t) => t.id === selected) ?? tiers[0];

  function selectTier(id: PackageTier) {
    setSelected(id);
    analytics.packageCompare(id);
  }

  return (
    <div className="explorer" data-explorer="package-ladder">
      <div className="explorer__toolbar">
        <div className="explorer__toolbar-text">
          <SystemLabel tone="accent">Package ladder</SystemLabel>
          <p className="type-small text-muted" style={{ margin: 0 }}>
            Compare buyer, scope, ARPU, support, and COGS posture. Prices are working hypotheses, not
            current revenue.
          </p>
        </div>
      </div>

      <div
        className="package-ladder-cols"
        role="list"
        aria-label="Base, Core, and Premier comparison"
      >
        {tiers.map((tier) => {
          const isActive = tier.id === selected;
          return (
            <button
              key={tier.id}
              type="button"
              role="listitem"
              className={cn("package-ladder-col", isActive && "package-ladder-col--active")}
              aria-pressed={isActive}
              onClick={() => selectTier(tier.id)}
            >
              <SystemLabel tone={isActive ? "accent" : "text"}>{tier.name}</SystemLabel>
              <p className="type-h3" style={{ margin: 0 }}>
                {formatUsd(tier.monthlyPriceUsd)}
                <span className="type-small text-muted">/mo</span>
              </p>
              <p className="type-small text-muted" style={{ margin: 0 }}>
                {formatUsd(tier.annualRevenuePerLearnerUsd)} annual ARPU
              </p>
              <StateBadge status={tier.status} />
            </button>
          );
        })}
      </div>

      <div className="explorer-table-wrap" role="region" aria-label="Package ladder comparison table">
        <table className="explorer-table package-compare-table">
          <thead>
            <tr>
              <th scope="col">
                <SystemLabel>Dimension</SystemLabel>
              </th>
              {tiers.map((tier) => (
                <th
                  key={tier.id}
                  scope="col"
                  data-active={tier.id === selected ? "true" : "false"}
                >
                  <SystemLabel tone={tier.id === selected ? "accent" : undefined}>
                    {tier.name}
                  </SystemLabel>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {COMPARE_ROWS.map((row) => (
              <tr key={row.id}>
                <th scope="row" className="type-small">
                  {row.label}
                </th>
                {tiers.map((tier) => (
                  <td
                    key={tier.id}
                    className="type-small"
                    data-active={tier.id === selected ? "true" : "false"}
                  >
                    {row.value(tier)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {active ? (
        <RuledCard
          selected
          footer={
            <div className="loop-diagram__footer">
              <SystemLabel tone="accent">{active.name} detail</SystemLabel>
              <StateBadge status={active.cogsState} />
            </div>
          }
        >
          <SystemLabel tone="accent">Selected tier</SystemLabel>
          <p className="type-small" style={{ margin: 0 }}>
            {active.competitiveFrame}
          </p>
          <ul className="section-list section-list--compact">
            {active.scope.map((item) => (
              <li key={item} className="type-small">
                {item}
              </li>
            ))}
          </ul>
          <SystemLabel>Buyers</SystemLabel>
          <ul className="section-list section-list--compact">
            {active.primaryBuyers.map((b) => (
              <li key={b} className="type-small">
                {b}
              </li>
            ))}
          </ul>
          {active.notes ? <CaveatNote>{active.notes}</CaveatNote> : null}
        </RuledCard>
      ) : null}

      <p className="chart-summary">
        Three-column ruled comparison · no popular-tier chrome · institutional row lives separately
      </p>
    </div>
  );
}
