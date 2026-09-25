defmodule Tenant.Docs.Document do
  @moduledoc """
  A tenant-scoped document. Policies are the generated Shen checks: the
  actor must be the proof struct itself, issued for the request tenant.
  """
  use Ash.Resource,
    domain: Tenant.Docs,
    data_layer: Ash.DataLayer.Ets,
    authorizers: [Ash.Policy.Authorizer]

  multitenancy do
    strategy :attribute
    attribute :tenant_id
  end

  attributes do
    attribute :id, :string, primary_key?: true, allow_nil?: false, public?: true
    attribute :tenant_id, :string, allow_nil?: false, public?: true
    attribute :title, :string, allow_nil?: false, public?: true
  end

  actions do
    defaults [:read, :destroy]

    create :create do
      accept [:id, :title]
    end

    update :update do
      accept [:title]
      # Authorize against the loaded record (FilterCheck on the row id).
      require_atomic? false
    end
  end

  policies do
    policy action_type([:read, :create]) do
      authorize_if Tenant.Shen.Policy.TenantAccessCheck
    end

    policy action_type([:update, :destroy]) do
      authorize_if {Tenant.Shen.Policy.WriteAccessFilter, attribute: :id}
    end
  end
end
