package app

import "testing"

func TestContractTenantIsolation(t *testing.T) {
	ms := []Membership{{Tenant: "red", User: "u1", Active: true}, {Tenant: "blue", User: "u2", Active: false}}
	if CanActOnTenant("red", "blue", "u1", ms) {
		t.Fatal("cross-tenant membership granted access")
	}
	if CanActOnTenant("blue", "blue", "u2", ms) {
		t.Fatal("inactive membership granted access")
	}
}

func TestContractInventoryReservation(t *testing.T) {
	s := Stock{Warehouse: "w1", SKU: "x", OnHand: 10, Reserved: 7}
	if AvailableUnits(s) != 3 {
		t.Fatalf("available=%d want 3", AvailableUnits(s))
	}
	if _, err := Reserve(s, 4); err == nil {
		t.Fatal("reserved beyond availability")
	}
}

func TestContractPricing(t *testing.T) {
	got, err := LineTotal(1000, 3, 1000)
	if err != nil || got != 2700 {
		t.Fatalf("total=%d err=%v want 2700", got, err)
	}
}

func TestContractPromotionPolicy(t *testing.T) {
	p := Promotion{Tenant: "red", MinimumCents: 1000, RateBPS: 1000, UsageLimit: 2, Used: 2}
	if got, _ := ApplyPromotion("blue", 2000, p); got != 2000 {
		t.Fatalf("cross-tenant promotion changed total: %d", got)
	}
	if got, _ := ApplyPromotion("red", 2000, p); got != 2000 {
		t.Fatalf("exhausted promotion changed total: %d", got)
	}
}

func TestContractOrderTransitions(t *testing.T) {
	if ValidTransition("shipped", "cancelled") {
		t.Fatal("shipped order was cancelled")
	}
	if !ValidTransition("shipped", "delivered") {
		t.Fatal("delivery transition rejected")
	}
}

func TestContractRefundAccounting(t *testing.T) {
	if Refundable(1000, 800, 300) {
		t.Fatal("cumulative refund exceeded capture")
	}
	if !Refundable(1000, 800, 200) {
		t.Fatal("remaining refundable amount rejected")
	}
}

func TestContractShipmentAllocation(t *testing.T) {
	stocks := []Stock{{Warehouse: "a", SKU: "x", OnHand: 5, Reserved: 4}, {Warehouse: "b", SKU: "x", OnHand: 3, Reserved: 0}}
	got, err := Allocate(stocks, "x", 4)
	if err != nil || len(got) != 2 || got[0].Units != 1 || got[1].Units != 3 {
		t.Fatalf("allocation=%+v err=%v", got, err)
	}
}

func TestContractSupportTenantBoundary(t *testing.T) {
	ticket := Ticket{Tenant: "red", Requester: "u1", Private: true}
	if CanViewTicket("blue", "agent", true, ticket) {
		t.Fatal("support crossed tenant boundary")
	}
	if !CanViewTicket("red", "agent", true, ticket) {
		t.Fatal("same-tenant support denied")
	}
}

func TestContractRiskScoreBounded(t *testing.T) {
	if got := RiskScore(RiskSignal{Velocity: 20, AmountCents: 500000, AccountAgeDays: 1}); got != 100 {
		t.Fatalf("risk=%d want 100", got)
	}
}

func TestContractIdempotency(t *testing.T) {
	if !SameIdempotentRequest("abc", "abc") || SameIdempotentRequest("abc", "def") {
		t.Fatal("idempotency hash comparison broken")
	}
}
