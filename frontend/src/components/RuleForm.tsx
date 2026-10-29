import { useId, useState, type FormEvent } from "react";
import { RULE_LIMITS, SEVERITIES, type Rule, type RuleInput, type Severity } from "../api/types";
import { SEVERITY_LABEL } from "../lib/severity";
import "./RuleForm.css";

interface RuleFormProps {
  /** The rule being edited; omit to write a new one. */
  rule?: Rule;
  /** Saves the rule; rejecting shows the error's message in the form. */
  onSubmit: (input: RuleInput) => Promise<void>;
  onCancel: () => void;
}

export function RuleForm({ rule, onSubmit, onCancel }: RuleFormProps) {
  const ids = { title: useId(), instructions: useId(), severity: useId(), error: useId() };
  const [title, setTitle] = useState(rule?.title ?? "");
  const [instructions, setInstructions] = useState(rule?.instructions ?? "");
  const [severity, setSeverity] = useState<Severity>(rule?.severity ?? "medium");
  const [enabled, setEnabled] = useState(rule?.enabled ?? true);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (!title.trim() || !instructions.trim()) {
      setError("Give the standard a title and instructions.");
      return;
    }
    setError(null);
    setSaving(true);
    try {
      await onSubmit({ title: title.trim(), instructions: instructions.trim(), severity, enabled });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save the standard.");
      setSaving(false);
    }
  }

  return (
    <form className="rule-form" onSubmit={handleSubmit} noValidate>
      <label htmlFor={ids.title}>Title</label>
      <input
        id={ids.title}
        className="input"
        value={title}
        maxLength={RULE_LIMITS.maxTitle}
        placeholder="Wrap errors with context"
        onChange={(e) => setTitle(e.target.value)}
        disabled={saving}
      />

      <label htmlFor={ids.instructions}>Instructions</label>
      <textarea
        id={ids.instructions}
        className="input rule-form__text"
        rows={5}
        value={instructions}
        maxLength={RULE_LIMITS.maxInstructions}
        placeholder="Return errors as fmt.Errorf(&quot;doing X: %w&quot;, err). Flag any function that returns a bare err."
        onChange={(e) => setInstructions(e.target.value)}
        disabled={saving}
      />
      <p className="rule-form__count">
        {instructions.length} / {RULE_LIMITS.maxInstructions}
      </p>

      <div className="rule-form__row">
        <div>
          <label htmlFor={ids.severity}>Severity</label>
          <select
            id={ids.severity}
            className="input"
            value={severity}
            onChange={(e) => setSeverity(e.target.value as Severity)}
            disabled={saving}
          >
            {SEVERITIES.map((s) => (
              <option key={s} value={s}>
                {SEVERITY_LABEL[s]}
              </option>
            ))}
          </select>
        </div>
        <label className="rule-form__check">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} disabled={saving} />
          Apply to my reviews
        </label>
      </div>

      {error && (
        <p id={ids.error} className="rule-form__error" role="alert">
          {error}
        </p>
      )}

      <div className="rule-form__actions">
        <button type="submit" className="button" disabled={saving}>
          {saving ? "Saving…" : rule ? "Save changes" : "Add standard"}
        </button>
        <button type="button" className="button button--secondary" onClick={onCancel} disabled={saving}>
          Cancel
        </button>
      </div>
    </form>
  );
}
