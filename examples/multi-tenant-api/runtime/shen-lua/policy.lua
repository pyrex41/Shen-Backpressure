-- policy.lua — CLI/differential runner for the Decidable-Shen-fragment tier
-- on the REAL Shen kernel.
--
-- Every other tier in the lattice (Cedar, Rego, the Go EvalClauses evaluator,
-- the generated guard ctors) is a LOWERING of specs/core.shen. This runner is
-- not: it loads the spec itself into shen-lua (a certified Shen 41.1 port) and
-- answers each authorization request by asking the kernel's sequent-calculus
-- typechecker whether the request's ground term INHABITS the access type:
--
--     allow(level=tenant, P, T, isMember)
--        :<=>  [principal T isMember] : tenant-access
--
-- The `: verified` premises in the spec are discharged by one extra datatype
-- rule (see sb_policy.lua): a ground premise holds iff it evaluates to true.
-- Evaluation of those premises is total BECAUSE the targets are in the
-- certified decidable fragment (no recursion, flat =/not/comparisons/element?
-- on ground data — see `sb policy --decidable` and
-- specs/decidable-fragment.cert). That certification is the termination
-- argument for this runner; outside the fragment, premise evaluation could
-- diverge and this tier must not be used.
--
-- Protocol (stdin/stdout, one request per line, tab-separated):
--   PING                                          -> READY
--   CHECK <level> <principal> <tenant> <resource> <isMember> <isOwned>
--                                                 -> allow | deny
--   level in {tenant, resource}; booleans are "true"/"false".
-- Any malformed request or evaluation error answers "deny" (fail closed).
--
-- Locating shen-lua: set SHEN_LUA_DIR to a shen-lua checkout, or have the
-- `shen` rock installed (luarocks install shen) on package.path.
--
-- Usage:
--   luajit policy.lua --spec ../../specs/core.shen            # serve protocol
--   luajit policy.lua --spec ../../specs/core.shen --bench    # µs/check timing

local function die(msg)
  io.stderr:write("policy.lua: " .. msg .. "\n")
  os.exit(1)
end

-- ---- args ------------------------------------------------------------------
local here = arg[0]:match("^(.*)[/\\][^/\\]*$") or "."
local specpath, bench = nil, false
do
  local i = 1
  while i <= #arg do
    if arg[i] == "--spec" then specpath = arg[i + 1]; i = i + 2
    elseif arg[i] == "--bench" then bench = true; i = i + 1
    else die("unknown arg: " .. arg[i]) end
  end
end
specpath = specpath or (here .. "/../../specs/core.shen")

-- ---- boot the shared policy core --------------------------------------------
package.path = here .. "/?.lua;" .. package.path
local ok, sb = pcall(function()
  return require("sb_policy").init{ spec = specpath }
end)
if not ok then die(tostring(sb)) end
local allow = sb.allow

-- ---- bench mode -------------------------------------------------------------
if bench then
  local N = 5000
  local t0 = os.clock()
  for i = 1, N do allow("resource", "u1", "t1", "r1", true, true) end
  local dt = os.clock() - t0
  print(string.format("resource-access (full chain, allow): %.1f us/check", dt / N * 1e6))
  t0 = os.clock()
  for i = 1, N do allow("tenant", "u1", "t1", "", false, false) end
  dt = os.clock() - t0
  print(string.format("tenant-access (deny path):           %.1f us/check", dt / N * 1e6))
  os.exit(0)
end

-- ---- serve ------------------------------------------------------------------
io.stdout:setvbuf("line")
for line in io.lines() do
  if line == "PING" then
    io.write("READY\n")
  elseif line == "QUIT" then
    break
  else
    local level, prin, tenant, resource, mem, own =
      line:match("^CHECK\t([^\t]*)\t([^\t]*)\t([^\t]*)\t([^\t]*)\t([^\t]*)\t([^\t]*)$")
    if level then
      local a = allow(level, prin, tenant, resource, mem == "true", own == "true")
      io.write(a and "allow\n" or "deny\n")
    else
      io.write("deny\n")           -- malformed: fail closed
    end
  end
end
