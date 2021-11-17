/*
 *
 * k6 - a next-generation load testing tool
 * Copyright (C) 2016 Load Impact
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as
 * published by the Free Software Foundation, either version 3 of the
 * License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package stats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"go.k6.io/k6/lib/types"
)

// Threshold is a representation of a single threshold for a single metric
type Threshold struct {
	// Source is the text based source of the threshold
	Source string
	// LastFailed is a marker if the last testing of this threshold failed
	LastFailed bool
	// AbortOnFail marks if a given threshold fails that the whole test should be aborted
	AbortOnFail bool
	// AbortGracePeriod is a the minimum amount of time a test should be running before a failing
	// this threshold will abort the test
	AbortGracePeriod types.NullDuration
	// parsed is the threshold condition parsed from the Source expression
	parsed *thresholdCondition
}

func newThreshold(src string, abortOnFail bool, gracePeriod types.NullDuration) (*Threshold, error) {
	condition, err := parseThresholdCondition(src)
	if err != nil {
		return nil, err
	}

	return &Threshold{
		Source:           src,
		parsed:           condition,
		AbortOnFail:      abortOnFail,
		AbortGracePeriod: gracePeriod,
	}, nil
}

func (t *Threshold) runNoTaint(sinks map[string]float64) (bool, error) {
	// Extract the sink value for the aggregation method used in the threshold
	// expression
	lhs, ok := sinks[t.parsed.AggregationMethod]
	if !ok {
		return false, fmt.Errorf("unable to apply threshold %s over metrics; reason: "+
			"no metric supporting the %s aggregation method found",
			t.parsed.AggregationMethod,
			t.parsed.AggregationMethod)
	}

	// Apply the threshold expression operator to the left and
	// right hand side values
	passes := false
	switch t.parsed.Operator {
	case ">":
		passes = lhs > t.parsed.Value
	case ">=":
		passes = lhs >= t.parsed.Value
	case "<=":
		passes = lhs <= t.parsed.Value
	case "<":
		passes = lhs < t.parsed.Value
	case "==":
		passes = lhs == t.parsed.Value
	case "===":
		// Considering a sink always maps to float64 values,
		// strictly equal is equivalent to loosely equal
		passes = lhs == t.parsed.Value
	case "!=":
		passes = lhs != t.parsed.Value
	default:
		// The ParseThresholdCondition constructor should ensure that no invalid
		// operator gets through, but let's protech our future selves anyhow.
		return false, fmt.Errorf("unable to apply threshold %s over metrics; reason: %s is an invalid operator", t.Source, t.parsed.Operator)
	}

	// Perform the actual threshold verification
	return passes, nil
}

func (t *Threshold) run(sinks map[string]float64) (bool, error) {
	b, err := t.runNoTaint(sinks)
	t.LastFailed = !b
	return b, err
}

type thresholdCondition struct {
	AggregationMethod string  `json:"left-hand"`
	Operator          string  `json:"operator"`
	Value             float64 `json:"right-hand"`
}

// ParseThresholdCondition parses a threshold condition expression,
// as defined in a JS script (for instance p(95)<1000), into a ThresholdCondition
// instance, using our parser combinators package.

// This parser expect a threshold expression matching the following BNF
//
// ```
// assertion           -> aggregation_method whitespace* operator whitespace* float
// aggregation_method  -> trend | rate | gauge | counter
// counter             -> "count" | "sum" | "rate"
// gauge               -> "last" | "min" | "max" | "value"
// rate                -> "rate"
// trend               -> "min" | "mean" | "avg" | "max" | percentile
// percentile          -> "p(" float ")"
// operator            -> ">" | ">=" | "<=" | "<" | "==" | "===" | "!="
// float               -> digit+ (. digit+)?
// digit               -> "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9"
// whitespace          -> space | tab
// tab                 -> "\t"
// space               -> " "
// ```
func parseThresholdCondition(expression string) (*thresholdCondition, error) {
	parser := ParseAssertion()

	// Parse the Threshold as provided in the JS script options thresholds value (p(95)<1000)
	result := parser([]rune(expression))
	if result.Err != nil {
		return nil, fmt.Errorf("parsing threshold condition %s failed; "+
			"reason: the parser failed on %s",
			expression,
			result.Err.ErrorAtChar([]rune(expression)))
	}

	// The Sequence combinator will return a slice of interface{}
	// instances. Up to us to decide what we want to cast them down
	// to.
	// Considering our expression format, the parser should return a slice
	// of size 3 to us: aggregation_method operator sink_value. The type system
	// ensures us it should be the case too, but let's protect our future selves anyhow.
	parsed := result.Payload.([]interface{})
	if len(parsed) != 3 {
		return nil, fmt.Errorf("parsing threshold condition %s failed; reason: malformed expression", expression)
	}

	// Unpack the various components of the parsed threshold expression
	method := parsed[0].(string)
	operator := parsed[1].(string)
	value := parsed[2].(float64)

	return &thresholdCondition{AggregationMethod: method, Operator: operator, Value: value}, nil
}

type thresholdConfig struct {
	Threshold        string             `json:"threshold"`
	AbortOnFail      bool               `json:"abortOnFail"`
	AbortGracePeriod types.NullDuration `json:"delayAbortEval"`
}

//used internally for JSON marshalling
type rawThresholdConfig thresholdConfig

func (tc *thresholdConfig) UnmarshalJSON(data []byte) error {
	//shortcircuit unmarshalling for simple string format
	if err := json.Unmarshal(data, &tc.Threshold); err == nil {
		return nil
	}

	rawConfig := (*rawThresholdConfig)(tc)
	return json.Unmarshal(data, rawConfig)
}

func (tc thresholdConfig) MarshalJSON() ([]byte, error) {
	var data interface{} = tc.Threshold
	if tc.AbortOnFail {
		data = rawThresholdConfig(tc)
	}

	return MarshalJSONWithoutHTMLEscape(data)
}

// Thresholds is the combination of all Thresholds for a given metric
type Thresholds struct {
	Thresholds []*Threshold
	Abort      bool
	Sinked     map[string]float64
}

// NewThresholds returns Thresholds objects representing the provided source strings
func NewThresholds(sources []string) (Thresholds, error) {
	tcs := make([]thresholdConfig, len(sources))
	for i, source := range sources {
		tcs[i].Threshold = source
	}

	return newThresholdsWithConfig(tcs)
}

func newThresholdsWithConfig(configs []thresholdConfig) (Thresholds, error) {
	ts := make([]*Threshold, len(configs))
	for i, config := range configs {
		t, err := newThreshold(config.Threshold, config.AbortOnFail, config.AbortGracePeriod)
		if err != nil {
			return Thresholds{}, fmt.Errorf("threshold %d error: %w", i, err)
		}
		ts[i] = t
	}

	return Thresholds{ts, false, make(map[string]float64)}, nil
}

func (ts *Thresholds) runAll(t time.Duration) (bool, error) {
	succeeded := true
	for i, th := range ts.Thresholds {
		b, err := th.run(ts.Sinked)
		if err != nil {
			return false, fmt.Errorf("threshold %d run error: %w", i, err)
		}

		if !b {
			succeeded = false

			if ts.Abort || !th.AbortOnFail {
				continue
			}

			ts.Abort = !th.AbortGracePeriod.Valid ||
				th.AbortGracePeriod.Duration < types.Duration(t)
		}
	}

	return succeeded, nil
}

// Run processes all the thresholds with the provided Sink at the provided time and returns if any
// of them fails
func (ts *Thresholds) Run(sink Sink, t time.Duration) (bool, error) {
	// Update the sinks store
	f := sink.Format(t)
	for k, v := range f {
		ts.Sinked[k] = v
	}

	return ts.runAll(t)
}

// UnmarshalJSON is implementation of json.Unmarshaler
func (ts *Thresholds) UnmarshalJSON(data []byte) error {
	var configs []thresholdConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return err
	}
	newts, err := newThresholdsWithConfig(configs)
	if err != nil {
		return err
	}
	*ts = newts
	return nil
}

// MarshalJSON is implementation of json.Marshaler
func (ts Thresholds) MarshalJSON() ([]byte, error) {
	configs := make([]thresholdConfig, len(ts.Thresholds))
	for i, t := range ts.Thresholds {
		configs[i].Threshold = t.Source
		configs[i].AbortOnFail = t.AbortOnFail
		configs[i].AbortGracePeriod = t.AbortGracePeriod
	}

	return MarshalJSONWithoutHTMLEscape(configs)
}

// MarshalJSONWithoutHTMLEscape marshals t to JSON without escaping characters
// for safe use in HTML.
func MarshalJSONWithoutHTMLEscape(t interface{}) ([]byte, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(t)
	bytes := buffer.Bytes()
	if err == nil && len(bytes) > 0 {
		// Remove the newline appended by Encode() :-/
		// See https://github.com/golang/go/issues/37083
		bytes = bytes[:len(bytes)-1]
	}
	return bytes, err
}

var _ json.Unmarshaler = &Thresholds{}
var _ json.Marshaler = &Thresholds{}
