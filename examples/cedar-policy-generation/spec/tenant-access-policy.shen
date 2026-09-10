\* ====================================================================
   Example: Tenant-Access Policy Spec that could generate Cedar

   This is a stub / direction note showing the shape of a Shen spec
   whose evaluation could produce Cedar permits.

   In the real implementation (currently in shen-rust/examples/shen-cedar-authz),
   a similar pattern is used with role grants + inheritance DAG.
   ==================================================================== *\

\* --- Base grants: role × action × resource --- *\

(defun base-grants ()
  [ ["Analyst" "Eval" "pure"]
    ["Auditor" "Eval" "logs"]
    ["Admin"   "Eval" "any"] ])

\* --- Role hierarchy (child inherits from parent) --- *\

(defun role-parents ()
  [ ["Admin" "Analyst"]
    ["Admin" "Auditor"]
    ["Lead"  "Analyst"] ])

\* --- Transitive closure logic (simplified) --- *\

\* ... (the actual expand-all / reaches logic would live here or be
   pulled from the shen-rust version) ... *\

\* The output of evaluating this theory would be turned into Cedar:
   permit(principal in Role::"Admin", action == Action::"Eval", resource);
   etc.

   Then strict-validated and used as the runtime policy, while
   compile-time guards (from the same or related Shen spec) ensure
   the decision is consulted.
\*