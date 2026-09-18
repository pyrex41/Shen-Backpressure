package app

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
