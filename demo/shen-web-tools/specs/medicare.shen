\\ specs/medicare.shen — Medicare insurance domain types
\\
\\ Defines the type structure for Medicare plan lookup:
\\   plan types, coverage categories, pricing, location, cache entries.

(datatype medicare-plan-type
  ___________________________
  "original" : medicare-plan-type;

  ___________________________
  "advantage" : medicare-plan-type;

  ___________________________
  "part-d" : medicare-plan-type;

  ___________________________
  "supplement" : medicare-plan-type;

  ___________________________
  "part-a" : medicare-plan-type;

  ___________________________
  "part-b" : medicare-plan-type;)

(datatype zip-code
  X : string;
  (string? X) : verified;
  ___________________________
  X : zip-code;)

(datatype medicare-query
  PlanType : medicare-plan-type;
  Zip : zip-code;
  ___________________________
  [medicare-query PlanType Zip] : medicare-query;

  \\ Query with optional drug/service filter
  PlanType : medicare-plan-type;
  Zip : zip-code;
  Filter : string;
  ___________________________
  [medicare-query PlanType Zip Filter] : medicare-query;)

(datatype plan-premium
  Name : string;
  Monthly : string;
  Deductible : string;
  ___________________________
  [plan-premium Name Monthly Deductible] : plan-premium;)

(datatype plan-detail
  Name : string;
  Carrier : string;
  PlanType : medicare-plan-type;
  Premium : plan-premium;
  Rating : string;
  Url : string;
  ___________________________
  [plan-detail Name Carrier PlanType Premium Rating Url] : plan-detail;)

(datatype medicare-result
  Query : medicare-query;
  Plans : (list plan-detail);
  Summary : string;
  Sources : (list string);
  Timestamp : number;
  ___________________________
  [medicare-result Query Plans Summary Sources Timestamp] : medicare-result;

  \\ Cached result wraps a medicare-result with TTL
  Result : medicare-result;
  CachedAt : number;
  TTL : number;
  ___________________________
  [cached-medicare Result CachedAt TTL] : medicare-result;)

(datatype price-comparison
  Plans : (list plan-detail);
  CheapestMonthly : plan-detail;
  CheapestDeductible : plan-detail;
  AveragePremium : string;
  ___________________________
  [price-comparison Plans CheapestMonthly CheapestDeductible AveragePremium]
    : price-comparison;)
