import { useMemo, useState } from "react";
import type { Review } from "../api/types";
import {
  NO_FILTERS,
  filterFindings,
  hasActiveFilters,
  toggleSeverity,
  uniqueSkills,
  type FindingFilters,
} from "../lib/findings";
import { FindingList } from "./FindingList";
import { EmptyState } from "./PageState";
import { PrSummary } from "./PrSummary";
import { SeverityTiles } from "./SeverityTiles";
import "./ReviewResults.css";

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
                <option value="">All skills</option>
                {skills.map((skill) => (
                  <option key={skill.id} value={skill.id}>
                    {skill.name}
                  </option>
                ))}
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
