defmodule Tenant.GuardsTest do
  @moduledoc """
  The generated constructors enforce every premise of the Shen spec,
  including the hostile cases from examples/multi-tenant-api.
  """
  use ExUnit.Case, async: true

  alias Tenant.{Authn, Authz}

  alias Tenant.Shen.{
    AuthenticatedUser,
    GuardError,
    OperatorCredential,
    OperatorId,
    ResourceAccess,
    ResourceId,
    Role,
    TenantAccess,
    TenantId,
    UserId,
    VerifiedToken,
    WriteAccess
  }

  @secret "test-secret"

  defp alice, do: Authn.issue_token("alice", @secret) |> Authn.authenticate(@secret) |> ok!()
  defp ok!({:ok, value}), do: value

  describe "authentication chain" do
    test "a valid token yields a human principal bound to its subject" do
      assert {:ok, principal} = Authn.authenticate(Authn.issue_token("alice", @secret), @secret)
      assert Tenant.Shen.AuthenticatedPrincipal.member?(principal)
    end

    test "a bad signature never reaches the guard types" do
      assert {:error, :bad_signature} = Authn.authenticate("alice.forged", @secret)
    end

    test "empty signatures are rejected by the verified-token premise" do
      assert {:error, {:premise, ~s|(not (= Sig ""))|, [sig: ""]}} =
               VerifiedToken.new(UserId.new!("alice"), "")
    end

    test "hostile: pairing alice's token with bob's user id fails (= User (head Token))" do
      token = VerifiedToken.new!(UserId.new!("alice"), "sig")
      bob = UserId.new!("bob")

      assert {:error, {:premise, "(= User (head Token))", values}} =
               AuthenticatedUser.new(token, bob)

      assert values[:user] == bob
    end

    test "operators need a non-empty secret" do
      assert {:error, {:premise, ~s|(not (= Secret ""))|, _}} =
               OperatorCredential.new(OperatorId.new!("nightly-export"), "")

      assert {:ok, _} = Authn.operator("nightly-export", "s3cr3t")
      assert {:error, :bad_secret} = Authn.operator("nightly-export", "nope")
    end
  end

  describe "authorization chain" do
    test "members get tenant access, non-members get the IsMember premise error" do
      assert {:ok, access} = Authz.tenant_access(alice(), "acme")
      assert access |> TenantAccess.tenant() |> TenantId.val() == "acme"

      assert {:error, {:premise, "(= IsMember true)", [is_member: false]}} =
               Authz.tenant_access(alice(), "initech")
    end

    test "hostile: a resource from another tenant cannot be bound (cross-tenant)" do
      access = ok!(Authz.tenant_access(alice(), "acme"))
      assert {:ok, _} = Authz.resource_access(access, %{id: "doc-1", tenant_id: "acme"})

      assert {:error, {:premise, "(= Owner (head (tail Access)))", values}} =
               Authz.resource_access(access, %{id: "doc-9", tenant_id: "globex"})

      assert values[:owner] == TenantId.new!("globex")
    end

    test "write access needs a role the spec's can-write? accepts" do
      editor = ok!(Authz.tenant_access(alice(), "acme"))
      viewer = ok!(Authz.tenant_access(alice(), "globex"))

      assert {:ok, %WriteAccess{}} =
               editor
               |> Authz.resource_access(%{id: "d", tenant_id: "acme"})
               |> ok!()
               |> Authz.write_access()

      assert {:error, {:premise, "(can-write? Role)", [role: role]}} =
               viewer
               |> Authz.resource_access(%{id: "d", tenant_id: "globex"})
               |> ok!()
               |> Authz.write_access()

      assert Role.val(role) == "viewer"
    end

    test "role is constrained to the spec's element? set" do
      assert {:error, {:premise, ~s|(element? X ["viewer" "editor" "admin"])|, [x: "root"]}} =
               Role.new("root")
    end

    test "constructors reject values of the wrong shape" do
      assert {:error, {:type, "Access : tenant-access", _}} =
               ResourceAccess.new(%{tenant: "acme"}, ResourceId.new!("d"), TenantId.new!("acme"))

      assert {:error, {:type, "IsMember : boolean", _}} =
               TenantAccess.new(alice(), TenantId.new!("acme"), "yes")
    end

    test "new! raises GuardError with the Shen premise" do
      assert_raise GuardError, ~r/\(= IsMember true\)/, fn ->
        TenantAccess.new!(alice(), TenantId.new!("acme"), false)
      end
    end
  end

  test "Term.to_shen gives the Shen representation (wrappers unwrap, composites are lists)" do
    access = ok!(Authz.tenant_access(alice(), "acme"))
    [[[sub, sig], user], tenant, true] = Tenant.Shen.Term.to_shen(access)
    assert {sub, user, tenant} == {"alice", "alice", "acme"}
    assert is_binary(sig)
  end
end
