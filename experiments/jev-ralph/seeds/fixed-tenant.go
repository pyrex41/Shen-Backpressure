package app

func CanActOnTenant(actorTenant, requestedTenant, user string, memberships []Membership) bool {
	if actorTenant != requestedTenant {
		return false
	}
	for _, m := range memberships {
		if m.Tenant == requestedTenant && m.User == user && m.Active {
			return true
		}
	}
	return false
}

func CanViewTicket(viewerTenant, viewer string, support bool, t Ticket) bool {
	if viewerTenant != t.Tenant {
		return false
	}
	if support {
		return true
	}
	return !t.Private || viewer == t.Requester
}
