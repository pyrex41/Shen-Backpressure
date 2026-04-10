// Hand-written TypeScript implementation of the `sum-nonneg` Shen define
// in specs/shen-derive-smoke.shen. The generated test file at
// sum_nonneg.shen-derive.test.ts asserts this matches the spec on sampled
// inputs.

export function SumNonneg(xs: number[]): number {
  let total = 0;
  for (const x of xs) {
    if (x >= 0) total += x;
  }
  return total;
}
