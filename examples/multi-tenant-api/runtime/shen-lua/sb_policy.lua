-- sb_policy.lua — the decidable-fragment policy core on the real Shen kernel.
--
-- Shared by the CLI/differential runner (policy.lua) and the OpenResty demo
-- (../openresty/). See policy.lua's header comment for the full story; the
-- short version: authorization is decided by TYPE INHABITATION — the kernel's
-- sequent-calculus typechecker (shen.typecheck) judges whether a request's
-- ground term inhabits tenant-access / resource-access, with `: verified`
-- premises discharged by total ground evaluation (the decidable-fragment
-- certification is the termination argument).
--
-- Usage:
--   local sb = require("sb_policy")
--   sb.init{ spec = "/path/to/core.shen" }   -- boots shen-lua, loads the spec
--   sb.allow("tenant",   user, tenant, "",       isMember, false)
--   sb.allow("resource", user, tenant, resource, isMember, isOwned)
--
-- Locating shen-lua: set SHEN_LUA_DIR to a checkout, or have the `shen` rock
-- installed (luarocks install shen) on package.path.

local M = {}

local shen, P, R
local CONS, NIL, TENANT_ACCESS, RESOURCE_ACCESS, typecheck

local function l3(a, b, c) return R.cons(a, R.cons(b, R.cons(c, NIL))) end

-- shen.typecheck takes SYNTAX, not values: the syntax of the list [a b] is
-- the application (cons a (cons b ())). Strings/numbers/booleans are their
-- own syntax.
local function syn(arr, i)
  i = i or 1
  if i > #arr then return NIL end
  return l3(CONS, arr[i], syn(arr, i + 1))
end

-- Valid HUMAN principal: only the identity strings and boolean flags vary per
-- request (iss/aud/sig non-empty, exp > 0, user-id bound to the JWT sub) —
-- mirrors cedar-verify's makeDummyPrincipal.
local function principal_syn(sub)
  local claims = syn{ sub, 9999999999, "shen-backpressure", "api" }
  local jwt    = syn{ claims, "dummy-sig-non-empty" }
  return syn{ jwt, sub }            -- authenticated-user (= the human principal)
end

function M.init(opts)
  opts = opts or {}
  if shen then return M end          -- idempotent

  local sldir = opts.shen_lua_dir or os.getenv("SHEN_LUA_DIR")
  if sldir and sldir ~= "" then
    package.path = sldir .. "/?.lua;" .. package.path
  end
  local ok, mod = pcall(require, "shen")
  if not ok then
    error("sb_policy: cannot require 'shen' — set SHEN_LUA_DIR to a shen-lua " ..
          "checkout or `luarocks install shen` " ..
          "(https://github.com/pyrex41/shen-lua)\n" .. tostring(mod), 0)
  end
  shen = mod
  shen.boot{ quiet = true }
  P, R = shen.prims, shen.runtime

  -- The discharge rule: a ground `: verified` premise holds iff it evaluates
  -- to true. Total for fragment-certified targets; trap-error fails closed.
  shen.eval([[
(define sb.holds?
  Premise -> (trap-error (= true (eval Premise)) (/. E false)))
(datatype sb-discharge-verified
  if (sb.holds? Premise)
  ______________________
  Premise : verified;)
]])

  -- Load the actual spec (source of truth, no lowering), echo hushed so
  -- protocol/log streams stay clean.
  local spec = assert(opts.spec, "sb_policy.init: opts.spec (path to core.shen) required")
  do
    local f = assert(io.open(spec, "r"), "sb_policy: spec not found: " .. spec)
    f:close()
  end
  local prev = P.GLOBALS["*hush*"]
  P.GLOBALS["*hush*"] = true
  local okl, err = pcall(shen.call, "load", spec)
  P.GLOBALS["*hush*"] = prev
  if not okl then error("sb_policy: spec load failed: " .. tostring(err), 0) end

  CONS, NIL = R.intern("cons"), R.NIL
  TENANT_ACCESS   = R.intern("tenant-access")
  RESOURCE_ACCESS = R.intern("resource-access")
  typecheck = P.F["shen.typecheck"]

  -- Self-test: refuse to serve if the kernel disagrees with the spec.
  assert(M.allow("tenant",   "u1", "t1", "",   true,  false) == true,  "self-test: member must allow")
  assert(M.allow("tenant",   "u1", "t1", "",   false, false) == false, "self-test: non-member must deny")
  assert(M.allow("resource", "u1", "t1", "r1", true,  true)  == true,  "self-test: owned must allow")
  assert(M.allow("resource", "u1", "t1", "r1", true,  false) == false, "self-test: unowned must deny")
  assert(M.allow("resource", "u1", "t1", "r1", false, true)  == false, "self-test: resource needs tenant-access")
  return M
end

function M.allow(level, prin, tenant, resource, isMember, isOwned)
  -- Reset the kernel's inference counter per decision (the kernel REPL does
  -- exactly this per evaluation — toplevel.kl). The counter is GLOBAL and
  -- cumulative; without the reset a long-lived process (an nginx worker)
  -- crosses *maxinferences* after a few thousand checks and the engine
  -- throws maxinfexceeded on every check thereafter (fail-closed denies —
  -- caught live in the OpenResty demo). With the reset, *maxinferences*
  -- becomes a PER-DECISION inference budget: a check that exceeds it is
  -- denied, which is the right failure mode for a decidable policy tier.
  P.GLOBALS["shen.*infs*"] = 0
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

return M
