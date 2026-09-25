defmodule Tenant.Directory do
  @moduledoc """
  Stand-in for the membership / role tables a real app would query.

  This module is TCB: the `IsMember` premise of `tenant-access` is only as
  true as `member?/2` (see AUDIT.md).
  """

  @memberships %{
    "alice" => %{"acme" => "editor", "globex" => "viewer"},
    "bob" => %{"globex" => "admin"}
  }

  @operators %{"nightly-export" => "s3cr3t"}

  @doc "Role of `user` in `tenant`, or nil when not a member."
  @spec role(String.t(), String.t()) :: String.t() | nil
  def role(user, tenant), do: get_in(@memberships, [user, tenant])

  @doc "Operators are members of every tenant (support / batch jobs)."
  @spec operator_secret(String.t()) :: String.t() | nil
  def operator_secret(operator), do: Map.get(@operators, operator)
end
