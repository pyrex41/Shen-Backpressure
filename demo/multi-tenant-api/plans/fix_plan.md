# Multi-Tenant SaaS API — Implementation Plan

- [x] Set up SQLite database schema (users, tenants, tenant_memberships, resources, access_logs)
- [x] Implement JWT signing and validation (login endpoint, token parsing middleware)
- [x] Implement authentication proof construction (JWT → AuthenticatedUser with guard types)
- [x] Implement tenant membership lookup and TenantAccess proof construction
- [x] Implement resource ownership check and ResourceAccess proof construction
- [x] Implement HTTP handlers: POST /auth/login, GET /tenants/:id/resources, GET /tenants/:id/resources/:rid, POST /tenants/:id/resources
- [x] Build htmx admin dashboard (tenant list, user list, access logs, resource browser)
- [x] Integration tests for proof chain (cross-tenant access rejected, expired token rejected, non-member rejected, valid access accepted)
