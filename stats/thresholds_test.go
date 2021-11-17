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
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.k6.io/k6/lib/types"
)

func TestNewThreshold(t *testing.T) {
	// Arrange
	src := `rate<0.01`
	abortOnFail := false
	gracePeriod := types.NullDurationFrom(2 * time.Second)

	// Act
	th, err := newThreshold(src, abortOnFail, gracePeriod)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, src, th.Source)
	assert.False(t, th.LastFailed)
	assert.Equal(t, abortOnFail, th.AbortOnFail)
	assert.Equal(t, gracePeriod, th.AbortGracePeriod)
}

func TestNewThreshold_InvalidThresholdConditionExpression(t *testing.T) {
	// Arrange
	src := "1+1==2"
	abortOnFail := false
	gracePeriod := types.NullDurationFrom(2 * time.Second)

	// Act
	th, err := newThreshold(src, abortOnFail, gracePeriod)

	// Assert
	assert.Error(t, err, "instantiating a threshold with an invalid expression should fail")
	assert.Nil(t, th, "instantiating a threshold with an invalid expression should return a nil Threshold")
}

func TestThreshold_runNoTaint(t *testing.T) {
	type fields struct {
		Source           string
		LastFailed       bool
		AbortOnFail      bool
		AbortGracePeriod types.NullDuration
		parsed           *thresholdCondition
	}
	type args struct {
		sinks map[string]float64
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		{
			"valid expression over passing threshold",
			fields{"rate<0.01", false, false, types.NullDurationFrom(2 * time.Second), &thresholdCondition{"rate", "<", 0.01}},
			args{map[string]float64{"rate": 0.00001}},
			true,
			false,
		},
		{
			"valid expression over failing threshold",
			fields{"rate>0.01", false, false, types.NullDurationFrom(2 * time.Second), &thresholdCondition{"rate", ">", 0.01}},
			args{map[string]float64{"rate": 0.00001}},
			false,
			false,
		},
		{
			"valid expression over non-existing sink",
			fields{"rate>0.01", false, false, types.NullDurationFrom(2 * time.Second), &thresholdCondition{"rate", ">", 0.01}},
			args{map[string]float64{"med": 27.2}},
			false,
			true,
		},
		{
			// The ParseThresholdCondition constructor should ensure that no invalid
			// operator gets through, but let's protech our future selves anyhow.
			"invalid expression operator",
			fields{"rate&0.01", false, false, types.NullDurationFrom(2 * time.Second), &thresholdCondition{"rate", "&", 0.01}},
			args{map[string]float64{"rate": 0.00001}},
			false,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &Threshold{
				Source:           tt.fields.Source,
				LastFailed:       tt.fields.LastFailed,
				AbortOnFail:      tt.fields.AbortOnFail,
				AbortGracePeriod: tt.fields.AbortGracePeriod,
				parsed:           tt.fields.parsed,
			}
			got, err := tr.runNoTaint(tt.args.sinks)
			if (err != nil) != tt.wantErr {
				t.Errorf("Threshold.runNoTaint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Threshold.runNoTaint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestThresholdRun(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		sinks := map[string]float64{"rate": 0.0001}
		th, err := newThreshold(`rate<0.01`, false, types.NullDuration{})
		assert.NoError(t, err)

		t.Run("no taint", func(t *testing.T) {
			b, err := th.runNoTaint(sinks)
			assert.NoError(t, err)
			assert.True(t, b)
			assert.False(t, th.LastFailed)
		})

		t.Run("taint", func(t *testing.T) {
			b, err := th.run(sinks)
			assert.NoError(t, err)
			assert.True(t, b)
			assert.False(t, th.LastFailed)
		})
	})

	t.Run("false", func(t *testing.T) {
		sinks := map[string]float64{"rate": 1}
		th, err := newThreshold(`rate<0.01`, false, types.NullDuration{})
		assert.NoError(t, err)

		t.Run("no taint", func(t *testing.T) {
			b, err := th.runNoTaint(sinks)
			assert.NoError(t, err)
			assert.False(t, b)
			assert.False(t, th.LastFailed)
		})

		t.Run("taint", func(t *testing.T) {
			b, err := th.run(sinks)
			assert.NoError(t, err)
			assert.False(t, b)
			assert.True(t, th.LastFailed)
		})
	})
}

func TestParseThresholdCondition(t *testing.T) {
	type args struct {
		expression string
	}
	tests := []struct {
		name    string
		args    args
		want    *thresholdCondition
		wantErr bool
	}{
		{"valid Counter count expression with Integer value", args{"count<100"}, &thresholdCondition{"count", "<", 100}, false},
		{"valid Counter count expression with Real value", args{"count<100.10"}, &thresholdCondition{"count", "<", 100.10}, false},
		{"valid Counter rate expression with Integer value", args{"rate<100"}, &thresholdCondition{"rate", "<", 100}, false},
		{"valid Counter rate expression with Real value", args{"rate<100.10"}, &thresholdCondition{"rate", "<", 100.10}, false},
		{"valid Gauge value expression with Integer value", args{"value<100"}, &thresholdCondition{"value", "<", 100}, false},
		{"valid Gauge value expression with Real value", args{"value<100.10"}, &thresholdCondition{"value", "<", 100.10}, false},
		{"valid Rate rate expression with Integer value", args{"rate<100"}, &thresholdCondition{"rate", "<", 100}, false},
		{"valid Rate rate expression with Real value", args{"rate<100.10"}, &thresholdCondition{"rate", "<", 100.10}, false},
		{"valid Trend avg expression with Integer value", args{"avg<100"}, &thresholdCondition{"avg", "<", 100}, false},
		{"valid Trend avg expression with Real value", args{"avg<100.10"}, &thresholdCondition{"avg", "<", 100.10}, false},
		{"valid Trend min expression with Integer value", args{"avg<100"}, &thresholdCondition{"avg", "<", 100}, false},
		{"valid Trend min expression with Real value", args{"min<100.10"}, &thresholdCondition{"min", "<", 100.10}, false},
		{"valid Trend max expression with Integer value", args{"max<100"}, &thresholdCondition{"max", "<", 100}, false},
		{"valid Trend max expression with Real value", args{"max<100.10"}, &thresholdCondition{"max", "<", 100.10}, false},
		{"valid Trend med expression with Integer value", args{"med<100"}, &thresholdCondition{"med", "<", 100}, false},
		{"valid Trend med expression with Real value", args{"med<100.10"}, &thresholdCondition{"med", "<", 100.10}, false},
		{"valid Trend percentile expression with Integer N and Integer value", args{"p(99)<100"}, &thresholdCondition{"p(99)", "<", 100}, false},
		{"valid Trend percentile expression with Integer N and Real value", args{"p(99)<100.10"}, &thresholdCondition{"p(99)", "<", 100.10}, false},
		{"valid Trend percentile expression with Real N and Integer value", args{"p(99.9)<100"}, &thresholdCondition{"p(99.9)", "<", 100}, false},
		{"valid Trend percentile expression with Real N and Real value", args{"p(99.9)<100.10"}, &thresholdCondition{"p(99.9)", "<", 100.10}, false},
		{"valid Trend percentile expression with Real N and Real value", args{"p(99.9)<100.10"}, &thresholdCondition{"p(99.9)", "<", 100.10}, false},
		{"valid > operator", args{"med>100"}, &thresholdCondition{"med", ">", 100}, false},
		{"valid > operator", args{"med>=100"}, &thresholdCondition{"med", ">=", 100}, false},
		{"valid > operator", args{"med<100"}, &thresholdCondition{"med", "<", 100}, false},
		{"valid > operator", args{"med<=100"}, &thresholdCondition{"med", "<=", 100}, false},
		{"valid > operator", args{"med==100"}, &thresholdCondition{"med", "==", 100}, false},
		{"valid > operator", args{"med===100"}, &thresholdCondition{"med", "===", 100}, false},
		{"valid > operator", args{"med!=100"}, &thresholdCondition{"med", "!=", 100}, false},
		{"threshold expressions whitespaces are ignored", args{"count    \t<\t\t\t   200    "}, &thresholdCondition{"count", "<", 200}, false},
		{"threshold expressions newlines are ignored", args{"count<200\n"}, &thresholdCondition{"count", "<", 200}, false},
		{"non-existing aggregation method", args{"foo<100"}, nil, true},
		{"malformed aggregation method", args{"mad<100"}, nil, true},
		{"non-existing operator", args{"med&100"}, nil, true},
		{"malformed operator", args{"med&=100"}, nil, true},
		{"no value", args{"med<"}, nil, true},
		{"invalid type value (boolean)", args{"med<false"}, nil, true},
		{"invalid value operation(+type)", args{"med<rate"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseThresholdCondition(tt.args.expression)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseThresholdCondition() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseThresholdCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewThresholds(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ts, err := NewThresholds([]string{})
		assert.NoError(t, err)
		assert.Len(t, ts.Thresholds, 0)
	})
	t.Run("two", func(t *testing.T) {
		sources := []string{`rate<0.01`, `p(95)<200`}
		ts, err := NewThresholds(sources)
		assert.NoError(t, err)
		assert.Len(t, ts.Thresholds, 2)
		for i, th := range ts.Thresholds {
			assert.Equal(t, sources[i], th.Source)
			assert.False(t, th.LastFailed)
			assert.False(t, th.AbortOnFail)
		}
	})
}

func TestNewThresholdsWithConfig(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ts, err := NewThresholds([]string{})
		assert.NoError(t, err)
		assert.Len(t, ts.Thresholds, 0)
	})
	t.Run("two", func(t *testing.T) {
		configs := []thresholdConfig{
			{`rate<0.01`, false, types.NullDuration{}},
			{`p(95)<200`, true, types.NullDuration{}},
		}
		ts, err := newThresholdsWithConfig(configs)
		assert.NoError(t, err)
		assert.Len(t, ts.Thresholds, 2)
		for i, th := range ts.Thresholds {
			assert.Equal(t, configs[i].Threshold, th.Source)
			assert.False(t, th.LastFailed)
			assert.Equal(t, configs[i].AbortOnFail, th.AbortOnFail)
		}
	})
}

func TestThresholdsRunAll(t *testing.T) {
	zero := types.NullDuration{}
	oneSec := types.NullDurationFrom(time.Second)
	twoSec := types.NullDurationFrom(2 * time.Second)
	testdata := map[string]struct {
		succ  bool
		err   bool
		abort bool
		grace types.NullDuration
		srcs  []string
	}{
		"one passing":                {true, false, false, zero, []string{`rate<0.01`}},
		"one failing":                {false, false, false, zero, []string{`p(95)<200`}},
		"two passing":                {true, false, false, zero, []string{`rate<0.1`, `rate<0.01`}},
		"two failing":                {false, false, false, zero, []string{`p(95)<200`, `rate<0.1`}},
		"two mixed":                  {false, false, false, zero, []string{`rate<0.01`, `p(95)<200`}},
		"one aborting":               {false, false, true, zero, []string{`p(95)<200`}},
		"abort with grace period":    {false, false, true, oneSec, []string{`p(95)<200`}},
		"no abort with grace period": {false, false, true, twoSec, []string{`p(95)<200`}},
	}

	for name, data := range testdata {
		t.Run(name, func(t *testing.T) {
			ts, err := NewThresholds(data.srcs)
			ts.Sinked = map[string]float64{"rate": 0.0001, "p(95)": 500}
			ts.Thresholds[0].AbortOnFail = data.abort
			ts.Thresholds[0].AbortGracePeriod = data.grace

			runDuration := 1500 * time.Millisecond

			assert.NoError(t, err)

			b, err := ts.runAll(runDuration)

			if data.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if data.succ {
				assert.True(t, b)
			} else {
				assert.False(t, b)
			}

			if data.abort && data.grace.Duration < types.Duration(runDuration) {
				assert.True(t, ts.Abort)
			} else {
				assert.False(t, ts.Abort)
			}
		})
	}
}

func TestThresholdsRun(t *testing.T) {
	ts, err := NewThresholds([]string{"p(95)<2000"})
	assert.NoError(t, err)

	t.Run("error", func(t *testing.T) {
		b, err := ts.Run(DummySink{}, 0)
		assert.Error(t, err)
		assert.False(t, b)
	})

	t.Run("pass", func(t *testing.T) {
		b, err := ts.Run(DummySink{"p(95)": 1234.5}, 0)
		assert.NoError(t, err)
		assert.True(t, b)
	})

	t.Run("fail", func(t *testing.T) {
		b, err := ts.Run(DummySink{"p(95)": 4000}, 0)
		assert.NoError(t, err)
		assert.False(t, b)
	})
}

func TestThresholdsJSON(t *testing.T) {
	var testdata = []struct {
		JSON        string
		srcs        []string
		abortOnFail bool
		gracePeriod types.NullDuration
		outputJSON  string
	}{
		{
			`[]`,
			[]string{},
			false,
			types.NullDuration{},
			"",
		},
		{
			`["rate<0.01"]`,
			[]string{"rate<0.01"},
			false,
			types.NullDuration{},
			"",
		},
		{
			`["rate<0.01"]`,
			[]string{"rate<0.01"},
			false,
			types.NullDuration{},
			`["rate<0.01"]`,
		},
		{
			`["rate<0.01","p(95)<200"]`,
			[]string{"rate<0.01", "p(95)<200"},
			false,
			types.NullDuration{},
			"",
		},
		{
			`[{"threshold":"rate<0.01"}]`,
			[]string{"rate<0.01"},
			false,
			types.NullDuration{},
			`["rate<0.01"]`,
		},
		{
			`[{"threshold":"rate<0.01","abortOnFail":true,"delayAbortEval":null}]`,
			[]string{"rate<0.01"},
			true,
			types.NullDuration{},
			"",
		},
		{
			`[{"threshold":"rate<0.01","abortOnFail":true,"delayAbortEval":"2s"}]`,
			[]string{"rate<0.01"},
			true,
			types.NullDurationFrom(2 * time.Second),
			"",
		},
		{
			`[{"threshold":"rate<0.01","abortOnFail":false}]`,
			[]string{"rate<0.01"},
			false,
			types.NullDuration{},
			`["rate<0.01"]`,
		},
		{
			`[{"threshold":"rate<0.01"}, "p(95)<200"]`,
			[]string{"rate<0.01", "p(95)<200"},
			false,
			types.NullDuration{},
			`["rate<0.01","p(95)<200"]`,
		},
	}

	for _, data := range testdata {
		t.Run(data.JSON, func(t *testing.T) {
			var ts Thresholds
			assert.NoError(t, json.Unmarshal([]byte(data.JSON), &ts))
			assert.Equal(t, len(data.srcs), len(ts.Thresholds))
			for i, src := range data.srcs {
				assert.Equal(t, src, ts.Thresholds[i].Source)
				assert.Equal(t, data.abortOnFail, ts.Thresholds[i].AbortOnFail)
				assert.Equal(t, data.gracePeriod, ts.Thresholds[i].AbortGracePeriod)
			}

			t.Run("marshal", func(t *testing.T) {
				data2, err := MarshalJSONWithoutHTMLEscape(ts)
				assert.NoError(t, err)
				output := data.JSON
				if data.outputJSON != "" {
					output = data.outputJSON
				}
				assert.Equal(t, output, string(data2))
			})
		})
	}

	t.Run("bad JSON", func(t *testing.T) {
		var ts Thresholds
		assert.Error(t, json.Unmarshal([]byte("42"), &ts))
		assert.Nil(t, ts.Thresholds)
		assert.False(t, ts.Abort)
	})

	t.Run("bad source", func(t *testing.T) {
		var ts Thresholds
		assert.Error(t, json.Unmarshal([]byte(`["="]`), &ts))
		assert.Nil(t, ts.Thresholds)
		assert.False(t, ts.Abort)
	})
}
