defmodule Tenant.Docs do
  @moduledoc "Ash domain for tenant-scoped documents."
  use Ash.Domain, validate_config_inclusion?: false

  resources do
    resource Tenant.Docs.Document
  end
end
