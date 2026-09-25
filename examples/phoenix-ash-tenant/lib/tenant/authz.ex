defmodule Tenant.Authz do
  @moduledoc """
  Builds the authorization proofs (tenant-access → resource-access →
  write-access) and hosts the pure functions pinned against the Shen spec
  by `sb derive` (`member_of?/2`, `visible_titles/2`).
  """

  alias Tenant.Shen.{
    AuthenticatedPrincipal,
    HumanPrincipal,
    OperatorPrincipal,
    ResourceAccess,
    ResourceId,
    Role,
    TenantAccess,
    TenantId,
    WriteAccess
  }

  @doc "Proof that `principal` may act inside `tenant` (directory lookup is TCB)."
  @spec tenant_access(AuthenticatedPrincipal.t(), String.t()) ::
          {:ok, TenantAccess.t()} | {:error, term()}
  def tenant_access(principal, tenant) do
    with {:ok, tenant_id} <- TenantId.new(tenant) do
      TenantAccess.new(principal, tenant_id, member?(principal, tenant))
    end
  end

  @doc """
  Proof that the tenant-access holder may touch the document. The owner is
  the document's own tenant, so a document from another tenant fails the
  `(= Owner (head (tail Access)))` premise.
  """
  @spec resource_access(TenantAccess.t(), %{id: String.t(), tenant_id: String.t()}) ::
          {:ok, ResourceAccess.t()} | {:error, term()}
  def resource_access(access, %{id: id, tenant_id: owner}) do
    with {:ok, resource} <- ResourceId.new(id),
         {:ok, owner} <- TenantId.new(owner) do
      ResourceAccess.new(access, resource, owner)
    end
  end

  @doc "Proof that the holder may write the document (role from the directory)."
  @spec write_access(ResourceAccess.t()) :: {:ok, WriteAccess.t()} | {:error, term()}
  def write_access(resource_access) do
    access = ResourceAccess.access(resource_access)
    tenant = access |> TenantAccess.tenant() |> TenantId.val()

    with {:ok, role} <- Role.new(role_in(TenantAccess.principal(access), tenant) || "none") do
      WriteAccess.new(resource_access, role)
    end
  end

  defp member?(principal, tenant), do: role_in(principal, tenant) != nil

  defp role_in(principal, tenant) do
    # is_struct/2 rather than a %HumanPrincipal{} pattern: matching on the
    # struct outside its module breaks @opaque (Dialyzer: call_without_opaque).
    cond do
      is_struct(principal, HumanPrincipal) ->
        user =
          principal
          |> HumanPrincipal.auth()
          |> Tenant.Shen.AuthenticatedUser.user()
          |> Tenant.Shen.UserId.val()

        Tenant.Directory.role(user, tenant)

      is_struct(principal, OperatorPrincipal) ->
        "admin"
    end
  end

  # --- pure functions pinned by shen-derive --------------------------------

  @doc "Shen `member-of?`: is `x` an element of `xs`?"
  @spec member_of?(String.t(), [String.t()]) :: boolean()
  def member_of?(x, xs), do: x in xs

  @doc "Shen `visible-titles`: titles of the `[tenant, title]` rows owned by `tenant`, in order."
  @spec visible_titles(String.t(), [[String.t()]]) :: [String.t()]
  def visible_titles(tenant, docs) do
    for [^tenant, title] <- docs, do: title
  end
end
