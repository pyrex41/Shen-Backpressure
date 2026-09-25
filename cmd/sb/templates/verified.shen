\* The if-introduction rule for verified facts: inside the then-branch of
   (if P X Y), P : verified holds. Without it Shen's `if` only needs
   P : boolean, so guarded constructions (specs/good/) cannot typecheck.
   Loaded before (tc +) by bin/shen-check.sh and sb mutate-spec. *\
(datatype verified-if
  P : boolean;
  P : verified >> X : A;
  Y : A;
  _______________________
  (if P X Y) : A;)
