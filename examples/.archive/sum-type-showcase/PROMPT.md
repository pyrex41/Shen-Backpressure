Standalone demo of sum types with closed variant enforcement. Shows how the same Shen sum-type rule is enforced via different language mechanisms in Go and TypeScript.

Stack: Go stdlib + TypeScript (tsc). No frameworks.

Domain: A simple shape system to isolate the sum-type pattern.

Shen spec (specs/core.shen):

```shen
(datatype shape-circle
  Radius : number;
  (> Radius 0) : verified;
  =========================
  Radius : shape;)

(datatype shape-rectangle
  Width : number;
  Height : number;
  (> Width 0) : verified;
  (> Height 0) : verified;
  ==========================
  [Width Height] : shape;)
```

Two datatype blocks produce the same conclusion type `shape` -> shengen generates a sum type.

Generate guard types in Go showing:
1. The `Shape` interface with private marker method `isShape()`
2. `ShapeCircle` and `ShapeRectangle` as the only implementors
3. A function `area(s Shape) float64` that uses a type switch — exhaustive
4. Attempting to add a new `ShapeTriangle` type outside the shenguard package fails to compile because it can't implement the private marker method
5. Show the compile error message

Then generate the same spec in TypeScript showing:
1. The `type Shape = ShapeCircle | ShapeRectangle` union
2. Exhaustive pattern matching with a `never`-type default case
3. Adding a new variant causes a TS compile error in the exhaustive match

Create:
- specs/core.shen
- go/shenguard/guards_gen.go + go/main.go (demonstrate + show compile error)
- ts/shenguard/guards_gen.ts + ts/main.ts (demonstrate + show compile error)

This demonstrates the same Shen sum-type rule enforced via different language mechanisms — Go interfaces vs TypeScript discriminated unions.
