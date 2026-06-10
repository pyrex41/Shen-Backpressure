-- policy.lua — the Decidable-Shen-fragment tier, run on the REAL Shen kernel.
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
-- rule (sb-discharge-verified below): a ground premise holds iff it evaluates
-- to true. Evaluation of those premises is total BECAUSE the targets are in
-- the certified decidable fragment (no recursion, flat =/not/comparisons/
-- element? on ground data — see `sb policy --decidable` and
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
local specpath, bench = nil, false
do
  local i = 1
  while i <= #arg do
    if arg[i] == "--spec" then specpath = arg[i + 1]; i = i + 2
    elseif arg[i] == "--bench" then bench = true; i = i + 1
    else die("unknown arg: " .. arg[i]) end
  end
end
if not specpath then
  -- default: spec relative to this script's location
  local here = arg[0]:match("^(.*)[/\\][^/\\]*$") or "."
  specpath = here .. "/../../specs/core.shen"
end
do
  local f = io.open(specpath, "r")
  if not f then die("spec not found: " .. specpath .. " (use --spec)") end
  f:close()
end

-- ---- boot shen-lua ---------------------------------------------------------
local sldir = os.getenv("SHEN_LUA_DIR")
if sldir and sldir ~= "" then
  package.path = sldir .. "/?.lua;" .. package.path
end
local ok, shen = pcall(require, "shen")
if not ok then
  die("cannot require 'shen' — set SHEN_LUA_DIR to a shen-lua checkout " ..
      "or `luarocks install shen` (https://github.com/pyrex41/shen-lua)\n" ..
      tostring(shen))
end
shen.boot{ quiet = true }
local P, R = shen.prims, shen.runtime

-- ---- the discharge rule ----------------------------------------------------
-- A ground `: verified` premise holds iff it evaluates to true. The premises
-- of fragment-certified targets are total ground computations, so `eval` here
-- always terminates; trap-error fails closed on anything else.
shen.eval([[
(define sb.holds?
  Premise -> (trap-error (= true (eval Premise)) (/. E false)))
(datatype sb-discharge-verified
  if (sb.holds? Premise)
  ______________________
  Premise : verified;)
]])

-- ---- load the actual spec (source of truth, no lowering) -------------------
do
  local prev = P.GLOBALS["*hush*"]
  P.GLOBALS["*hush*"] = true        -- keep the load echo off the protocol stream
  local okl, res = pcall(shen.call, "load", specpath)
  P.GLOBALS["*hush*"] = prev
  if not okl then die("spec load failed: " .. tostring(res)) end
end

-- ---- syntax-tree construction ----------------------------------------------
-- shen.typecheck takes SYNTAX, not values: the syntax of the list [a b] is
-- the application (cons a (cons b ())). Strings/numbers/booleans are their
-- own syntax.
local CONS, NIL = R.intern("cons"), R.NIL
local function l3(a, b, c) return R.cons(a, R.cons(b, R.cons(c, NIL))) end
local function syn(arr, i)
  i = i or 1
  if i > #arr then return NIL end
  return l3(CONS, arr[i], syn(arr, i + 1))
end

-- Mirror of cedar-verify's makeDummyPrincipal: a valid HUMAN principal whose
-- only request-varying parts are the identity strings and the boolean flags
-- (iss/aud/sig non-empty, exp > 0, user-id bound to the JWT sub).
local function principal_syn(sub)
  local claims = syn{ sub, 9999999999, "shen-backpressure", "api", }
  local jwt    = syn{ claims, "dummy-sig-non-empty" }
  return syn{ jwt, sub }            -- authenticated-user (= the human principal)
end

local TENANT_ACCESS   = R.intern("tenant-access")
local RESOURCE_ACCESS = R.intern("resource-access")
local typecheck = P.F["shen.typecheck"]

local function allow(level, prin, tenant, resource, isMember, isOwned)
  local pterm = principal_syn(prin)
  local tterm = syn{ pterm, tenant, isMember }
  local term, ty
  if level == "tenant" then
    term, ty = tterm, TENANT_ACCESS
  elseif level == "resource" then
    term, ty = syn{ tterm, resource, isOwned }, RESOURCE_ACCESS
  else
    return false
  end
  local okc, res = pcall(typecheck, term, ty)
  return okc and res ~= false
end

-- ---- self-test: refuse to serve if the kernel disagrees with the spec ------
assert(allow("tenant",   "u1", "t1", "",   true,  false) == true,  "self-test: member must allow")
assert(allow("tenant",   "u1", "t1", "",   false, false) == false, "self-test: non-member must deny")
assert(allow("resource", "u1", "t1", "r1", true,  true)  == true,  "self-test: owned must allow")
assert(allow("resource", "u1", "t1", "r1", true,  false) == false, "self-test: unowned must deny")
assert(allow("resource", "u1", "t1", "r1", false, true)  == false, "self-test: resource needs tenant-access")

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
