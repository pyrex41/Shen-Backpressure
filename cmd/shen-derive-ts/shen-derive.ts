#!/usr/bin/env -S npx tsx
// shen-derive-ts — Verification gate for Shen specs (TypeScript port).
//
// Given a .shen spec file containing a (define ...) block, generate a
// TypeScript test file that asserts a hand-written implementation matches
// the spec pointwise on sampled inputs.
//
// See thoughts/shared/handoffs/general/2026-04-10_17-04-28_shen-derive-ts-port-prompt.md
// for the full design.

import { readFileSync, writeFileSync } from "node:fs";

import { parseFile, findDefine } from "./specfile/parse.ts";
import { buildTypeTable } from "./specfile/typetable.ts";
import { buildHarness, emit, type HarnessConfig } from "./verify/harness.ts";

const VERSION = "0.1.0";

function usage(): void {
  process.stderr.write(
    `shen-derive-ts — Verification gate for Shen specs (v${VERSION})

Usage: shen-derive-ts verify <spec.shen> [flags]

Flags:
  --func NAME                 (required) which (define ...) block to verify
  --impl-module PATH          (required) relative TS import path of the impl, e.g. ./processable
  --impl-func NAME            (required) exported TS function name
  --import PATH               (required) import path of the shengen-ts guards module, e.g. ./guards_gen
  --import-alias ALIAS        default: "shenguard"
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
  func: string;
  implModule: string;
  implFunc: string;
  importPath: string;
  importAlias: string;
  out: string;
  maxCases: number;
  seed: number;
  randomDraws: number;
};

function parseFlags(args: string[]): Flags {
  const f: Flags = {
    func: "",
    implModule: "",
    implFunc: "",
    importPath: "",
    importAlias: "shenguard",
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
        process.stderr.write(`error: unknown flag ${a}\n`);
        process.exit(1);
    }
  }
  return f;
}

function cmdVerify(args: string[]): void {
  if (args.length === 0) {
    usage();
    process.exit(1);
  }
  const specPath = args[0];
  const flags = parseFlags(args.slice(1));

  const missing: string[] = [];
  if (!flags.func) missing.push("--func");
  if (!flags.implModule) missing.push("--impl-module");
  if (!flags.implFunc) missing.push("--impl-func");
  if (!flags.importPath) missing.push("--import");
  if (missing.length > 0) {
    process.stderr.write(`error: missing required flags: ${missing.join(", ")}\n`);
    usage();
    process.exit(1);
  }

  let src: string;
  try {
    src = readFileSync(specPath, "utf8");
  } catch (e) {
    process.stderr.write(`read ${specPath}: ${(e as Error).message}\n`);
    process.exit(1);
  }

  let sf;
  try {
    sf = parseFile(src);
  } catch (e) {
    process.stderr.write(`parse spec: ${(e as Error).message}\n`);
    process.exit(1);
  }

  if (!findDefine(sf, flags.func)) {
    process.stderr.write(`define ${JSON.stringify(flags.func)} not found in ${specPath}\n`);
    process.exit(1);
  }

  const tt = buildTypeTable(sf.datatypes, flags.importPath, flags.importAlias);

  const cfg: HarnessConfig = {
    spec: sf,
    tt,
    allDefines: sf.defines,
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

function main(): void {
  const args = process.argv.slice(2);
  if (args.length === 0) {
    usage();
    process.exit(1);
  }
  switch (args[0]) {
    case "verify":
      cmdVerify(args.slice(1));
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

main();
