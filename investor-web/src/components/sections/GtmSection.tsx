import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { adoptionLadder, beachheadJobs } from "@/data";

/** Beachhead jobs plus overlapping adoption ladder. */
export function GtmSection() {
  const rungs = [...adoptionLadder].sort((a, b) => a.order - b.order);

  return (
    <div className="section-extras">
      <SystemLabel tone="accent">Two immediate family jobs</SystemLabel>
      <RuledGrid columns={2}>
        {beachheadJobs.map((job) => (
          <RuledCard key={job.id} footer={<SystemLabel>Beachhead</SystemLabel>}>
            <p className="type-h3" style={{ margin: 0 }}>
              {job.title}
            </p>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              {job.body}
            </p>
          </RuledCard>
        ))}
      </RuledGrid>

      <SystemLabel tone="accent">Adoption ladder</SystemLabel>
      <ol className="ladder-list" aria-label="Go-to-market adoption ladder">
        {rungs.map((rung) => (
          <li key={rung.id} className="ladder-list__item">
            <RuledCard
              footer={
                <div className="loop-diagram__footer">
                  <SystemLabel>
                    Rung {String(rung.order).padStart(2, "0")} /{" "}
                    {String(rungs.length).padStart(2, "0")}
                  </SystemLabel>
                  <StateBadge status={rung.status} />
                </div>
              }
            >
              <p className="type-h3" style={{ margin: 0 }}>
                {rung.name}
              </p>
              <dl className="detail-list">
                <div>
                  <dt>
                    <SystemLabel>Buyer</SystemLabel>
                  </dt>
                  <dd className="type-small">{rung.buyer}</dd>
                </div>
                <div>
                  <dt>
                    <SystemLabel>Proof</SystemLabel>
                  </dt>
                  <dd className="type-small">{rung.proof}</dd>
                </div>
                <div>
                  <dt>
                    <SystemLabel>Integration</SystemLabel>
                  </dt>
                  <dd className="type-small">{rung.integration}</dd>
                </div>
              </dl>
            </RuledCard>
          </li>
        ))}
      </ol>
    </div>
  );
}
