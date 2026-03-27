package handlers

import (
	"html/template"
	"net/http"
)

// admin dashboard data types
type tenantRow struct {
	ID   string
	Name string
}

type userRow struct {
	ID    string
	Email string
}

type membershipRow struct {
	UserID   string
	TenantID string
	Role     string
}

type logRow struct {
	ID         int
	UserID     string
	TenantID   string
	ResourceID string
	Action     string
	Allowed    bool
	Timestamp  string
}

type resourceRow struct {
	ID        string
	Title     string
	Body      string
	CreatedAt string
}

type adminData struct {
	Tenants     []tenantRow
	Users       []userRow
	Memberships []membershipRow
	Logs        []logRow
}

var adminTmpl = template.Must(template.New("admin").Parse(adminHTML))
var partialTenantResources = template.Must(template.New("tenant-resources").Parse(tenantResourcesHTML))

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	data := adminData{}

	// Load tenants
	rows, err := s.DB.Query("SELECT id, name FROM tenants ORDER BY name")
	if err != nil {
		http.Error(w, "query tenants: "+err.Error(), 500)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var t tenantRow
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			http.Error(w, "scan tenant: "+err.Error(), 500)
			return
		}
		data.Tenants = append(data.Tenants, t)
	}

	// Load users
	urows, err := s.DB.Query("SELECT id, email FROM users ORDER BY email")
	if err != nil {
		http.Error(w, "query users: "+err.Error(), 500)
		return
	}
	defer urows.Close()
	for urows.Next() {
		var u userRow
		if err := urows.Scan(&u.ID, &u.Email); err != nil {
			http.Error(w, "scan user: "+err.Error(), 500)
			return
		}
		data.Users = append(data.Users, u)
	}

	// Load memberships
	mrows, err := s.DB.Query("SELECT user_id, tenant_id, role FROM tenant_memberships ORDER BY tenant_id, user_id")
	if err != nil {
		http.Error(w, "query memberships: "+err.Error(), 500)
		return
	}
	defer mrows.Close()
	for mrows.Next() {
		var m membershipRow
		if err := mrows.Scan(&m.UserID, &m.TenantID, &m.Role); err != nil {
			http.Error(w, "scan membership: "+err.Error(), 500)
			return
		}
		data.Memberships = append(data.Memberships, m)
	}

	// Load recent access logs (last 50)
	lrows, err := s.DB.Query(
		"SELECT id, user_id, tenant_id, COALESCE(resource_id, ''), action, allowed, timestamp FROM access_logs ORDER BY id DESC LIMIT 50",
	)
	if err != nil {
		http.Error(w, "query logs: "+err.Error(), 500)
		return
	}
	defer lrows.Close()
	for lrows.Next() {
		var l logRow
		var allowedInt int
		if err := lrows.Scan(&l.ID, &l.UserID, &l.TenantID, &l.ResourceID, &l.Action, &allowedInt, &l.Timestamp); err != nil {
			http.Error(w, "scan log: "+err.Error(), 500)
			return
		}
		l.Allowed = allowedInt == 1
		data.Logs = append(data.Logs, l)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	adminTmpl.Execute(w, data)
}

func (s *Server) handleAdminTenantResources(w http.ResponseWriter, r *http.Request) {
	tid := r.PathValue("tid")

	rows, err := s.DB.Query(
		"SELECT id, title, body, created_at FROM resources WHERE tenant_id = ? ORDER BY created_at DESC", tid,
	)
	if err != nil {
		http.Error(w, "query resources: "+err.Error(), 500)
		return
	}
	defer rows.Close()

	var resources []resourceRow
	for rows.Next() {
		var res resourceRow
		if err := rows.Scan(&res.ID, &res.Title, &res.Body, &res.CreatedAt); err != nil {
			http.Error(w, "scan resource: "+err.Error(), 500)
			return
		}
		resources = append(resources, res)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	partialTenantResources.Execute(w, resources)
}

func (s *Server) handleAdminLogsPartial(w http.ResponseWriter, r *http.Request) {
	lrows, err := s.DB.Query(
		"SELECT id, user_id, tenant_id, COALESCE(resource_id, ''), action, allowed, timestamp FROM access_logs ORDER BY id DESC LIMIT 50",
	)
	if err != nil {
		http.Error(w, "query logs: "+err.Error(), 500)
		return
	}
	defer lrows.Close()

	var logs []logRow
	for lrows.Next() {
		var l logRow
		var allowedInt int
		if err := lrows.Scan(&l.ID, &l.UserID, &l.TenantID, &l.ResourceID, &l.Action, &allowedInt, &l.Timestamp); err != nil {
			http.Error(w, "scan log: "+err.Error(), 500)
			return
		}
		l.Allowed = allowedInt == 1
		logs = append(logs, l)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	partialLogsTable.Execute(w, logs)
}

var partialLogsTable = template.Must(template.New("logs-partial").Parse(logsTableHTML))

const logsTableHTML = `<table>
<thead><tr><th>#</th><th>User</th><th>Tenant</th><th>Resource</th><th>Action</th><th>Result</th><th>Time</th></tr></thead>
<tbody>
{{range .}}<tr class="{{if .Allowed}}allowed{{else}}denied{{end}}">
<td>{{.ID}}</td><td>{{.UserID}}</td><td>{{.TenantID}}</td><td>{{.ResourceID}}</td><td>{{.Action}}</td>
<td>{{if .Allowed}}ALLOWED{{else}}DENIED{{end}}</td><td>{{.Timestamp}}</td>
</tr>{{end}}
{{if not .}}<tr><td colspan="7" class="empty">No access logs yet</td></tr>{{end}}
</tbody></table>`

const tenantResourcesHTML = `{{if .}}<table>
<thead><tr><th>ID</th><th>Title</th><th>Body</th><th>Created</th></tr></thead>
<tbody>
{{range .}}<tr><td>{{.ID}}</td><td>{{.Title}}</td><td>{{.Body}}</td><td>{{.CreatedAt}}</td></tr>{{end}}
</tbody></table>
{{else}}<p class="empty">No resources for this tenant</p>{{end}}`

const adminHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Admin Dashboard — Multi-Tenant API</title>
<script src="https://unpkg.com/htmx.org@2.0.4"></script>
<style>
  :root { --bg: #0f1117; --surface: #1a1d27; --border: #2a2d3a; --text: #e0e0e8; --muted: #8888a0; --accent: #6c8cff; --green: #4caf88; --red: #e05555; }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: system-ui, -apple-system, sans-serif; background: var(--bg); color: var(--text); line-height: 1.5; }
  h1 { padding: 1.5rem 2rem 0.5rem; font-size: 1.5rem; font-weight: 600; }
  h1 span { color: var(--muted); font-weight: 400; font-size: 0.9rem; margin-left: 0.5rem; }
  .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; padding: 1rem 2rem; }
  @media (max-width: 900px) { .grid { grid-template-columns: 1fr; } }
  .card { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; padding: 1rem 1.25rem; }
  .card h2 { font-size: 0.85rem; text-transform: uppercase; letter-spacing: 0.05em; color: var(--muted); margin-bottom: 0.75rem; }
  .full-width { grid-column: 1 / -1; }
  table { width: 100%; border-collapse: collapse; font-size: 0.85rem; }
  th { text-align: left; padding: 0.4rem 0.6rem; border-bottom: 1px solid var(--border); color: var(--muted); font-weight: 500; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.05em; }
  td { padding: 0.4rem 0.6rem; border-bottom: 1px solid var(--border); }
  tr:last-child td { border-bottom: none; }
  tr.allowed td:nth-child(6) { color: var(--green); font-weight: 600; }
  tr.denied td:nth-child(6) { color: var(--red); font-weight: 600; }
  .empty { color: var(--muted); font-style: italic; text-align: center; padding: 1rem; }
  .tenant-btn { display: inline-block; background: var(--border); color: var(--text); border: none; border-radius: 4px; padding: 0.3rem 0.75rem; margin: 0.15rem; cursor: pointer; font-size: 0.8rem; transition: background 0.15s; }
  .tenant-btn:hover, .tenant-btn.htmx-request { background: var(--accent); color: #fff; }
  #resource-browser { min-height: 3rem; }
  .htmx-indicator { display: none; color: var(--muted); font-style: italic; }
  .htmx-request .htmx-indicator { display: inline; }
  .refresh-btn { float: right; background: none; border: 1px solid var(--border); color: var(--muted); border-radius: 4px; padding: 0.2rem 0.6rem; cursor: pointer; font-size: 0.75rem; }
  .refresh-btn:hover { border-color: var(--accent); color: var(--accent); }
</style>
</head>
<body>
<h1>Admin Dashboard <span>Multi-Tenant API</span></h1>
<div class="grid">

  <div class="card">
    <h2>Tenants</h2>
    <table>
    <thead><tr><th>ID</th><th>Name</th></tr></thead>
    <tbody>
    {{range .Tenants}}<tr><td>{{.ID}}</td><td>{{.Name}}</td></tr>{{end}}
    {{if not .Tenants}}<tr><td colspan="2" class="empty">No tenants</td></tr>{{end}}
    </tbody></table>
  </div>

  <div class="card">
    <h2>Users</h2>
    <table>
    <thead><tr><th>ID</th><th>Email</th></tr></thead>
    <tbody>
    {{range .Users}}<tr><td>{{.ID}}</td><td>{{.Email}}</td></tr>{{end}}
    {{if not .Users}}<tr><td colspan="2" class="empty">No users</td></tr>{{end}}
    </tbody></table>
  </div>

  <div class="card full-width">
    <h2>Tenant Memberships</h2>
    <table>
    <thead><tr><th>User</th><th>Tenant</th><th>Role</th></tr></thead>
    <tbody>
    {{range .Memberships}}<tr><td>{{.UserID}}</td><td>{{.TenantID}}</td><td>{{.Role}}</td></tr>{{end}}
    {{if not .Memberships}}<tr><td colspan="3" class="empty">No memberships</td></tr>{{end}}
    </tbody></table>
  </div>

  <div class="card full-width">
    <h2>Resource Browser
      <span class="htmx-indicator">Loading...</span>
    </h2>
    <p style="margin-bottom:0.5rem; font-size:0.85rem; color:var(--muted);">Select a tenant to browse resources:</p>
    {{range .Tenants}}
    <button class="tenant-btn"
            hx-get="/admin/tenants/{{.ID}}/resources"
            hx-target="#resource-browser"
            hx-swap="innerHTML">{{.Name}} ({{.ID}})</button>
    {{end}}
    <div id="resource-browser" style="margin-top:0.75rem;">
      <p class="empty">Click a tenant above</p>
    </div>
  </div>

  <div class="card full-width">
    <h2>Access Logs
      <button class="refresh-btn"
              hx-get="/admin/logs"
              hx-target="#logs-container"
              hx-swap="innerHTML">Refresh</button>
    </h2>
    <div id="logs-container">
    <table>
    <thead><tr><th>#</th><th>User</th><th>Tenant</th><th>Resource</th><th>Action</th><th>Result</th><th>Time</th></tr></thead>
    <tbody>
    {{range .Logs}}<tr class="{{if .Allowed}}allowed{{else}}denied{{end}}">
    <td>{{.ID}}</td><td>{{.UserID}}</td><td>{{.TenantID}}</td><td>{{.ResourceID}}</td><td>{{.Action}}</td>
    <td>{{if .Allowed}}ALLOWED{{else}}DENIED{{end}}</td><td>{{.Timestamp}}</td>
    </tr>{{end}}
    {{if not .Logs}}<tr><td colspan="7" class="empty">No access logs yet</td></tr>{{end}}
    </tbody></table>
    </div>
  </div>

</div>
</body>
</html>`
