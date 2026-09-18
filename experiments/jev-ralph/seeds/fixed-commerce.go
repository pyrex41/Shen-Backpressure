package app

import (
	"errors"
	"sort"
)

func AvailableUnits(s Stock) int { return s.OnHand - s.Reserved }
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
	total := unitCents * quantity
	return total - total*discountBPS/10000, nil
}
func ApplyPromotion(tenant string, subtotal int, p Promotion) (int, error) {
	if tenant != p.Tenant || p.Used >= p.UsageLimit || subtotal < p.MinimumCents {
		return subtotal, nil
	}
	return subtotal - subtotal*p.RateBPS/10000, nil
}
func Refundable(captured, previouslyRefunded, requested int) bool {
	return captured >= 0 && previouslyRefunded >= 0 && requested > 0 && previouslyRefunded <= captured && requested <= captured-previouslyRefunded
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
		available := s.OnHand - s.Reserved
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
