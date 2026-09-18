package app

func ValidTransition(from, to string) bool {
	allowed := map[string]map[string]bool{
		"draft": {"placed": true, "cancelled": true}, "placed": {"paid": true, "cancelled": true},
		"paid": {"shipped": true, "refunded": true}, "shipped": {"delivered": true, "cancelled": true},
		"delivered": {"refunded": true},
	}
	return allowed[from][to]
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
