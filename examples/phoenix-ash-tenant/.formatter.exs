[
  import_deps: [:ash],
  # Generated files (lib/tenant/shen/*_gen.ex, shen/, test/derived/) are
  # owned by shengen-ex and drift-checked byte for byte; do not format them.
  inputs: [
    "{mix,.formatter}.exs",
    "config/*.exs",
    "lib/tenant/*.ex",
    "lib/tenant/docs/*.ex",
    "test/*.exs"
  ]
]
