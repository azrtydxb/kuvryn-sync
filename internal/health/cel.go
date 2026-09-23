package health

import (
	"fmt"
	"sort"

	"github.com/google/cel-go/cel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

// ruleCostLimit bounds the work a single health rule may do per object; a
// rule that exceeds it fails closed instead of stalling reconciliation.
const ruleCostLimit = 100000

var celEnv = mustCELEnv()

func mustCELEnv() *cel.Env {
	env, err := cel.NewEnv(cel.Variable("object", cel.DynType))
	if err != nil {
		panic(err)
	}
	return env
}

// CompileRule checks that a HealthCheck expression is valid CEL returning a bool.
func CompileRule(expression string) (cel.Program, error) {
	ast, issues := celEnv.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	if ast.OutputType() != cel.BoolType && ast.OutputType() != cel.DynType {
		return nil, fmt.Errorf("expression must evaluate to a bool, not %s", ast.OutputType())
	}
	return celEnv.Program(ast, cel.CostLimit(ruleCostLimit))
}

type compiledRule struct {
	program cel.Program
	rule    corev1alpha1.HealthRule
	source  string
}

// Evaluator judges health with HealthCheck rules first and built-in rules
// and kstatus conventions otherwise.
type Evaluator struct {
	rules map[schema.GroupKind][]compiledRule
}

// NewEvaluator compiles HealthChecks, applied in name order per kind. It
// returns an error naming every rule that does not compile, alongside an
// Evaluator that skips those rules.
func NewEvaluator(checks []corev1alpha1.HealthCheck) (Evaluator, error) {
	sorted := append([]corev1alpha1.HealthCheck(nil), checks...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	evaluator := Evaluator{rules: map[schema.GroupKind][]compiledRule{}}
	var invalid []string
	for _, check := range sorted {
		gk := schema.GroupKind{Group: check.Spec.Group, Kind: check.Spec.Kind}
		for i, rule := range check.Spec.Rules {
			program, err := CompileRule(rule.Expression)
			source := fmt.Sprintf("HealthCheck %s rule %d", check.Name, i)
			if err != nil {
				invalid = append(invalid, fmt.Sprintf("%s: %v", source, err))
				continue
			}
			evaluator.rules[gk] = append(evaluator.rules[gk], compiledRule{program: program, rule: rule, source: source})
		}
	}
	if len(invalid) > 0 {
		return evaluator, fmt.Errorf("invalid health rules: %v", invalid)
	}
	return evaluator, nil
}

// Evaluate returns obj's health.
func (e Evaluator) Evaluate(obj unstructured.Unstructured) (Result, error) {
	rules := e.rules[obj.GroupVersionKind().GroupKind()]
	if len(rules) == 0 {
		return Evaluate(obj)
	}
	fallback, err := Evaluate(obj)
	if err != nil {
		return Result{}, err
	}
	result := Result{Resource: fallback.Resource}
	for _, rule := range rules {
		out, _, err := rule.program.Eval(map[string]any{"object": obj.Object})
		if err != nil {
			// A failing rule holds the rollout rather than passing it.
			return progressing(result, "HealthCheckFailed", fmt.Sprintf("%s: %v", rule.source, err)), nil
		}
		matched, ok := out.Value().(bool)
		if !ok {
			return progressing(result, "HealthCheckFailed", fmt.Sprintf("%s did not return a bool", rule.source)), nil
		}
		if matched {
			result.State = rule.rule.State
			result.Reason = "HealthCheck"
			result.Message = rule.rule.Message
			return result, nil
		}
	}
	return fallback, nil
}
