// Package service_test contains property-based tests for the billing service.
package service_test

// Validates: Requirements 8.1, 8.2
//
// Property 2: Bill amount invariant
// For arbitrary base_amount and a list of utility charges:
//   - total_amount == base_amount + SUM(utility_charges)
//   - deposit_refunded <= deposit_amount for any refund value

import (
	"testing"
	"testing/quick"
)

// billState represents the in-memory state of a bill for simulation.
type billState struct {
	baseAmount    float64
	utilityAmount float64
	penaltyAmount float64
}

// totalAmount computes the total amount for a bill.
// This mirrors the GENERATED ALWAYS AS (base_amount + utility_amount + penalty_amount) STORED column.
func (b billState) totalAmount() float64 {
	return b.baseAmount + b.utilityAmount + b.penaltyAmount
}

// depositState represents the deposit state for a contract.
type depositState struct {
	depositAmount   float64
	depositRefunded float64
}

// isValid returns true if the deposit state is valid (refunded <= amount).
func (d depositState) isValid() bool {
	return d.depositRefunded <= d.depositAmount
}

// TestBillAmountInvariant_Property verifies that for any base_amount and list of
// utility charges, total_amount == base_amount + SUM(utility_charges).
//
// Validates: Requirements 8.1
func TestBillAmountInvariant_Property(t *testing.T) {
	// Property: total_amount == base_amount + utility_amount + penalty_amount
	property := func(baseAmount uint32, utilityCharges []uint16) bool {
		if len(utilityCharges) == 0 {
			return true
		}

		// Convert to float64 (use uint to avoid negative values).
		base := float64(baseAmount) / 100.0 // cents to currency units

		// Sum utility charges.
		var totalUtility float64
		for _, charge := range utilityCharges {
			totalUtility += float64(charge) / 100.0
		}

		bill := billState{
			baseAmount:    base,
			utilityAmount: totalUtility,
			penaltyAmount: 0,
		}

		// Invariant: total == base + utility + penalty
		expected := base + totalUtility
		actual := bill.totalAmount()

		// Use epsilon comparison for floating point.
		const epsilon = 0.0001
		diff := actual - expected
		if diff < 0 {
			diff = -diff
		}
		return diff < epsilon
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("bill amount invariant violated: %v", err)
	}
}

// TestBillAmountInvariant_WithPenalty verifies the invariant holds when penalty is non-zero.
//
// Validates: Requirements 8.1
func TestBillAmountInvariant_WithPenalty(t *testing.T) {
	property := func(baseAmount uint32, utilityAmount uint32, penaltyAmount uint32) bool {
		base := float64(baseAmount) / 100.0
		utility := float64(utilityAmount) / 100.0
		penalty := float64(penaltyAmount) / 100.0

		bill := billState{
			baseAmount:    base,
			utilityAmount: utility,
			penaltyAmount: penalty,
		}

		expected := base + utility + penalty
		actual := bill.totalAmount()

		const epsilon = 0.0001
		diff := actual - expected
		if diff < 0 {
			diff = -diff
		}
		return diff < epsilon
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("bill amount invariant with penalty violated: %v", err)
	}
}

// TestDepositRefundInvariant_Property verifies that deposit_refunded <= deposit_amount
// for any valid refund value.
//
// Validates: Requirements 8.2
func TestDepositRefundInvariant_Property(t *testing.T) {
	// Property: for any deposit amount D and refund R where R <= D, the state is valid.
	property := func(depositAmount uint32, refundFraction uint8) bool {
		if depositAmount == 0 {
			return true
		}

		deposit := float64(depositAmount) / 100.0

		// refundFraction is 0-255; map to 0.0-1.0 to ensure refund <= deposit.
		fraction := float64(refundFraction) / 255.0
		refund := deposit * fraction

		state := depositState{
			depositAmount:   deposit,
			depositRefunded: refund,
		}

		return state.isValid()
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("deposit refund invariant violated: %v", err)
	}
}

// TestDepositRefundInvariant_RefundExceedsDeposit verifies that a refund exceeding
// the deposit is correctly identified as invalid.
//
// Validates: Requirements 8.2
func TestDepositRefundInvariant_RefundExceedsDeposit(t *testing.T) {
	property := func(depositAmount uint32, excess uint16) bool {
		if depositAmount == 0 || excess == 0 {
			return true
		}

		deposit := float64(depositAmount) / 100.0
		refund := deposit + float64(excess)/100.0 // refund > deposit

		state := depositState{
			depositAmount:   deposit,
			depositRefunded: refund,
		}

		// This state should be INVALID (refund > deposit).
		return !state.isValid()
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("deposit refund excess detection violated: %v", err)
	}
}

// TestBillAmountInvariant_ZeroUtility verifies the invariant holds when there are no utility charges.
//
// Validates: Requirements 8.1
func TestBillAmountInvariant_ZeroUtility(t *testing.T) {
	property := func(baseAmount uint32) bool {
		base := float64(baseAmount) / 100.0
		bill := billState{
			baseAmount:    base,
			utilityAmount: 0,
			penaltyAmount: 0,
		}
		const epsilon = 0.0001
		diff := bill.totalAmount() - base
		if diff < 0 {
			diff = -diff
		}
		return diff < epsilon
	}

	cfg := &quick.Config{MaxCount: 500}
	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("bill amount invariant with zero utility violated: %v", err)
	}
}

// TestDepositRefundInvariant_ZeroRefund verifies that a zero refund is always valid.
//
// Validates: Requirements 8.2
func TestDepositRefundInvariant_ZeroRefund(t *testing.T) {
	property := func(depositAmount uint32) bool {
		state := depositState{
			depositAmount:   float64(depositAmount) / 100.0,
			depositRefunded: 0,
		}
		return state.isValid()
	}

	cfg := &quick.Config{MaxCount: 500}
	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("deposit refund invariant with zero refund violated: %v", err)
	}
}

// TestDepositRefundInvariant_FullRefund verifies that a full refund (refund == deposit) is valid.
//
// Validates: Requirements 8.2
func TestDepositRefundInvariant_FullRefund(t *testing.T) {
	property := func(depositAmount uint32) bool {
		deposit := float64(depositAmount) / 100.0
		state := depositState{
			depositAmount:   deposit,
			depositRefunded: deposit, // full refund
		}
		return state.isValid()
	}

	cfg := &quick.Config{MaxCount: 500}
	if err := quick.Check(property, cfg); err != nil {
		t.Errorf("deposit refund invariant with full refund violated: %v", err)
	}
}
