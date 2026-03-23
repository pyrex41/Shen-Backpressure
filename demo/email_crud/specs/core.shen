\* specs/core.shen - Formal type specifications for Email CRUD app *\
\* Domain: Personalized email campaigns with demographic-based copy *\

\* --- Basic value types --- *\

(datatype email-addr
  X : string;
  ==============
  X : email-addr;)

(datatype user-id
  X : string;
  ==============
  X : user-id;)

\* --- Age decade: must be a valid decade (10-100) --- *\

(datatype age-decade
  X : number;
  (>= X 10) : verified;
  (<= X 100) : verified;
  (= 0 (shen.mod X 10)) : verified;
  ==================================
  X : age-decade;)

\* --- US state code: two-character string --- *\

(datatype us-state
  X : string;
  (= 2 (length X)) : verified;
  =============================
  X : us-state;)

\* --- Demographics: the pair that drives copy selection --- *\

(datatype demographics
  Age : age-decade;
  State : us-state;
  =========================
  [Age State] : demographics;)

\* --- User profile status: known or unknown --- *\
\* A known profile has demographics populated *\
\* An unknown profile has no demographics yet *\

(datatype known-profile
  Id : user-id;
  Email : email-addr;
  Demo : demographics;
  ==============================
  [Id Email Demo] : known-profile;)

(datatype unknown-profile
  Id : user-id;
  Email : email-addr;
  ==============================
  [Id Email] : unknown-profile;)

\* --- Copy content: tailored text for a demographic segment --- *\

(datatype copy-content
  Body : string;
  Demo : demographics;
  ===========================
  [Body Demo] : copy-content;)

\* --- Email campaign --- *\

(datatype email-id
  X : string;
  ==============
  X : email-id;)

(datatype campaign-email
  Id : email-id;
  Recipient : email-addr;
  Subject : string;
  CtaUrl : string;
  ==============================
  [Id Recipient Subject CtaUrl] : campaign-email;)

\* --- KEY INVARIANT: tailored copy delivery requires known demographics --- *\
\* A user can only receive personalized copy if their profile is known. *\
\* This is the core backpressure rule: unknown users must be prompted first. *\

(datatype copy-delivery
  Profile : known-profile;
  Copy : copy-content;
  (= (tail (tail (head Profile))) (tail Copy)) : verified;
  =========================================================
  [Profile Copy] : copy-delivery;)

\* --- Prompt requirement: unknown users must go through the prompt flow --- *\
\* An unknown profile can only proceed to copy after being upgraded to known *\

(datatype prompt-required
  Profile : unknown-profile;
  ==========================
  Profile : prompt-required;)

\* --- Profile upgrade: unknown -> known via user-supplied demographics --- *\

(datatype profile-upgrade
  Old : unknown-profile;
  Demo : demographics;
  ===================================
  [Old Demo] : profile-upgrade;)

\* --- Safe copy view: the proof-carrying type combining delivery chain --- *\
\* Either the user was already known, or they completed the prompt flow *\

(datatype safe-copy-view
  Delivery : copy-delivery;
  =============================
  Delivery : safe-copy-view;)

(datatype safe-copy-view-from-prompt
  Upgrade : profile-upgrade;
  Copy : copy-content;
  =============================
  [Upgrade Copy] : safe-copy-view;)
