// CLI for the multilang-paired demo (TypeScript implementation).
//
// Reads JSONL from stdin, emits JSON-per-line to stdout. Same I/O
// shape as `go/main.go`, `py/main.py`, `rs/main.rs`. Run via
// `npx tsx ts/main.ts < fixture-inputs.jsonl`.
//
// NOTE: hasBulkLine is implemented inline below rather than imported
// from `guards_gen.ts`. The TS emitter now parses the trailing
// `pattern -> result where (guard)` placement (it previously dropped
// the guard, see shengen-ts issue #27), but the inline version is kept
// so the four CLIs share one hand-checked implementation of the
// fixture contract. The parity check is the behavioural contract.

import * as readline from "node:readline";
import {
  CartItem,
  Cart,
  CustomerId,
  Sku,
  DiscountEligible,
} from "./guards_gen.ts";

type ItemInput = { sku: string; qty: number };
type Input = {
  caseId: string;
  customerId: string;
  items: ItemInput[];
  subtotal: number;
  minSubtotal: number;
  minItems: number;
};

type Output = {
  caseId: string;
  ok: boolean;
  outcome: string;
  itemCount: number;
  hasBulkLine: boolean;
};

function hasBulkLine(items: CartItem[]): boolean {
  for (const it of items) {
    if (it.qty() >= 5) return true;
  }
  return false;
}

function processRow(input: Input): Output {
  const out: Output = {
    caseId: input.caseId,
    ok: false,
    outcome: "",
    itemCount: input.items.length,
    hasBulkLine: false,
  };

  // Build CartItems (verified premise: qty >= 1).
  const items: CartItem[] = [];
  for (const it of input.items) {
    const built = CartItem.tryCreate(Sku.createOrThrow(it.sku), it.qty);
    if (built instanceof Error) {
      out.outcome = "rejected-item-build";
      return out;
    }
    items.push(built);
  }

  // Build Cart (verified premise: subtotal >= 0).
  const cart = Cart.tryCreate(
    input.subtotal,
    CustomerId.createOrThrow(input.customerId),
    items
  );
  if (cart instanceof Error) {
    out.outcome = "rejected-cart-build";
    return out;
  }

  out.hasBulkLine = hasBulkLine(items);

  // Build DiscountEligible (cross-field premises).
  const de = DiscountEligible.tryCreate(
    cart,
    items.length,
    input.minSubtotal,
    input.minItems
  );
  if (de instanceof Error) {
    if (!(input.subtotal >= input.minSubtotal)) {
      out.outcome = "rejected-subtotal";
    } else {
      out.outcome = "rejected-items";
    }
    return out;
  }

  out.ok = true;
  out.outcome = "eligible";
  return out;
}

async function main(): Promise<void> {
  const rl = readline.createInterface({
    input: process.stdin,
    terminal: false,
  });
  for await (const line of rl) {
    const trimmed = line.trim();
    if (trimmed === "") continue;
    let parsed: Input;
    try {
      parsed = JSON.parse(trimmed) as Input;
    } catch (e) {
      process.stderr.write(`ts: invalid input line ${JSON.stringify(trimmed)}: ${e}\n`);
      process.exit(1);
    }
    const out = processRow(parsed);
    process.stdout.write(JSON.stringify(out) + "\n");
  }
}

void main();
