defmodule Tenant.TracerTest do
  @moduledoc """
  The generated compile tracer (also active for `mix compile`, see
  mix.exs) refuses hostile Elixir that forges a guard struct.
  """
  use ExUnit.Case, async: false

  setup do
    previous = Code.get_compiler_option(:tracers)
    Code.put_compiler_option(:tracers, [Tenant.Shen.GuardTracer])
    on_exit(fn -> Code.put_compiler_option(:tracers, previous) end)
  end

  defp compile(source) do
    Code.compile_string(source, "hostile.ex")
    :compiled
  rescue
    e in CompileError -> {:refused, Exception.message(e)}
  end

  test "struct literal outside the guard module is refused" do
    assert {:refused, msg} =
             compile(~S"""
             defmodule Hostile.Literal do
               def forge(p, t), do: %Tenant.Shen.TenantAccess{principal: p, tenant: t, is_member: true}
             end
             """)

    assert msg =~ "build it with Tenant.Shen.TenantAccess.new/N"
  end

  test "struct update syntax is refused" do
    assert {:refused, _} =
             compile(~S"""
             defmodule Hostile.Update do
               def retarget(access, t), do: %Tenant.Shen.TenantAccess{access | tenant: t}
             end
             """)
  end

  test "Kernel.struct/2 in a module that names guard types is refused" do
    assert {:refused, msg} =
             compile(~S"""
             defmodule Hostile.StructFun do
               def forge(t), do: struct(Tenant.Shen.ResourceAccess, owner: t)
             end
             """)

    assert msg =~ "Kernel.struct/2 is not allowed"
  end

  test "pattern matching and constructor calls are allowed" do
    assert :compiled =
             compile(~S"""
             defmodule Friendly.Reader do
               def tenant(%Tenant.Shen.TenantAccess{} = access), do: Tenant.Shen.TenantAccess.tenant(access)
               def build(p, t), do: Tenant.Shen.TenantAccess.new(p, t, true)
             end
             """)
  end

  test "struct/2 and defexception in modules that never touch guards are fine" do
    assert :compiled =
             compile(~S"""
             defmodule Friendly.Plain do
               defexception [:message]
               def mk(mod), do: struct(mod, [])
             end
             """)
  end
end
