import { MetricBlock, MetricRow } from "@/components/MetricBlock";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { TextLink } from "@/components/TextLink";
import { productState } from "@/data";

/** LIVE / IN DEVELOPMENT / PLANNED columns from product-state inventory. */
export function CurrentStateSection() {
  const live = productState.filter((p) => p.status === "LIVE");
  const building = productState.filter((p) => p.status === "IN_DEVELOPMENT");
  const planned = productState.filter((p) => p.status === "PLANNED");

  return (
    <div className="section-extras">
      <MetricRow summary="Inputs to date — discovery only">
        <MetricBlock label="Founder hours" value="~40" hint="Discovery, not traction" />
        <MetricBlock label="Cash spent" value="~$1,000" hint="Inputs to date" />
        <MetricBlock label="Family use" value="1 week" hint="Discovery only" />
      </MetricRow>

      <RuledGrid columns={3}>
        <StateColumn title="Live" items={live} empty="None" />
        <StateColumn title="In development" items={building} empty="None" />
        <StateColumn title="Planned" items={planned} empty="None" />
      </RuledGrid>
    </div>
  );
}

function StateColumn({
  title,
  items,
  empty,
}: {
  title: string;
  items: typeof productState;
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
