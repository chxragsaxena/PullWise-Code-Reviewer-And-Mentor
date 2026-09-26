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

export function uniqueSkills(findings: readonly Finding[]): { id: string; name: string }[] {
  const seen = new Map<string, string>();
  for (const f of findings) if (!seen.has(f.skillId)) seen.set(f.skillId, f.skillName);
  return [...seen].map(([id, name]) => ({ id, name })).sort((a, b) => a.name.localeCompare(b.name));
}

export function toggleSeverity(filters: FindingFilters, severity: Severity): FindingFilters {
  const severities = filters.severities.includes(severity)
    ? filters.severities.filter((s) => s !== severity)
    : [...filters.severities, severity];
  return { ...filters, severities };
}
