# Behavioural smoke test for the Elixir generated from features.shen.
# Run by elixir_test.go: elixir -pa <ebin> features_smoke.exs
alias Feat.Shen.{AccountId, Amount, Audited, BalanceChecked, Currency, Defines, Money, SafeTransfer}
alias Feat.Shen.{Settled, StepCall, StepWait, Term, Transfer, Workflow, WorkflowStep}

check = fn label, value ->
  unless value, do: raise("smoke check failed: #{label}")
end

a = AccountId.new!("a")
b = AccountId.new!("b")
usd = Currency.new!(:usd)
ten = Money.new!(Amount.new!(10), usd)

# wrappers / constrained
check.("amount >= 0", Amount.new(-1) == {:error, {:premise, "(>= X 0)", [x: -1]}})
check.("amount type", match?({:error, {:type, "X : number", _}}, Amount.new("1")))
check.("currency element?", match?({:error, {:premise, "(element? X [usd eur gbp])", _}}, Currency.new(:jpy)))

# guarded: not-equal + head of nested composite
check.("from != to", match?({:error, {:premise, "(not (= From To))", _}}, Transfer.new(a, a, ten)))
zero = Money.new!(Amount.new!(0), usd)
check.("(> (head Value) 0)", match?({:error, {:premise, "(> (head Value) 0)", _}}, Transfer.new(a, b, zero)))
tx = Transfer.new!(a, b, ten)

# deep head/tail chain
check.("balance >= amount", match?({:error, {:premise, _, _}}, BalanceChecked.new(5, tx)))
bc = BalanceChecked.new!(10, tx)

# structural equality between guard values
tx2 = Transfer.new!(b, a, ten)
check.("(= Tx (head (tail Check)))", match?({:error, {:premise, "(= Tx (head (tail Check)))", _}}, SafeTransfer.new(tx2, bc)))
safe = SafeTransfer.new!(tx, bc)
check.("alias", match?({:ok, _}, Settled.new(safe)))
check.("alias type", match?({:error, {:type, _, _}}, Settled.new(tx)))

# Shen representation
check.("to_shen", Term.to_shen(tx) == ["a", "b", [10, :usd]])

# tagged sum type + define with non-linear patterns
wait = StepWait.new!(Amount.new!(5))
call_a = StepCall.new!(a, 2)
call_b = StepCall.new!(b, 1)
check.("tagged to_shen", Term.to_shen(call_a) == [:call, "a", 2] and Term.to_shen(wait) == [:wait, 5])
check.("sum member?", WorkflowStep.member?(wait) and not WorkflowStep.member?(a))
check.("retries <= 3", match?({:error, _}, StepCall.new(a, 4)))
check.("workflow ok", match?({:ok, _}, Workflow.new([wait, call_a], a)))
check.("workflow owner", match?({:error, {:premise, "(allowed-owner? Owner Steps)", _}}, Workflow.new([call_a, call_b], a)))
check.("workflow non-empty", match?({:error, {:premise, "(> (length Steps) 0)", _}}, Workflow.new([], a)))
check.("workflow list type", match?({:error, {:type, _, _}}, Workflow.new([a], a)))

# runtime-via premise delegates to Feat.Shen.Runtime
check.("runtime-via", match?({:error, {:premise, _, _}}, Audited.new(a, "")) and match?({:ok, _}, Audited.new(a, "x")))

# define lowering: where-guards, first-match order, non-linear patterns
check.("classify", Enum.map([1, 50, 500], &Defines.classify/1) == [:small, :medium, :large])
check.("same?", Defines.same?("x", "x") and not Defines.same?("x", "y"))
check.("same? unwraps guards", Defines.same?(a, "a"))
check.("total-retries", Defines.total_retries([wait, call_a, call_b]) == 3)

# new! raises GuardError carrying the premise
try do
  Amount.new!(-5)
  raise "expected GuardError"
rescue
  e in Feat.Shen.GuardError -> check.("GuardError message", Exception.message(e) =~ "(>= X 0)")
end

IO.puts("features smoke: ok")
