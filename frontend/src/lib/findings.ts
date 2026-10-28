import { SEVERITIES, type Finding, type Severity, type SeverityCounts } from "../api/types";

export interface FindingFilters {
  /** Empty means "all severities". */
  severities: readonly Severity[];
  /** null means "all skills". */
  skillId: string | null;
}

export const NO_FILTERS: FindingFilters = { severities: [], skillId: null };

export function hasActiveFilters(filters: FindingFilters): boolean {
  return filters.severities.length > 0 || filters.skillId !== null;
}

export function filterFindings(findings: readonly Finding[], filters: FindingFilters): Finding[] {
  return findings.filter(
    (f) =>
      (filters.severities.length === 0 || filters.severities.includes(f.severity)) &&
      (filters.skillId === null || f.skillId === filters.skillId),
  );
}

export function countBySeverity(findings: readonly Finding[]): SeverityCounts {
  const counts: SeverityCounts = { critical: 0, high: 0, medium: 0, suggestion: 0 };
  for (const f of findings) counts[f.severity] += 1;
  return counts;
}

export interface SeverityGroup {
  severity: Severity;
  findings: Finding[];
}

/** Groups findings by severity, most serious first, skipping empty groups. */
export function groupBySeverity(findings: readonly Finding[]): SeverityGroup[] {
  return SEVERITIES.map((severity) => ({
    severity,
    findings: findings.filter((f) => f.severity === severity),
  })).filter((group) => group.findings.length > 0);
}

export type SkillGroup = "language" | "design";

export interface SkillOption {
  id: string;
  name: string;
  group: SkillGroup;
  /** Findings from this skill in the list that was passed in. */
  count: number;
}

/**
 * Skills that judge design rather than one language's code, in the order they
 * are shown. Any other skill (Go, TypeScript, ...) is a language skill and is
 * listed first. A skill's category isn't in the API, so a new design skill
 * must be added here to be grouped with these.
 */
const DESIGN_SKILL_ORDER = ["rest", "solid"];

/**
 * The skills that produced findings, for a filter menu: language skills first
 * (alphabetical), then REST, then SOLID.
 */
export function uniqueSkills(findings: readonly Finding[]): SkillOption[] {
  const found = new Map<string, SkillOption>();
  for (const f of findings) {
    const existing = found.get(f.skillId);
    if (existing) {
      existing.count += 1;
    } else {
      const group: SkillGroup = DESIGN_SKILL_ORDER.includes(f.skillId) ? "design" : "language";
      found.set(f.skillId, { id: f.skillId, name: f.skillName, group, count: 1 });
    }
  }

  const designRank = (id: string) => DESIGN_SKILL_ORDER.indexOf(id);
  return [...found.values()].sort((a, b) => {
    if (a.group !== b.group) return a.group === "language" ? -1 : 1;
    if (a.group === "design") return designRank(a.id) - designRank(b.id);
    return a.name.localeCompare(b.name);
  });
}

export function toggleSeverity(filters: FindingFilters, severity: Severity): FindingFilters {
  const severities = filters.severities.includes(severity)
    ? filters.severities.filter((s) => s !== severity)
    : [...filters.severities, severity];
  return { ...filters, severities };
}
