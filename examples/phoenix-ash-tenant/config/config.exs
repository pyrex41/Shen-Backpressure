import Config

config :tenant, ash_domains: [Tenant.Docs]

config :ash,
  disable_async?: true,
  default_string_length_count: :codepoints

config :logger, level: :warning
