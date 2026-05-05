import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const CLI = fileURLToPath(new URL("./shen-derive.ts", import.meta.url));

function runCli(cwd: string, args: string[]) {
  return spawnSync(
    process.execPath,
    ["--experimental-strip-types", CLI, ...args],
    { cwd, encoding: "utf8" },
  );
}

test("verify combines --spec and positional spec inputs", () => {
  const dir = mkdtempSync(join(tmpdir(), "shen-derive-multispec-"));
  writeFileSync(
    join(dir, "types.shen"),
    `
(datatype thing
  X : string;
  ==========
  X : thing;)
`,
  );
  writeFileSync(
    join(dir, "roundtrip.shen"),
    `
(define roundtrip?
  {thing --> boolean}
  X -> (= X X))
`,
  );

  const res = runCli(dir, [
    "verify",
    "--spec",
    "types.shen",
    "roundtrip.shen",
    "--func",
    "roundtrip?",
    "--impl-module",
    "./impl",
    "--impl-func",
    "Roundtrip",
    "--import",
    "./guards_gen",
  ]);

  assert.equal(res.status, 0, res.stderr);
  assert.ok(res.stdout.includes(`import { Roundtrip } from "./impl";`));
  assert.ok(res.stdout.includes("shenguard.mustThing("));
  assert.ok(res.stdout.includes("const want = true;"));
});

test("verify binds repeated --extern adapters", () => {
  const dir = mkdtempSync(join(tmpdir(), "shen-derive-extern-"));
  writeFileSync(
    join(dir, "spec.shen"),
    `
(datatype thing
  X : string;
  ==========
  X : thing;)

(define roundtrip?
  {thing --> boolean}
  X -> (= X (decode (encode X))))
`,
  );
  writeFileSync(
    join(dir, "derive-externs.ts"),
    `
export function encode(v) {
  return v;
}

export function decode(v) {
  return v;
}
`,
  );

  const res = runCli(dir, [
    "verify",
    "spec.shen",
    "--func",
    "roundtrip?",
    "--impl-module",
    "./impl",
    "--impl-func",
    "Roundtrip",
    "--import",
    "./guards_gen",
    "--extern",
    "encode=./derive-externs.ts::encode/1",
    "--extern",
    "decode=./derive-externs.ts::decode/1",
  ]);

  assert.equal(res.status, 0, res.stderr);
  assert.ok(res.stdout.includes("Roundtrip("));
  assert.ok(res.stdout.includes("const want = true;"));
});

test("verify accepts Promise-returning --extern adapters", () => {
  const dir = mkdtempSync(join(tmpdir(), "shen-derive-async-extern-"));
  writeFileSync(
    join(dir, "spec.shen"),
    `
(datatype thing
  X : string;
  ==========
  X : thing;)

(define roundtrip?
  {thing --> boolean}
  X -> (= X (decode (encode X))))
`,
  );
  writeFileSync(
    join(dir, "derive-externs.ts"),
    `
export async function encode(v) {
  return v;
}

export async function decode(v) {
  return v;
}
`,
  );

  const res = runCli(dir, [
    "verify",
    "spec.shen",
    "--func",
    "roundtrip?",
    "--impl-module",
    "./impl",
    "--impl-func",
    "Roundtrip",
    "--import",
    "./guards_gen",
    "--extern",
    "encode=./derive-externs.ts::encode/1",
    "--extern",
    "decode=./derive-externs.ts::decode/1",
  ]);

  assert.equal(res.status, 0, res.stderr);
  assert.ok(res.stdout.includes(`test("case_00", async () => {`));
  assert.ok(res.stdout.includes("const got = await Roundtrip("));
  assert.ok(res.stdout.includes("const want = true;"));
});
