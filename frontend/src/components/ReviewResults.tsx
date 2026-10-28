import { useMemo, useState } from "react";
import type { Review } from "../api/types";
import {
  NO_FILTERS,
  filterFindings,
  hasActiveFilters,
  toggleSeverity,
  uniqueSkills,
  type FindingFilters,
  type SkillGroup,
} from "../lib/findings";
import { FindingList } from "./FindingList";
import { EmptyState } from "./PageState";
import { PrSummary } from "./PrSummary";
import { SeverityTiles } from "./SeverityTiles";
import "./ReviewResults.css";

const GROUPS: { group: SkillGroup; label: string }[] = [
  { group: "language", label: "Language" },
  { group: "design", label: "Design" },
];

export function ReviewResults({ review }: { review: Review }) {
  const [filters, setFilters] = useState<FindingFilters>(NO_FILTERS);
  const skills = useMemo(() => uniqueSkills(review.findings), [review.findings]);
  const visible = useMemo(() => filterFindings(review.findings, filters), [review.findings, filters]);

  const total = review.findings.length;

  return (
    <>
      <PrSummary pullRequest={review.pullRequest} />

      {total === 0 ? (
        <EmptyState title="No issues found">
          None of the checks that apply to this pull request flagged anything.
        </EmptyState>
      ) : (
        <>
          <SeverityTiles
            counts={review.counts}
            selected={filters.severities}
            onToggle={(severity) => setFilters((f) => toggleSeverity(f, severity))}
          />

          <div className="filter-bar">
            <label className="filter-bar__field">
              <span>Skill</span>
              <select
                className="input"
                value={filters.skillId ?? ""}
                onChange={(e) => setFilters((f) => ({ ...f, skillId: e.target.value || null }))}
              >
                <option value="">All skills ({total})</option>
                {GROUPS.map(({ group, label }) => {
                  const options = skills.filter((s) => s.group === group);
                  if (options.length === 0) return null;
                  return (
                    <optgroup key={group} label={label}>
                      {options.map((skill) => (
                        <option key={skill.id} value={skill.id}>
                          {skill.name} ({skill.count})
                        </option>
                      ))}
                    </optgroup>
                  );
                })}
              </select>
            </label>
            <p className="filter-bar__count" role="status">
              Showing {visible.length} of {total} {total === 1 ? "finding" : "findings"}
            </p>
            {hasActiveFilters(filters) && (
              <button type="button" className="button button--ghost" onClick={() => setFilters(NO_FILTERS)}>
                Clear filters
              </button>
            )}
          </div>

          {visible.length === 0 ? (
            <EmptyState title="No findings match these filters">Try clearing a filter.</EmptyState>
          ) : (
            <FindingList reviewId={review.id} findings={visible} />
          )}
        </>
      )}
    </>
  );
}
