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

export type SkillGroup = "language" | "design" | "standards";

export interface SkillOption {
  id: string;
  name: string;
  group: SkillGroup;
  /** Findings from this skill in the list that was passed in. */
  count: number;
}

/** Design skills in display order; the API has no skill category, so new ones are added here. */
const DESIGN_SKILL_ORDER = ["rest", "solid"];

/** The id findings from the user's own standards are stored under. */
const STANDARDS_SKILL_ID = "custom";

const GROUP_ORDER: SkillGroup[] = ["language", "design", "standards"];

function groupOf(skillId: string): SkillGroup {
  if (skillId === STANDARDS_SKILL_ID) return "standards";
  return DESIGN_SKILL_ORDER.includes(skillId) ? "design" : "language";
}

/** Skills that produced findings for a filter menu: language (A-Z), then REST, SOLID, then your standards. */
export function uniqueSkills(findings: readonly Finding[]): SkillOption[] {
  const found = new Map<string, SkillOption>();
  for (const f of findings) {
    const existing = found.get(f.skillId);
    if (existing) {
      existing.count += 1;
    } else {
      found.set(f.skillId, { id: f.skillId, name: f.skillName, group: groupOf(f.skillId), count: 1 });
    }
  }

  const designRank = (id: string) => DESIGN_SKILL_ORDER.indexOf(id);
  return [...found.values()].sort((a, b) => {
    if (a.group !== b.group) return GROUP_ORDER.indexOf(a.group) - GROUP_ORDER.indexOf(b.group);
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
