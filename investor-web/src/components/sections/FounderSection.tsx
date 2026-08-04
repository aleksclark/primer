import { MetricBlock, MetricRow } from "@/components/MetricBlock";
import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { SystemLabel } from "@/components/SystemLabel";
import { founderProof, teamPlan } from "@/data";

/** Founder timeline + compact team plan. Salary stays in use-of-funds. */
export function FounderSection() {
  const timeline = founderProof;
  const ebackpack = founderProof.find((e) => e.id === "ebackpack-lms");

  return (
    <div className="section-extras">
      {ebackpack?.metrics ? (
        <MetricRow summary="eBackpack execution metrics, founder-attested">
          {ebackpack.metrics.map((m) => (
            <MetricBlock key={m.label} label={m.label} value={m.value} hint="Founder-attested" />
          ))}
        </MetricRow>
      ) : null}

      <SystemLabel tone="accent">Timeline</SystemLabel>
      <ol className="timeline-list" aria-label="Founder proof timeline">
        {timeline.map((event) => (
          <li key={event.id} className="timeline-list__item">
            <RuledCard
              footer={
                <div className="loop-diagram__footer">
                  <SystemLabel>{event.period}</SystemLabel>
                  <SystemLabel>{event.proofType.replace("_", " ")}</SystemLabel>
                </div>
              }
            >
              <p className="type-h3" style={{ margin: 0 }}>
                {event.title}
              </p>
              <p className="type-small text-muted" style={{ margin: 0 }}>
                {event.summary}
              </p>
              {event.metrics && event.id !== "ebackpack-lms" ? (
                <ul className="section-list section-list--compact">
                  {event.metrics.map((m) => (
                    <li key={m.label} className="type-small">
                      {m.label}: {m.value}
                    </li>
                  ))}
                </ul>
              ) : null}
            </RuledCard>
          </li>
        ))}
      </ol>

      <SystemLabel tone="accent">18-month team plan</SystemLabel>
      <RuledGrid columns={3}>
        {teamPlan.map((role) => (
          <RuledCard
            key={role.id}
            footer={
              <SystemLabel tone={role.presence === "current" ? "accent" : "text"}>
                {role.presence === "current" ? "Current" : "Planned hire"}
              </SystemLabel>
            }
          >
            <p className="type-h3" style={{ margin: 0 }}>
              {role.title}
            </p>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              {role.timing}
            </p>
            <ul className="section-list section-list--compact">
              {role.ownership.map((item) => (
                <li key={item} className="type-small">
                  {item}
                </li>
              ))}
            </ul>
          </RuledCard>
        ))}
      </RuledGrid>
    </div>
  );
}
