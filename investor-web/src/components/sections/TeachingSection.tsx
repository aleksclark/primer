import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { teachingExamples } from "@/data";

/** Four-to-six ruled teaching examples — mechanism, not efficacy. */
export function TeachingSection() {
  const examples = [...teachingExamples].sort((a, b) => a.order - b.order);

  return (
    <div className="section-extras">
      <RuledGrid columns={2}>
        {examples.map((ex) => (
          <RuledCard
            key={ex.id}
            footer={
              <div className="loop-diagram__footer">
                <SystemLabel>{String(ex.order).padStart(2, "0")}</SystemLabel>
                <StateBadge status={ex.status} />
              </div>
            }
          >
            <SystemLabel tone="accent">{ex.principle}</SystemLabel>
            <p className="type-h3" style={{ margin: 0 }}>
              {ex.title}
            </p>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              {ex.example}
            </p>
          </RuledCard>
        ))}
      </RuledGrid>
    </div>
  );
}
