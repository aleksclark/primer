import { RuledCard, RuledGrid } from "@/components/RuledCard";
import { SystemLabel } from "@/components/SystemLabel";
import { problemPoints } from "@/data";

/** Family/classroom comparison with founder example labeled illustrative. */
export function ProblemSection() {
  return (
    <div className="section-extras">
      <RuledGrid columns={2}>
        {problemPoints.map((point) => (
          <RuledCard
            key={point.id}
            attention={point.side === "founder-example"}
            footer={
              <SystemLabel tone={point.side === "founder-example" ? "attention" : "text"}>
                {point.side === "founder-example" ? "Illustrative" : point.side.replace("-", " ")}
              </SystemLabel>
            }
          >
            <SystemLabel>{String(problemPoints.indexOf(point) + 1).padStart(2, "0")}</SystemLabel>
            <p className="type-h3" style={{ margin: 0 }}>
              {point.title}
            </p>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              {point.body}
            </p>
          </RuledCard>
        ))}
      </RuledGrid>
    </div>
  );
}
