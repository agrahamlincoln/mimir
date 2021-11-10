package rules

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	yaml "gopkg.in/yaml.v2"

	"github.com/grafana/mimir/pkg/ruler/rulefmt"
)

// A RecordingRule records its vector expression into new timeseries.
type RecordingRule struct {
	name   string
	vector parser.Expr
	labels labels.Labels
	// Protects the below.
	mtx sync.Mutex
	// The health of the recording rule.
	health RuleHealth
	// Timestamp of last evaluation of the recording rule.
	evaluationTimestamp time.Time
	// The last error seen by the recording rule.
	lastError error
	// Duration of how long it took to evaluate the recording rule.
	evaluationDuration time.Duration
	// Used by cortex federated ruler to determine src and dest for rules
	srcTenants string
	destTenant string
}

// NewRecordingRule returns a new recording rule.
func NewRecordingRule(name string, vector parser.Expr, lset labels.Labels, srcTenants, destTenant string) *RecordingRule {
	return &RecordingRule{
		name:       name,
		vector:     vector,
		health:     HealthUnknown,
		labels:     lset,
		srcTenants: srcTenants,
		destTenant: destTenant,
	}
}

// Name returns the rule name.
func (rule *RecordingRule) Name() string {
	return rule.name
}

// Query returns the rule query expression.
func (rule *RecordingRule) Query() parser.Expr {
	return rule.vector
}

// Labels returns the rule labels.
func (rule *RecordingRule) Labels() labels.Labels {
	return rule.labels
}

// Eval evaluates the rule and then overrides the metric names and labels accordingly.
func (rule *RecordingRule) Eval(ctx context.Context, ts time.Time, query QueryFunc, _ *url.URL) (promql.Vector, error) {
	// Set the context based on the src tenants for the query func
	vector, err := query(ctx, rule.vector.String(), ts)
	if err != nil {
		return nil, err
	}
	// Override the metric name and labels.
	for i := range vector {
		sample := &vector[i]

		lb := labels.NewBuilder(sample.Metric)

		lb.Set(labels.MetricName, rule.name)

		for _, l := range rule.labels {
			lb.Set(l.Name, l.Value)
		}

		sample.Metric = lb.Labels()
	}

	// Check that the rule does not produce identical metrics after applying
	// labels.
	if vector.ContainsSameLabelset() {
		err = fmt.Errorf("vector contains metrics with the same labelset after applying rule labels")
		rule.SetHealth(HealthBad)
		rule.SetLastError(err)
		return nil, err
	}

	rule.SetHealth(HealthGood)
	rule.SetLastError(err)
	return vector, nil
}

func (rule *RecordingRule) String() string {
	r := rulefmt.Rule{
		Record:     rule.name,
		Expr:       rule.vector.String(),
		Labels:     rule.labels.Map(),
		SrcTenants: rule.srcTenants,
		DestTenant: rule.destTenant,
	}

	byt, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Sprintf("error marshaling recording rule: %q", err.Error())
	}

	return string(byt)
}

// SetEvaluationDuration updates evaluationDuration to the time in seconds it took to evaluate the rule on its last evaluation.
func (rule *RecordingRule) SetEvaluationDuration(dur time.Duration) {
	rule.mtx.Lock()
	defer rule.mtx.Unlock()
	rule.evaluationDuration = dur
}

// SetLastError sets the current error seen by the recording rule.
func (rule *RecordingRule) SetLastError(err error) {
	rule.mtx.Lock()
	defer rule.mtx.Unlock()
	rule.lastError = err
}

// LastError returns the last error seen by the recording rule.
func (rule *RecordingRule) LastError() error {
	rule.mtx.Lock()
	defer rule.mtx.Unlock()
	return rule.lastError
}

// SetHealth sets the current health of the recording rule.
func (rule *RecordingRule) SetHealth(health RuleHealth) {
	rule.mtx.Lock()
	defer rule.mtx.Unlock()
	rule.health = health
}

// Health returns the current health of the recording rule.
func (rule *RecordingRule) Health() RuleHealth {
	rule.mtx.Lock()
	defer rule.mtx.Unlock()
	return rule.health
}

// GetEvaluationDuration returns the time in seconds it took to evaluate the recording rule.
func (rule *RecordingRule) GetEvaluationDuration() time.Duration {
	rule.mtx.Lock()
	defer rule.mtx.Unlock()
	return rule.evaluationDuration
}

// SetEvaluationTimestamp updates evaluationTimestamp to the timestamp of when the rule was last evaluated.
func (rule *RecordingRule) SetEvaluationTimestamp(ts time.Time) {
	rule.mtx.Lock()
	defer rule.mtx.Unlock()
	rule.evaluationTimestamp = ts
}

// GetEvaluationTimestamp returns the time the evaluation took place.
func (rule *RecordingRule) GetEvaluationTimestamp() time.Time {
	rule.mtx.Lock()
	defer rule.mtx.Unlock()
	return rule.evaluationTimestamp
}

func (rule *RecordingRule) GetDestTenant() string {
	return rule.destTenant
}

func (rule *RecordingRule) GetSrcTenants() string {
	return rule.srcTenants
}
