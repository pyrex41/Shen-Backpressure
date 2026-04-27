#!/usr/bin/env -S npx tsx
// shen-derive-ts — Verification gate for Shen specs (TypeScript port).
//
// Given one or more .shen spec files containing a (define ...) block, generate a
// TypeScript test file that asserts a hand-written implementation matches
// the spec pointwise on sampled inputs.
//
// See thoughts/shared/handoffs/general/2026-04-10_17-04-28_shen-derive-ts-port-prompt.md
// for the full design.

import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

import { parseFile, findDefine } from "./specfile/parse.ts";
import { buildTypeTable } from "./specfile/typetable.ts";
import {
  buildHarness,
  emit,
  type ExternBinding,
  type HarnessConfig,
} from "./verify/harness.ts";
import type { Value } from "./core/eval.ts";

const VERSION = "0.1.0";

function usage(): void {
  process.stderr.write(
    `shen-derive-ts — Verification gate for Shen specs (v${VERSION})

Usage: shen-derive-ts verify <spec.shen> [more-specs...] [flags]

Flags:
  --spec FILE                 add a spec file (may be repeated)
  --func NAME                 (required) which (define ...) block to verify
  --impl-module PATH          (required) relative TS import path of the impl, e.g. ./processable
  --impl-func NAME            (required) exported TS function name
  --import PATH               (required) import path of the shengen-ts guards module, e.g. ./guards_gen
  --import-alias ALIAS        default: "shenguard"
  --extern NAME=MOD::EXP/N    bind external function NAME to module export EXP with arity N (repeatable)
  --out FILE                  default: stdout
  --max-cases N               default: 50
  --seed N                    RNG seed (0 = deterministic boundary values only)
  --random-draws N            number of random primitive draws per type when --seed != 0

Example:
  npx tsx cmd/shen-derive-ts/shen-derive.ts verify specs/core.shen \\
    --func processable \\
    --impl-module ./runtime/processable \\
    --impl-func Processable \\
    --import ./runtime/guards_gen \\
    --out runtime/processable.shen-derive.test.ts
`,
  );
}

type Flags = {
  specPaths: string[];
  func: string;
  implModule: string;
  implFunc: string;
  importPath: string;
  importAlias: string;
  externSpecs: ExternSpec[];
  out: string;
  maxCases: number;
  seed: number;
  randomDraws: number;
};

type ExternSpec = {
  name: string;
  modulePath: string;
  exportName: string;
  arity: number | null;
};

function parseFlags(args: string[]): Flags {
  const f: Flags = {
    specPaths: [],
    func: "",
    implModule: "",
    implFunc: "",
    importPath: "",
    importAlias: "shenguard",
    externSpecs: [],
    out: "",
    maxCases: 50,
    seed: 0,
    randomDraws: 0,
  };

  for (let i = 0; i < args.length; i++) {
    const a = args[i];
    const next = () => {
      const v = args[++i];
      if (v === undefined) {
        process.stderr.write(`error: flag ${a} requires a value\n`);
        process.exit(1);
      }
      return v;
    };
    switch (a) {
      case "--spec":
        f.specPaths.push(next());
        break;
      case "--func":
        f.func = next();
        break;
      case "--impl-module":
        f.implModule = next();
        break;
      case "--impl-func":
        f.implFunc = next();
        break;
      case "--import":
        f.importPath = next();
        break;
      case "--import-alias":
        f.importAlias = next();
        break;
      case "--extern":
        f.externSpecs.push(parseExternSpec(next()));
        break;
      case "--out":
        f.out = next();
        break;
      case "--max-cases":
        f.maxCases = Number(next());
        break;
      case "--seed":
        f.seed = Number(next());
        break;
      case "--random-draws":
        f.randomDraws = Number(next());
        break;
      default:
        if (a.startsWith("-")) {
          process.stderr.write(`error: unknown flag ${a}\n`);
          process.exit(1);
        }
        f.specPaths.push(a);
    }
  }
  return f;
}

function parseExternSpec(raw: string): ExternSpec {
  const eq = raw.indexOf("=");
  if (eq <= 0) {
    throw new Error(`bad --extern ${JSON.stringify(raw)}: expected name=module::export/arity`);
  }
  const name = raw.slice(0, eq).trim();
  const rhs = raw.slice(eq + 1).trim();
  const sep = rhs.lastIndexOf("::");
  if (sep <= 0 || sep + 2 >= rhs.length) {
    throw new Error(`bad --extern ${JSON.stringify(raw)}: expected name=module::export/arity`);
  }
  const modulePath = rhs.slice(0, sep);
  let exportName = rhs.slice(sep + 2);
  let arity: number | null = null;
  const slash = exportName.lastIndexOf("/");
  if (slash !== -1) {
    const rawArity = exportName.slice(slash + 1);
    exportName = exportName.slice(0, slash);
    arity = Number(rawArity);
    if (!Number.isInteger(arity) || arity <= 0) {
      throw new Error(`bad --extern ${JSON.stringify(raw)}: arity must be a positive integer`);
    }
  }
  if (name === "" || modulePath === "" || exportName === "") {
    throw new Error(`bad --extern ${JSON.stringify(raw)}: empty name, module, or export`);
  }
  return { name, modulePath, exportName, arity };
}

async function loadExterns(specs: ExternSpec[]): Promise<ExternBinding[]> {
  const out: ExternBinding[] = [];
  for (const spec of specs) {
    const mod = await import(resolveModuleSpecifier(spec.modulePath));
    const adapter = (mod as Record<string, unknown>)[spec.exportName];
    if (typeof adapter !== "function") {
      throw new Error(
        `extern ${spec.name}: ${spec.modulePath}::${spec.exportName} is not a function`,
      );
    }
    const adapterFn = adapter as ((...args: Value[]) => unknown) & {
      arity?: unknown;
    };
    const exposedArity = typeof adapterFn.arity === "number"
      ? adapterFn.arity
      : adapter.length;
    const arity = spec.arity ?? exposedArity;
    if (!Number.isInteger(arity) || arity <= 0) {
      throw new Error(
        `extern ${spec.name}: provide /arity or set a positive numeric arity property`,
      );
    }
    out.push({
      name: spec.name,
      arity,
      fn: (...args: Value[]) => {
        const value = adapterFn(...args);
        if (isPromiseLike(value)) {
          throw new Error(`extern ${spec.name}: async adapters are not supported yet`);
        }
        return value as Value;
      },
    });
  }
  return out;
}

function resolveModuleSpecifier(modulePath: string): string {
  if (modulePath.startsWith(".") || modulePath.startsWith("/")) {
    return pathToFileURL(resolve(process.cwd(), modulePath)).href;
  }
  return modulePath;
}

function isPromiseLike(value: unknown): boolean {
  return typeof value === "object" && value !== null && "then" in value;
}

async function cmdVerify(args: string[]): Promise<void> {
  if (args.length === 0) {
    usage();
    process.exit(1);
  }
  let flags: Flags;
  try {
    flags = parseFlags(args);
  } catch (e) {
    process.stderr.write(`${(e as Error).message}\n`);
    process.exit(1);
  }

  const missing: string[] = [];
  if (flags.specPaths.length === 0) missing.push("<spec.shen>");
  if (!flags.func) missing.push("--func");
  if (!flags.implModule) missing.push("--impl-module");
  if (!flags.implFunc) missing.push("--impl-func");
  if (!flags.importPath) missing.push("--import");
  if (missing.length > 0) {
    process.stderr.write(`error: missing required flags: ${missing.join(", ")}\n`);
    usage();
    process.exit(1);
  }

  const sf = { datatypes: [], defines: [] } as ReturnType<typeof parseFile>;
  for (const specPath of flags.specPaths) {
    let src: string;
    try {
      src = readFileSync(specPath, "utf8");
    } catch (e) {
      process.stderr.write(`read ${specPath}: ${(e as Error).message}\n`);
      process.exit(1);
    }

    try {
      const parsed = parseFile(src);
      sf.datatypes.push(...parsed.datatypes);
      sf.defines.push(...parsed.defines);
    } catch (e) {
      process.stderr.write(`parse ${specPath}: ${(e as Error).message}\n`);
      process.exit(1);
    }
  }

  if (!findDefine(sf, flags.func)) {
    process.stderr.write(
      `define ${JSON.stringify(flags.func)} not found in ${flags.specPaths.join(", ")}\n`,
    );
    process.exit(1);
  }

  const tt = buildTypeTable(sf.datatypes, flags.importPath, flags.importAlias);
  let externs: ExternBinding[];
  try {
    externs = await loadExterns(flags.externSpecs);
  } catch (e) {
    process.stderr.write(`load externs: ${(e as Error).message}\n`);
    process.exit(1);
  }

  const cfg: HarnessConfig = {
    spec: sf,
    tt,
    allDefines: sf.defines,
    externs,
    funcName: flags.func,
    implModule: flags.implModule,
    implFunc: flags.implFunc,
    importPath: flags.importPath,
    importAlias: flags.importAlias,
    maxCases: flags.maxCases,
    seed: flags.seed,
    randomDraws: flags.randomDraws,
  };

  let harness;
  try {
    harness = buildHarness(cfg);
  } catch (e) {
    process.stderr.write(`build harness: ${(e as Error).message}\n`);
    process.exit(1);
  }

  const source = emit(harness);

  if (flags.out === "") {
    process.stdout.write(source);
    return;
  }
  try {
    writeFileSync(flags.out, source);
  } catch (e) {
    process.stderr.write(`write ${flags.out}: ${(e as Error).message}\n`);
    process.exit(1);
  }
  process.stderr.write(`wrote ${flags.out} (${harness.cases.length} cases)\n`);
}

async function main(): Promise<void> {
  const args = process.argv.slice(2);
  if (args.length === 0) {
    usage();
    process.exit(1);
  }
  switch (args[0]) {
    case "verify":
      await cmdVerify(args.slice(1));
      return;
    case "version":
    case "--version":
    case "-v":
      process.stdout.write(`shen-derive-ts ${VERSION}\n`);
      return;
    case "help":
    case "--help":
    case "-h":
      usage();
      return;
    default:
      process.stderr.write(`error: unknown command ${args[0]}\n`);
      usage();
      process.exit(1);
  }
}

main().catch((e) => {
  process.stderr.write(`${(e as Error).message}\n`);
  process.exit(1);
});
