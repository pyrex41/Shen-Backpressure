defmodule Tenant.Authn do
  @moduledoc """
  Check wrappers that turn raw credentials into Shen proof values.

  These functions are the trusted computing base for the I/O-backed
  premises: HMAC verification for `verified-token`, the directory lookup
  for `tenant-access`. Everything downstream only accepts the guard
  structs, whose constructors re-check every spec premise.
  """

  alias Tenant.Shen.{
    AuthenticatedUser,
    HumanPrincipal,
    OperatorCredential,
    OperatorId,
    OperatorPrincipal,
    UserId,
    VerifiedToken
  }

  @doc "Issues a `user.signature` token (test helper for the demo)."
  @spec issue_token(String.t(), binary()) :: String.t()
  def issue_token(user, secret), do: user <> "." <> sign(user, secret)

  @doc """
  Verifies a `user.signature` token and returns the human principal it
  authenticates. The user id is threaded from the token subject, which is
  exactly what the `(= User (head Token))` premise demands.
  """
  @spec authenticate(String.t(), binary()) :: {:ok, HumanPrincipal.t()} | {:error, term()}
  def authenticate(token, secret) do
    with [user, sig] <- String.split(token, ".", parts: 2),
         true <- secure_equal?(sign(user, secret), sig) || {:error, :bad_signature},
         {:ok, sub} <- UserId.new(user),
         {:ok, verified} <- VerifiedToken.new(sub, sig),
         {:ok, auth} <- AuthenticatedUser.new(verified, sub) do
      HumanPrincipal.new(auth)
    else
      {:error, _} = error -> error
      _ -> {:error, :malformed_token}
    end
  end

  @doc "Authenticates an operator (batch job / support) by shared secret."
  @spec operator(String.t(), String.t()) :: {:ok, OperatorPrincipal.t()} | {:error, term()}
  def operator(operator, secret) do
    expected = Tenant.Directory.operator_secret(operator)

    with true <-
           (is_binary(expected) and secure_equal?(expected, secret)) || {:error, :bad_secret},
         {:ok, id} <- OperatorId.new(operator),
         {:ok, cred} <- OperatorCredential.new(id, secret) do
      OperatorPrincipal.new(cred)
    end
  end

  defp secure_equal?(a, b), do: byte_size(a) == byte_size(b) and :crypto.hash_equals(a, b)

  defp sign(user, secret) do
    :crypto.mac(:hmac, :sha256, secret, user) |> Base.url_encode64(padding: false)
  end
end
