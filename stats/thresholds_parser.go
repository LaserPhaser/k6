package stats

import (
	"fmt"

	c "go.k6.io/k6/pkg/combinators"
)

// ParseAssertion parses any aggregation method as defined in
// the BNF: `aggregation_method whitespace* operator whitespace* float`.
// The Result's `Payload interface{}` value will hold the
// assertion expression as a `[]interface{}` slice of len 3, its content
// will hold values of type `string` at position 0, `string` at position 1,
// and `float64` at position 2.
func ParseAssertion() c.Parser {
	parser := c.Sequence(
		ParseAggregationMethod(),
		c.DiscardAll(c.Whitespace()),
		ParseOperator(),
		c.DiscardAll(c.Whitespace()),
		ParseValue(),
		c.DiscardAll(c.Newline()),
	)

	return func(input []rune) c.Result {
		res := parser(input)
		if res.Err != nil {
			return res
		}

		return c.Success(res.Payload, res.Remaining)
	}
}

// ParseAggregationMethod parses any aggregation method as defined in
// the BNF: `aggregation_method -> trend | rate | gauge | counter`.
// The Result's `Payload interface{}` value will hold the
// aggregation method name as a string.
func ParseAggregationMethod() c.Parser {
	parser := c.Expect(c.Alternative(
		ParseCounter(),
		ParseGauge(),
		ParseRate(),
		ParseTrend(),
		ParsePercentile(),
	), "aggregation method")

	return func(input []rune) c.Result {
		res := parser(input)
		if res.Err != nil {
			return res
		}

		return c.Success(res.Payload, res.Remaining)
	}
}

// ParseOperator parses a threshold expression supported operator
// as defined in the BNF: `operator -> ">" | ">=" | "<=" | "<" | "==" | "===" | "!="`.
// The Result's `Payload interface{}` value will hold the
// operator expression as a string.
func ParseOperator() c.Parser {
	parser := c.Expect(c.Alternative(
		c.Tag(">="),
		c.Tag("<="),
		c.Tag(">"),
		c.Tag("<"),
		c.Tag("==="),
		c.Tag("=="),
		c.Tag("!="),
	), "operator")

	return func(input []rune) c.Result {
		res := parser(input)
		if res.Err != nil {
			return res
		}

		return c.Success(res.Payload.(string), res.Remaining)
	}
}

// ParseTrend parses a trend aggregation method as defined in
// the BNF: `trend -> "avg" | "min" | "max" | "med" | percentile`.
// The Result's `Payload interface{}` value will hold the
// trend's aggregation method name as a string.
func ParseTrend() c.Parser {
	parser := c.Expect(c.Alternative(
		c.Tag("mean"),
		c.Tag("min"),
		c.Tag("max"),
		c.Tag("avg"),
		c.Tag("med"),
		ParsePercentile(),
	), "trend aggregation method")

	return func(input []rune) c.Result {
		res := parser(input)
		if res.Err != nil {
			return res
		}

		return c.Success(res.Payload, res.Remaining)
	}
}

// ParsePercentile parses a percentile as defined in
// the BNF: `percentile -> "p(" float ")"`. The Result's `Payload interface{}`
// value will hold the percentile expression as a string.
func ParsePercentile() c.Parser {
	parser := c.Expect(c.Sequence(
		c.Tag("p("),
		c.Float(),
		c.Char(')'),
	), "percentile")

	return func(input []rune) c.Result {
		res := parser(input)
		if res.Err != nil {
			return res
		}

		parsed := res.Payload.([]interface{})
		percentile := fmt.Sprintf("%s%g%s", parsed[0].(string), parsed[1].(float64), parsed[2].(string))

		return c.Success(percentile, res.Remaining)
	}
}

// ParseRate parses a rate aggregation method as defined in
// the BNF: `rate -> "rate"`. The Result's `Payload interface{}` value
// will hold the rate's aggregation method name as a string.
func ParseRate() c.Parser {
	parser := c.Expect(c.Tag("rate"), "rate aggregation method")

	return func(input []rune) c.Result {
		res := parser(input)
		if res.Err != nil {
			return res
		}

		return c.Success(res.Payload, res.Remaining)
	}
}

// ParseGauge parses a gauge aggregation method as defined in
// the BNF: `gauge -> "value"`. The Result's `Payload interface{}` value
// will hold the gauge's aggregation method name as a string.
func ParseGauge() c.Parser {
	parser := c.Expect(c.Alternative(
		c.Tag("value"),
	), "gauge aggregation method")

	return func(input []rune) c.Result {
		res := parser(input)
		if res.Err != nil {
			return res
		}

		return c.Success(res.Payload, res.Remaining)
	}
}

// ParseCounter parses a counter aggregation method as defined in
// the BNF: `counter -> "count" | "rate"`. The Result's `Payload interface{}` value
// will hold the counter's aggregation method name as a string.
func ParseCounter() c.Parser {
	parser := c.Expect(c.Alternative(
		c.Tag("count"),
		c.Tag("rate"),
	), "counter aggregation method")

	return func(input []rune) c.Result {
		res := parser(input)
		if res.Err != nil {
			return res
		}

		return c.Success(res.Payload, res.Remaining)
	}
}

// ParseValue parses a threshold assertion value as defined in
// the BNF:
// ```
// float -> digit+ (. digit+)?
// digit -> "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9"
// ```
//
// The Result's `Payload interface{}` value will hold the assertion's right
// hand side's as a float64.
func ParseValue() c.Parser {
	parser := c.Expect(c.Float(), "numerical value")

	return func(input []rune) c.Result {
		res := parser(input)
		if res.Err != nil {
			return res
		}

		return c.Success(res.Payload, res.Remaining)
	}
}
