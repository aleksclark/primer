import { useMemo, useState } from "react";
import { MetricBlock, MetricRow } from "@/components/MetricBlock";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { TextLink } from "@/components/TextLink";
import { productState } from "@/data";
import type { ProductCapability, ProductCapabilityCategory } from "@/data/types";
import { cn } from "@/lib/cn";

const FILTERS: { id: ProductCapabilityCategory | "all"; label: string }[] = [
  { id: "all", label: "All" },
  { id: "learner", label: "Learner" },
  { id: "parent", label: "Parent / educator" },
  { id: "lms", label: "LMS core" },
  { id: "media", label: "Media / projects" },
  { id: "school", label: "School integration" },
  { id: "compliance", label: "Compliance / research" },
];

function matchesFilter(
  item: ProductCapability,
  filter: ProductCapabilityCategory | "all",
): boolean {
  if (filter === "all") return true;
  return (item.categories ?? []).includes(filter);
}

/**
 * Product-state inventory with optional facet filters.
 * Legend always remains; planned work never appears live.
 */
export function ProductStateExplorer() {
  const [filter, setFilter] = useState<ProductCapabilityCategory | "all">("all");

  const filtered = useMemo(
    () => productState.filter((p) => matchesFilter(p, filter)),
    [filter],
  );

  const live = filtered.filter((p) => p.status === "LIVE");
  const building = filtered.filter((p) => p.status === "IN_DEVELOPMENT");
  const planned = filtered.filter((p) => p.status === "PLANNED");

  return (
    <div className="explorer" data-explorer="product-state">
      <MetricRow summary="Inputs to date — discovery only">
        <MetricBlock label="Founder hours" value="~40" hint="Discovery, not traction" />
        <MetricBlock label="Cash spent" value="~$1,000" hint="Inputs to date" />
        <MetricBlock label="Family use" value="1 week" hint="Discovery only" />
      </MetricRow>

      <div className="explorer__toolbar">
        <div className="explorer__toolbar-text">
          <SystemLabel tone="accent">Product-state explorer</SystemLabel>
          <p className="type-small text-muted" style={{ margin: 0 }}>
            Filter by surface. Status legend stays visible so planned work cannot look live.
          </p>
        </div>
      </div>

      <div
        className="state-legend"
        role="group"
        aria-label="Product status legend"
      >
        <SystemLabel>Legend</SystemLabel>
        <div className="state-legend__badges">
          <StateBadge status="LIVE" />
          <StateBadge status="IN_DEVELOPMENT" />
          <StateBadge status="PLANNED" />
        </div>
      </div>

      <div
        className="explorer-filter-row"
        role="toolbar"
        aria-label="Filter product capabilities"
      >
        {FILTERS.map((f) => {
          const pressed = filter === f.id;
          return (
            <button
              key={f.id}
              type="button"
              className={cn("explorer-filter", pressed && "explorer-filter--active")}
              aria-pressed={pressed}
              onClick={() => setFilter(f.id)}
            >
              {f.label}
            </button>
          );
        })}
      </div>

      <RuledGrid columns={3}>
        <StateColumn title="Live" items={live} empty="No live items in this filter" />
        <StateColumn
          title="In development"
          items={building}
          empty="No in-development items in this filter"
        />
        <StateColumn title="Planned" items={planned} empty="No planned items in this filter" />
      </RuledGrid>

      <p className="chart-summary" aria-live="polite">
        Showing {filtered.length} of {productState.length} capabilities · filter {filter} · legend
        always visible
      </p>
    </div>
  );
}

function StateColumn({
  title,
  items,
  empty,
}: {
  title: string;
  items: ProductCapability[];
  empty: string;
}) {
  return (
    <RuledCard
      footer={
        <SystemLabel>
          {items.length} {title.toLowerCase()}
        </SystemLabel>
      }
    >
      <SystemLabel tone={title === "Live" ? "accent" : "text"}>{title}</SystemLabel>
      {items.length === 0 ? (
        <p className="type-small text-muted" style={{ margin: 0 }}>
          {empty}
        </p>
      ) : (
        <ul className="state-column-list">
          {items.map((item) => (
            <li key={item.id} className="state-column-list__item">
              <div className="state-column-list__head">
                <span className="type-small" style={{ fontWeight: 500 }}>
                  {item.name}
                </span>
                <StateBadge status={item.status} />
              </div>
              <p className="type-small text-muted" style={{ margin: 0 }}>
                {item.description}
              </p>
              {item.categories && item.categories.length > 0 ? (
                <p className="type-label text-muted" style={{ margin: 0 }}>
                  {item.categories.join(" · ")}
                </p>
              ) : null}
              {item.nearTermMilestone ? (
                <p className="type-small text-muted" style={{ margin: 0 }}>
                  Milestone: {item.nearTermMilestone}
                </p>
              ) : null}
              {item.artifactUrl ? (
                <TextLink href={item.artifactUrl}>{item.artifactLabel ?? "Artifact"}</TextLink>
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </RuledCard>
  );
}
