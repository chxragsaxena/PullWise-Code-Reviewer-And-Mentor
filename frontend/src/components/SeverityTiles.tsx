import { SEVERITIES, type Severity, type SeverityCounts } from "../api/types";
import { SEVERITY_LABEL } from "../lib/severity";
import "./SeverityTiles.css";

interface SeverityTilesProps {
  counts: SeverityCounts;
  selected: readonly Severity[];
  onToggle: (severity: Severity) => void;
}

/** Counts per severity. Each tile is also a filter toggle for the findings list. */
export function SeverityTiles({ counts, selected, onToggle }: SeverityTilesProps) {
  return (
    <div className="tiles" role="group" aria-label="Filter findings by severity">
      {SEVERITIES.map((severity) => (
        <button
          key={severity}
          type="button"
          className={`tile tile--${severity}`}
          aria-pressed={selected.includes(severity)}
          onClick={() => onToggle(severity)}
        >
          <span className="tile__count">{counts[severity]}</span>
          <span className="tile__label">{SEVERITY_LABEL[severity]}</span>
        </button>
      ))}
    </div>
  );
}
