import { useState } from "react";
import { useApi } from "../api/ApiProvider";
import { ApiError } from "../api/client";
import { RULE_LIMITS, type Rule, type RuleInput } from "../api/types";
import { ErrorState, EmptyState, LoadingState } from "../components/PageState";
import { RuleForm } from "../components/RuleForm";
import { SeverityBadge } from "../components/SeverityBadge";
import { useAsync } from "../hooks/useAsync";
import "./StandardsPage.css";

/** "new" while writing a rule, a rule id while editing one, null otherwise. */
type Editing = "new" | string | null;

export function StandardsPage() {
  const api = useApi();
  const rules = useAsync(() => api.listRules(), [api]);
  const [editing, setEditing] = useState<Editing>(null);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  async function run(action: () => Promise<unknown>) {
    setActionError(null);
    try {
      await action();
      rules.reload();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "That did not work.");
    }
  }

  async function save(input: RuleInput, existing?: Rule) {
    if (existing) await api.updateRule(existing.id, input);
    else await api.createRule(input);
    setEditing(null);
    rules.reload();
  }

  if (rules.error instanceof ApiError && rules.error.status === 401) {
    return (
      <section className="standards">
        <h1>Your standards</h1>
        <EmptyState title="Sign in to write your own standards">
          Standards are rules in plain English that PullWise checks your pull requests against. They are private to your account.
        </EmptyState>
      </section>
    );
  }

  const list = rules.data ?? [];
  const atLimit = list.length >= RULE_LIMITS.maxRules;

  return (
    <section className="standards">
      <h1>Your standards</h1>
      <p className="standards__lead">
        Write rules in plain English, like "never call the database from an HTTP handler". On your next review the AI
        checks your code against them, and findings appear under "Your standards". Be specific about what to look for:
        vague rules give vague results.
      </p>

      {rules.error ? (
        <ErrorState error={rules.error} onRetry={rules.reload} />
      ) : !rules.data ? (
        <LoadingState label="Loading your standards" />
      ) : (
        <>
          <div className="standards__bar">
            <span>
              {list.length} of {RULE_LIMITS.maxRules} standards
            </span>
            {editing === null && (
              <button type="button" className="button" onClick={() => setEditing("new")} disabled={atLimit}>
                Add a standard
              </button>
            )}
          </div>

          {actionError && (
            <p className="standards__error" role="alert">
              {actionError}
            </p>
          )}

          {editing === "new" && <RuleForm onSubmit={(input) => save(input)} onCancel={() => setEditing(null)} />}

          {list.length === 0 && editing !== "new" ? (
            <EmptyState title="No standards yet">Add one, and your next review will check it.</EmptyState>
          ) : (
            <ul className="standards__list">
              {list.map((rule) =>
                editing === rule.id ? (
                  <li key={rule.id}>
                    <RuleForm rule={rule} onSubmit={(input) => save(input, rule)} onCancel={() => setEditing(null)} />
                  </li>
                ) : (
                  <li key={rule.id} className={`rule-card${rule.enabled ? "" : " rule-card--off"}`}>
                    <div className="rule-card__head">
                      <strong>{rule.title}</strong>
                      <SeverityBadge severity={rule.severity} />
                      {!rule.enabled && <span className="rule-card__off">Off</span>}
                    </div>
                    <p className="rule-card__text">{rule.instructions}</p>
                    <div className="rule-card__actions">
                      <button
                        type="button"
                        className="button button--ghost"
                        onClick={() => run(() => api.updateRule(rule.id, { ...rule, enabled: !rule.enabled }))}
                      >
                        {rule.enabled ? "Turn off" : "Turn on"}
                      </button>
                      <button type="button" className="button button--ghost" onClick={() => setEditing(rule.id)}>
                        Edit
                      </button>
                      {confirmDelete === rule.id ? (
                        <>
                          <button
                            type="button"
                            className="button button--secondary"
                            onClick={() => run(() => api.deleteRule(rule.id)).then(() => setConfirmDelete(null))}
                          >
                            Confirm delete
                          </button>
                          <button type="button" className="button button--ghost" onClick={() => setConfirmDelete(null)}>
                            Keep
                          </button>
                        </>
                      ) : (
                        <button type="button" className="button button--ghost" onClick={() => setConfirmDelete(rule.id)}>
                          Delete
                        </button>
                      )}
                    </div>
                  </li>
                ),
              )}
            </ul>
          )}
        </>
      )}
    </section>
  );
}
