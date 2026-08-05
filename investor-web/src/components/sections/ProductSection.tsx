import { RuledCard } from "@/components/RuledCard";
import { StateBadge } from "@/components/StateBadge";
import { SystemLabel } from "@/components/SystemLabel";
import { instructionalLoop } from "@/data";

/**
 * Static instructional-loop diagram.
 * Ordered list is the accessible equivalent; LLM is not visually central.
 */
export function ProductSection() {
  const nodes = [...instructionalLoop].sort((a, b) => a.order - b.order);

  return (
    <div className="section-extras">
      <SystemLabel tone="accent">Instructional loop</SystemLabel>
      <p className="type-small text-muted prose-measure" style={{ margin: 0 }}>
        Adult authority and longitudinal evidence frame the system. Specialist tutors and habit
        checks operate inside that memory — not as a free-standing chat product.
      </p>

      <ol className="loop-diagram" aria-label="Instructional loop steps">
        {nodes.map((node) => (
          <li key={node.id} className="loop-diagram__node">
            <RuledCard
              footer={
                <div className="loop-diagram__footer">
                  <SystemLabel>
                    {String(node.order).padStart(2, "0")} / {String(nodes.length).padStart(2, "0")}
                  </SystemLabel>
                  <StateBadge status={node.status} />
                </div>
              }
            >
              <p className="type-h3" style={{ margin: 0 }}>
                {node.name}
              </p>
              <p className="type-small text-muted" style={{ margin: 0 }}>
                {node.summary}
              </p>
            </RuledCard>
          </li>
        ))}
      </ol>

      <p className="chart-summary">
        Static diagram · every node carries a product-state badge · most loop elements planned or in
        development
      </p>
    </div>
  );
}
