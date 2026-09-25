package main

// derive.go — `shengen-ex derive`: the Elixir leg of shen-derive.
//
// Generates an ExUnit module that pins an Elixir implementation function
// against a Shen (define ...) evaluated *in-process* by shen-erl (the
// Shen/BEAM port): the oracle is the real Shen kernel running the spec,
// not a re-implementation. Inputs come from a deterministic boundary pool
// built per Shen type (guard types are sampled through their generated
// constructors, so every sample satisfies its premises), plus optional
// StreamData draws (--stream-data).

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

type deriveOptions struct {
	SpecPath   string
	Func       string
	Namespace  string
	ImplModule string
	ImplFunc   string
	TestModule string
	StreamData bool
	MaxCases   int
	MaxRuns    int
}

func cmdDerive(args []string) int {
	fs := flag.NewFlagSet("shengen-ex derive", flag.ExitOnError)
	o := deriveOptions{}
	fs.StringVar(&o.SpecPath, "spec", "specs/core.shen", "Shen spec holding the define")
	fs.StringVar(&o.Func, "func", "", "Shen define name, e.g. member-of? (required)")
	fs.StringVar(&o.Namespace, "namespace", "", "guard namespace, e.g. MyApp.Shen (required)")
	fs.StringVar(&o.ImplModule, "impl-module", "", "Elixir module of the implementation (required)")
	fs.StringVar(&o.ImplFunc, "impl-func", "", "implementation function name (default: Elixir form of --func)")
	fs.StringVar(&o.TestModule, "test-module", "", "ExUnit module name (default <ImplModule>.<Func>SpecTest)")
	fs.BoolVar(&o.StreamData, "stream-data", false, "also emit a StreamData property (requires :stream_data)")
	fs.IntVar(&o.MaxCases, "max-cases", 500, "cap on boundary-pool cases")
	fs.IntVar(&o.MaxRuns, "max-runs", 100, "StreamData max_runs")
	out := fs.String("out", "", "output .exs file (default stdout)")
	check := fs.Bool("check", false, "exit 1 if --out differs from the generated test")
	fs.Parse(args)
	if o.Func == "" || o.Namespace == "" || o.ImplModule == "" {
		fmt.Fprintln(os.Stderr, "shengen-ex derive: --func, --namespace and --impl-module are required")
		return 2
	}
	if o.ImplFunc == "" {
		o.ImplFunc = fnName(o.Func)
	}
	if o.TestModule == "" {
		o.TestModule = o.ImplModule + "." + toPascalCase(strings.TrimRight(fnName(o.Func), "?!")) + "SpecTest"
	}
	raw, err := os.ReadFile(o.SpecPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shengen-ex derive: %v\n", err)
		return 1
	}
	code, err := generateDeriveTest(string(raw), o)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shengen-ex derive: %v\n", err)
		return 1
	}
	return writeOutputs([]output{{*out, code}}, *check, "shengen-ex derive --func "+o.Func)
}

// literalPools collects string/number/symbol literals from the spec: they
// are the boundaries the definitions branch on.
func literalPools(defines []Define, types []Datatype) (strs, nums, syms []string) {
	seenS, seenN, seenY := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var walk func(n *Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		switch n.Kind {
		case NStr:
			if !seenS[n.Atom] {
				seenS[n.Atom] = true
				strs = append(strs, n.Atom)
			}
		case NAtom:
			switch {
			case isNumber(n.Atom):
				v := normalizeNumber(n.Atom)
				if !seenN[v] {
					seenN[v] = true
					nums = append(nums, v)
				}
			case !isVarAtom(n.Atom) && n.Atom != "_" && n.Atom != "true" && n.Atom != "false" && !isShenKeyword(n.Atom):
				if !seenY[n.Atom] {
					seenY[n.Atom] = true
					syms = append(syms, n.Atom)
				}
			}
		case NList, NCons:
			if n.Kind == NList && len(n.Items) > 0 {
				for _, it := range n.Items[1:] {
					walk(it)
				}
			} else {
				for _, it := range n.Items {
					walk(it)
				}
			}
			walk(n.Tail)
		}
	}
	for _, d := range defines {
		for _, c := range d.Clauses {
			for _, p := range c.Patterns {
				walk(p)
			}
			walk(c.Result)
			walk(c.Guard)
		}
	}
	for _, dt := range types {
		for _, r := range dt.Rules {
			for _, v := range r.Verified {
				if n, err := readShen(v.Raw); err == nil {
					walk(n)
				}
			}
		}
	}
	sort.Strings(strs)
	sort.Strings(nums)
	sort.Strings(syms)
	return
}

func isShenKeyword(s string) bool {
	switch s {
	case "->", "<-", "where", "-->", "|":
		return true
	}
	return false
}

type poolGen struct {
	st      *SymbolTable
	o       deriveOptions
	order   []string
	defs    map[string]string // type -> pool function body
	gens    map[string]string // type -> StreamData generator body
	strs    []string
	nums    []string
	syms    []string
	pending map[string]bool
}

func poolFn(t string) string { return "pool_" + typeKey(t) }
func genFn(t string) string  { return "gen_" + typeKey(t) }

func typeKey(t string) string {
	if elem := listElemType(t); elem != "" {
		return "list_of_" + typeKey(elem)
	}
	return strings.ReplaceAll(toSnake(t), "__", "_")
}

// need registers a pool (and generator) for Shen type t and its parts.
func (g *poolGen) need(t string) error {
	if _, ok := g.defs[t]; ok || g.pending[t] {
		return nil
	}
	g.pending[t] = true
	defer delete(g.pending, t)
	ns := g.o.Namespace
	var pool, gen string
	switch t {
	case "string":
		vals := append([]string{"", "a", "b", "tenant-1", "tenant-2", "Tenant-1", " ", "ünïcødé"}, g.strs...)
		pool = "[" + joinMap(vals, elixirLiteralString) + "]"
		gen = "StreamData.one_of([StreamData.member_of(pool_string()), StreamData.string(:printable, max_length: 8)])"
	case "number":
		vals := append([]string{"0", "1", "-1", "2", "0.5", "-0.5", "100", "1.0e9"}, g.nums...)
		pool = "[" + joinMap(vals, func(s string) string { return s }) + "]"
		gen = "StreamData.one_of([StreamData.member_of(pool_number()), StreamData.integer(), StreamData.float()])"
	case "boolean":
		pool = "[true, false]"
		gen = "StreamData.boolean()"
	case "symbol":
		vals := append([]string{"a", "b"}, g.syms...)
		pool = "[" + joinMap(vals, elixirAtom) + "]"
		gen = "StreamData.member_of(pool_symbol())"
	default:
		if elem := listElemType(t); elem != "" {
			if err := g.need(elem); err != nil {
				return err
			}
			p := poolFn(elem)
			pool = fmt.Sprintf("xs = %s()\n    picks = sample(xs, 8)\n\n    Enum.uniq(\n      [[] | Enum.map(picks, &[&1])] ++\n        for(x <- sample(xs, 4), y <- sample(xs, 4), do: [x, y]) ++\n        Enum.zip_with(xs, Enum.drop(xs, 1), &[&1, &2]) ++ [picks]\n    )", p)
			gen = fmt.Sprintf("StreamData.list_of(%s(), max_length: 5)", genFn(elem))
			break
		}
		info := g.st.Lookup(t)
		if info == nil {
			return fmt.Errorf("cannot sample Shen type %q (not a primitive, list, or spec datatype)", t)
		}
		if info.Category == "sumtype" {
			var parts []string
			for _, v := range g.st.SumTypes[t] {
				if err := g.need(v); err != nil {
					return err
				}
				parts = append(parts, fmt.Sprintf("sample(%s(), 8)", poolFn(v)))
			}
			pool = strings.Join(parts, " ++\n      ")
			gen = fmt.Sprintf("StreamData.member_of(%s())", poolFn(t))
			break
		}
		fs := fieldsOf(info)
		take := 4
		if len(fs) > 3 {
			take = 2
		}
		var gens, vars []string
		for i, f := range fs {
			if f.shenType == "" {
				return fmt.Errorf("cannot sample field %s of %s (unknown type)", f.shenVar, t)
			}
			if err := g.need(f.shenType); err != nil {
				return err
			}
			v := fmt.Sprintf("f%d", i+1)
			vars = append(vars, v)
			if len(fs) == 1 {
				gens = append(gens, fmt.Sprintf("%s <- %s()", v, poolFn(f.shenType)))
			} else {
				gens = append(gens, fmt.Sprintf("%s <- sample(%s(), %d)", v, poolFn(f.shenType), take))
			}
		}
		mod := g.st.moduleFor(t)
		pool = fmt.Sprintf("for %s,\n        {:ok, value} <- [%s.new(%s)],\n        do: value",
			strings.Join(gens, ",\n        "), mod, strings.Join(vars, ", "))
		if len(fs) == 1 && isPrimitive(fs[0].shenType) {
			gen = fmt.Sprintf("StreamData.one_of([StreamData.member_of(%s()), StreamData.map(%s(), &%s.new/1)])\n    |> StreamData.map(fn\n      {:ok, value} -> value\n      {:error, _} -> nil\n      value -> value\n    end)\n    |> StreamData.filter(&(&1 != nil))",
				poolFn(t), genFn(fs[0].shenType), mod)
		} else {
			gen = fmt.Sprintf("StreamData.member_of(%s())", poolFn(t))
		}
		_ = ns
	}
	g.defs[t] = pool
	g.gens[t] = gen
	g.order = append(g.order, t)
	return nil
}

func joinMap(xs []string, f func(string) string) string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, f(x))
	}
	return strings.Join(out, ", ")
}

func generateDeriveTest(spec string, o deriveOptions) (string, error) {
	types, defines, err := parseSpecContent(spec)
	if err != nil {
		return "", err
	}
	st := newSymbolTable(o.Namespace)
	st.Build(types)
	var def *Define
	for i := range defines {
		st.Defines[defines[i].Name] = &defines[i]
		if defines[i].Name == o.Func {
			def = &defines[i]
		}
	}
	if def == nil {
		return "", fmt.Errorf("define %q not found in %s", o.Func, o.SpecPath)
	}
	if def.RetType == "" {
		return "", fmt.Errorf("define %q has no {type signature}; derive needs one to sample inputs", o.Func)
	}
	arity := len(def.ParamType)
	if len(def.Clauses) > 0 && len(def.Clauses[0].Patterns) != arity {
		return "", fmt.Errorf("define %q: signature has %d parameter(s) but clauses take %d", o.Func, arity, len(def.Clauses[0].Patterns))
	}
	strs, nums, syms := literalPools(defines, types)
	g := &poolGen{st: st, o: o, defs: map[string]string{}, gens: map[string]string{}, pending: map[string]bool{},
		strs: strs, nums: nums, syms: syms}
	for _, t := range def.ParamType {
		if err := g.need(t); err != nil {
			return "", fmt.Errorf("define %q: %w", o.Func, err)
		}
	}

	ns := o.Namespace
	impl := o.ImplModule + "." + o.ImplFunc
	argv := make([]string, arity)
	for i := range argv {
		argv[i] = fmt.Sprintf("a%d", i+1)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Code generated by shengen-ex derive from %s (define %s). DO NOT EDIT.\n", o.SpecPath, o.Func)
	fmt.Fprintf(&b, `#
# Spec-equivalence test: %s/%d must agree with the Shen
# definition of %s evaluated in-process by shen-erl (the Shen kernel
# on the BEAM) on a deterministic boundary pool%s.
#
# Oracle location: $SHEN_ERL_EBIN, else $SHEN_ERL_ROOT/ebin, else a
# sibling shen-erl checkout. Signature: {%s}
`, impl, arity, o.Func, map[bool]string{true: " plus StreamData draws", false: ""}[o.StreamData],
		strings.Join(append(append([]string{}, def.ParamType...), def.RetType), " --> "))
	fmt.Fprintf(&b, "defmodule %s do\n  use ExUnit.Case, async: false\n", o.TestModule)
	if o.StreamData {
		b.WriteString("  use ExUnitProperties\n")
	}
	b.WriteString("\n  # shen-erl is put on the code path at runtime (see shen_erl_ebin!/0).\n  @compile {:no_warn_undefined, [:shen_erl_global_stores, :shen_erl_kl_primitives, :shen_erl_kl_compiler, :kl_sys, :kl_load]}\n")
	fmt.Fprintf(&b, "\n  @moduletag :shen_derive\n  @spec_path %s\n  @shen_fun %s\n  @max_cases %d\n\n",
		elixirLiteralString(o.SpecPath), elixirAtom(o.Func), o.MaxCases)
	b.WriteString("  setup_all do\n    {:ok, oracle: start_oracle!()}\n  end\n\n")

	fmt.Fprintf(&b, "  test %s, %%{oracle: oracle} do\n", elixirLiteralString(fmt.Sprintf("%s agrees with %s/%d on the boundary pool", o.Func, impl, arity)))
	b.WriteString("    cases = cases()\n    assert cases != [], \"empty sample pool\"\n\n    failures =\n      cases\n      |> Enum.with_index()\n")
	fmt.Fprintf(&b, "      |> Enum.flat_map(fn {[%s] = args, index} ->\n", strings.Join(argv, ", "))
	b.WriteString("        spec = oracle_call(oracle, args)\n")
	fmt.Fprintf(&b, "        impl = %s.Term.to_shen(%s(%s))\n", ns, impl, strings.Join(argv, ", "))
	b.WriteString("        if spec == impl, do: [], else: [%{case: index, input: args, spec: spec, impl: impl}]\n      end)\n\n")
	b.WriteString("    assert failures == [],\n           \"#{length(failures)} of #{length(cases)} cases disagree with the spec:\\n\" <>\n             inspect(Enum.take(failures, 5), pretty: true)\n  end\n\n")

	if o.StreamData {
		fmt.Fprintf(&b, "  property %s, %%{oracle: oracle} do\n", elixirLiteralString(fmt.Sprintf("%s agrees with %s/%d on StreamData draws", o.Func, impl, arity)))
		gens := make([]string, arity)
		for i, t := range def.ParamType {
			gens[i] = fmt.Sprintf("%s <- %s()", argv[i], genFn(t))
		}
		fmt.Fprintf(&b, "    check all %s, max_runs: %d do\n", strings.Join(gens, ", "), o.MaxRuns)
		fmt.Fprintf(&b, "      assert oracle_call(oracle, [%s]) == %s.Term.to_shen(%s(%s))\n    end\n  end\n\n", strings.Join(argv, ", "), ns, impl, strings.Join(argv, ", "))
	}

	// cases
	pools := make([]string, arity)
	for i, t := range def.ParamType {
		pools[i] = poolFn(t) + "()"
	}
	b.WriteString("  defp cases do\n")
	fmt.Fprintf(&b, "    product = cartesian([%s])\n", strings.Join(pools, ", "))
	b.WriteString("    stride = max(div(length(product), @max_cases), 1)\n    product |> Enum.take_every(stride) |> Enum.take(@max_cases)\n  end\n\n")
	b.WriteString("  defp sample(xs, k) when length(xs) <= k, do: xs\n  defp sample(xs, k), do: xs |> Enum.take_every(div(length(xs), k)) |> Enum.take(k)\n\n")
	b.WriteString("  defp cartesian([]), do: [[]]\n  defp cartesian([pool | rest]), do: for(x <- pool, tail <- cartesian(rest), do: [x | tail])\n\n")

	b.WriteString("  # Boundary pools, one per Shen type in the signature (guard types are\n  # built through their constructors, so every sample satisfies its premises).\n")
	for _, t := range g.order {
		fmt.Fprintf(&b, "  defp %s do\n    %s\n  end\n\n", poolFn(t), g.defs[t])
	}
	if o.StreamData {
		for _, t := range g.order {
			fmt.Fprintf(&b, "  defp %s do\n    %s\n  end\n\n", genFn(t), g.gens[t])
		}
	}
	b.WriteString(oracleCode)
	b.WriteString("end\n")
	return b.String(), nil
}

// oracleCode is the shen-erl harness shared by every generated test. The
// Shen VM keeps global ETS state owned by one process, so a single named
// oracle process serves all derive tests in a `mix test` run.
const oracleCode = `  # --- shen-erl oracle -------------------------------------------------

  defp oracle_call(oracle, args) do
    ref = make_ref()
    send(oracle, {:call, self(), ref, @shen_fun, args})

    receive do
      {^ref, {:ok, value}} -> value
      {^ref, {:error, reason}} -> {:shen_error, reason}
    after
      30_000 -> flunk("shen-erl oracle timed out on #{inspect(args)}")
    end
  end

  defp start_oracle! do
    pid =
      case Process.whereis(:shen_derive_oracle) do
        nil -> spawn_oracle!()
        pid -> pid
      end

    ref = make_ref()
    send(pid, {:load, self(), ref, @spec_path})

    receive do
      {^ref, :ok} -> pid
      {^ref, {:error, reason}} -> flunk("shen-erl could not load #{@spec_path}: #{inspect(reason)}")
    after
      120_000 -> flunk("shen-erl timed out loading #{@spec_path}")
    end
  end

  defp spawn_oracle! do
    Code.prepend_path(shen_erl_ebin!())
    parent = self()

    pid =
      spawn(fn ->
        Process.register(self(), :shen_derive_oracle)
        :shen_erl_global_stores.init()
        {:ok, sink} = StringIO.open("")
        :shen_erl_kl_primitives.set(:"*stoutput*", sink)
        :shen_erl_kl_primitives.set(:"*stinput*", :standard_io)
        :shen_erl_kl_compiler.boot()
        home = String.to_charlist(File.cwd!()) ++ ~c"/"
        :shen_erl_kl_primitives.set(:"*home-directory*", {:string, home})
        send(parent, {:shen_oracle_booted, self()})
        oracle_loop(MapSet.new())
      end)

    receive do
      {:shen_oracle_booted, ^pid} -> pid
    after
      120_000 -> flunk("shen-erl failed to boot")
    end
  end

  defp oracle_loop(loaded) do
    receive do
      {:load, from, ref, path} ->
        if MapSet.member?(loaded, path) do
          send(from, {ref, :ok})
          oracle_loop(loaded)
        else
          result =
            try do
              :kl_sys.tc(:-)
              :kl_load.load({:string, String.to_charlist(path)})
              :ok
            catch
              kind, reason -> {:error, {kind, reason}}
            end

          send(from, {ref, result})
          oracle_loop(if(result == :ok, do: MapSet.put(loaded, path), else: loaded))
        end

      {:call, from, ref, fun, args} ->
        reply =
          try do
            {:ok, {mod, name, _arity}} = :shen_erl_global_stores.get_mfa(fun)
            {:ok, from_kl(apply(mod, name, Enum.map(args, &to_kl/1)))}
          catch
            kind, reason -> {:error, {kind, reason}}
          end

        send(from, {ref, reply})
        oracle_loop(loaded)
    end
  end

  defp to_kl(value) when is_binary(value), do: {:string, String.to_charlist(value)}
  defp to_kl(value) when is_list(value), do: Enum.map(value, &to_kl/1)
  defp to_kl(value), do: value

  defp from_kl({:string, chars}), do: List.to_string(chars)
  defp from_kl(value) when is_list(value), do: Enum.map(value, &from_kl/1)
  defp from_kl(value), do: value

  defp shen_erl_ebin! do
    candidates =
      [
        System.get_env("SHEN_ERL_EBIN"),
        System.get_env("SHEN_ERL_ROOT") && Path.join(System.get_env("SHEN_ERL_ROOT"), "ebin"),
        Path.expand("../shen-erl/ebin"),
        Path.expand("../../../shen-erl/ebin")
      ]
      |> Enum.reject(&is_nil/1)

    Enum.find(candidates, &File.exists?(Path.join(&1, "shen_erl_kl_compiler.beam"))) ||
      flunk("shen-erl not found; set SHEN_ERL_EBIN (tried #{inspect(candidates)})")
  end
`
