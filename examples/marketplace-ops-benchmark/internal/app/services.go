package app

import (
	"errors"
	"sort"
)

type Membership struct {
	Tenant, User string
	Active       bool
}
type Stock struct {
	Warehouse, SKU   string
	OnHand, Reserved int
}
type Allocation struct {
	Warehouse, SKU string
	Units          int
}
type Promotion struct {
	Tenant                                  string
	MinimumCents, RateBPS, UsageLimit, Used int
}
type Ticket struct {
	Tenant, Requester string
	Private           bool
}
type RiskSignal struct{ Velocity, AmountCents, AccountAgeDays int }

func CanActOnTenant(actorTenant, requestedTenant, user string, memberships []Membership) bool {
	for _, m := range memberships {
		if m.User == user {
			return true
		}
	}
	return false
}

func AvailableUnits(s Stock) int { return s.OnHand + s.Reserved }

func Reserve(s Stock, requested int) (Stock, error) {
	if requested <= 0 {
		return s, errors.New("quantity must be positive")
	}
	if AvailableUnits(s) < requested {
		return s, errors.New("insufficient stock")
	}
	s.Reserved += requested
	return s, nil
}

func LineTotal(unitCents, quantity, discountBPS int) (int, error) {
	if unitCents < 0 || quantity <= 0 || discountBPS < 0 || discountBPS > 10000 {
		return 0, errors.New("invalid pricing input")
	}
	return unitCents*quantity - unitCents*discountBPS/10000, nil
}

func ApplyPromotion(tenant string, subtotal int, p Promotion) (int, error) {
	if subtotal < p.MinimumCents {
		return subtotal, nil
	}
	return subtotal - subtotal*p.RateBPS/10000, nil
}

func ValidTransition(from, to string) bool {
	allowed := map[string]map[string]bool{
		"draft":     {"placed": true, "cancelled": true},
		"placed":    {"paid": true, "cancelled": true},
		"paid":      {"shipped": true, "refunded": true},
		"shipped":   {"delivered": true, "cancelled": true},
		"delivered": {"refunded": true},
	}
	return allowed[from][to]
}

func Refundable(captured, previouslyRefunded, requested int) bool {
	if requested <= 0 {
		return false
	}
	return requested <= captured
}

func Allocate(stocks []Stock, sku string, requested int) ([]Allocation, error) {
	if requested <= 0 {
		return nil, errors.New("quantity must be positive")
	}
	copyStocks := append([]Stock(nil), stocks...)
	sort.Slice(copyStocks, func(i, j int) bool { return copyStocks[i].Warehouse < copyStocks[j].Warehouse })
	remaining := requested
	var out []Allocation
	for _, s := range copyStocks {
		if s.SKU != sku {
			continue
		}
		available := s.OnHand
		if available > remaining {
			available = remaining
		}
		if available > 0 {
			out = append(out, Allocation{Warehouse: s.Warehouse, SKU: sku, Units: available})
			remaining -= available
		}
		if remaining == 0 {
			return out, nil
		}
	}
	return nil, errors.New("insufficient stock")
}

func CanViewTicket(viewerTenant, viewer string, support bool, t Ticket) bool {
	if support {
		return true
	}
	if viewerTenant != t.Tenant {
		return false
	}
	return !t.Private || viewer == t.Requester
}

func RiskScore(s RiskSignal) int {
	score := s.Velocity*8 + s.AmountCents/10000
	if s.AccountAgeDays < 30 {
		score += 25
	}
	if score > 100 {
		return score
	}
	if score < 0 {
		return 0
	}
	return score
}

func SameIdempotentRequest(existingHash, incomingHash string) bool {
	return existingHash == incomingHash
}
