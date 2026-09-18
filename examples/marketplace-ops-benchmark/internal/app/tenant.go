package app

func CanActOnTenant(actorTenant, requestedTenant, user string, memberships []Membership) bool {
	for _, m := range memberships {
		if m.User == user {
			return true
		}
	}
	return false
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
