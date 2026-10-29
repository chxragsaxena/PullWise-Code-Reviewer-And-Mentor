package review

import "errors"

// Rule is a standard a user wrote for the AI to check their pull requests against.
type Rule struct {
	ID           string
	Title        string
	Instructions string
	Severity     Severity
	Enabled      bool
}

var (
	// ErrRuleNotFound means the rule does not exist or belongs to someone else.
	ErrRuleNotFound = errors.New("rule not found")
	// ErrRuleLimit means the user already has the maximum number of rules.
	ErrRuleLimit = errors.New("rule limit reached")
)
