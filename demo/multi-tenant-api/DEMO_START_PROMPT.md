Multi-tenant SaaS API in Go where every data access carries proof of authentication and tenant-scoped authorization. The proof chain is: JWT validation → AuthenticatedUser → TenantAccess → ResourceAccess. You cannot construct a ResourceAccess without proving tenant membership first — the Go compiler enforces this through shengen guard types.

Stack: Go stdlib net/http, SQLite, htmx frontend for an admin dashboard. No frameworks.

Domain entities:
- Users with JWT tokens and tenant memberships
- Tenants with isolated data
- Resources (documents) owned by tenants
- API endpoints that require authorization proofs

Invariants:
- An authenticated user requires a non-expired JWT token
- Tenant access requires an authenticated user who is a member of that tenant
- Resource access requires tenant access where the tenant owns the resource
- No endpoint can return data without a ResourceAccess proof
- Cross-tenant data access is impossible by construction

Operations:
- POST /auth/login → issues JWT, returns AuthenticatedUser proof
- GET /tenants/:id/resources → requires TenantAccess proof
- GET /tenants/:id/resources/:rid → requires ResourceAccess proof
- POST /tenants/:id/resources → create resource, requires TenantAccess proof
- Admin dashboard showing tenants, users, and access logs

Use /sb:ralph-scaffold to set up the Ralph loop with four-gate backpressure, then run the loop to build it out.
