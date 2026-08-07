package core

import (
	"fmt"
	"math/big"
	"strings"
)

// CostStatus describes how trustworthy a cost amount is.
type CostStatus string

const (
	COST_STATUS_EXACT     CostStatus = "exact"
	COST_STATUS_ESTIMATED CostStatus = "estimated"
	COST_STATUS_FREE      CostStatus = "free"
	COST_STATUS_UNPRICED  CostStatus = "unpriced"
)

const USD_DECIMAL_PLACES = 10

var usdScale = new(big.Int).Exp(big.NewInt(10), big.NewInt(USD_DECIMAL_PLACES), nil)
var usdCentScale = new(big.Int).Exp(big.NewInt(10), big.NewInt(USD_DECIMAL_PLACES-2), nil)

// Cost is one USD accounting result. AmountUSD always uses ten fractional
// decimal places when Status is non-empty.
type Cost struct {
	AmountUSD   string     `json:"amount_usd"`
	Status      CostStatus `json:"status"`
	PricingAsOf string     `json:"pricing_as_of,omitempty"`
	Source      string     `json:"source,omitempty"`
}

// FreeCost returns the canonical zero-cost value.
func FreeCost() Cost {
	return Cost{AmountUSD: "0.0000000000", Status: COST_STATUS_FREE}
}

// UnpricedCost returns the canonical value for an unknown price.
func UnpricedCost() Cost {
	return Cost{AmountUSD: "0.0000000000", Status: COST_STATUS_UNPRICED}
}

// ExactCostFromUSDTicks converts integer 1e-10 USD ticks without precision
// loss.
func ExactCostFromUSDTicks(ticks int64) Cost {
	return Cost{
		AmountUSD: formatUSDTicks(big.NewInt(ticks)),
		Status:    COST_STATUS_EXACT,
	}
}

// Cents rounds the canonical USD decimal to the nearest cent using half-up
// rounding. Callers use it only at an integer persistence boundary.
func (c Cost) Cents() (int64, error) {
	ticks, err := parseUSDTicks(c.AmountUSD)
	if err != nil {
		return 0, fmt.Errorf("cost amount_usd %q: %w", c.AmountUSD, err)
	}
	cents, remainder := new(big.Int), new(big.Int)
	cents.QuoRem(ticks, usdCentScale, remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(usdCentScale) >= 0 {
		cents.Add(cents, big.NewInt(1))
	}
	if !cents.IsInt64() {
		return 0, fmt.Errorf("cost amount_usd %q exceeds int64 cents", c.AmountUSD)
	}
	return cents.Int64(), nil
}

// Add combines two costs. The numeric amount remains the known subtotal;
// status becomes unpriced when either operand has an unknown component.
func (c Cost) Add(other Cost) (Cost, error) {
	if c.Status == "" {
		return other, nil
	}
	if other.Status == "" {
		return c, nil
	}
	left, err := parseUSDTicks(c.AmountUSD)
	if err != nil {
		return Cost{}, fmt.Errorf("cost amount_usd %q: %w", c.AmountUSD, err)
	}
	right, err := parseUSDTicks(other.AmountUSD)
	if err != nil {
		return Cost{}, fmt.Errorf("cost amount_usd %q: %w", other.AmountUSD, err)
	}
	amount := new(big.Int).Add(left, right)
	return Cost{
		AmountUSD:   formatUSDTicks(amount),
		Status:      combineCostStatus(c.Status, other.Status),
		PricingAsOf: combineCostMetadata(c.PricingAsOf, other.PricingAsOf),
		Source:      combineCostMetadata(c.Source, other.Source),
	}, nil
}

func parseUSDTicks(value string) (*big.Int, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || len(parts[1]) != USD_DECIMAL_PLACES || parts[0] == "" {
		return nil, fmt.Errorf("must be a decimal with %d fractional digits", USD_DECIMAL_PLACES)
	}
	if strings.HasPrefix(parts[0], "-") {
		return nil, fmt.Errorf("must not be negative")
	}
	digits := parts[0] + parts[1]
	amount, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, fmt.Errorf("must contain only decimal digits")
	}
	return amount, nil
}

func formatUSDTicks(ticks *big.Int) string {
	if ticks == nil {
		return "0.0000000000"
	}
	negative := ticks.Sign() < 0
	abs := new(big.Int).Abs(new(big.Int).Set(ticks))
	whole, fraction := new(big.Int), new(big.Int)
	whole.QuoRem(abs, usdScale, fraction)
	prefix := ""
	if negative {
		prefix = "-"
	}
	return fmt.Sprintf("%s%s.%0*d", prefix, whole.String(), USD_DECIMAL_PLACES, fraction)
}

func combineCostStatus(left, right CostStatus) CostStatus {
	if left == COST_STATUS_UNPRICED || right == COST_STATUS_UNPRICED {
		return COST_STATUS_UNPRICED
	}
	if left == COST_STATUS_ESTIMATED || right == COST_STATUS_ESTIMATED {
		return COST_STATUS_ESTIMATED
	}
	if left == COST_STATUS_EXACT || right == COST_STATUS_EXACT {
		return COST_STATUS_EXACT
	}
	return COST_STATUS_FREE
}

func combineCostMetadata(left, right string) string {
	if left == right {
		return left
	}
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return "mixed"
}
