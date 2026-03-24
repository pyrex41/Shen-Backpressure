package shenguard_test

import (
	"testing"

	"shengen/shenguard"
)

func TestAgeDecadeConstraints(t *testing.T) {
	// Valid decades
	for _, v := range []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100} {
		ad, err := shenguard.NewAgeDecade(v)
		if err != nil {
			t.Errorf("NewAgeDecade(%v) should succeed, got: %v", v, err)
		}
		if ad.Val() != v {
			t.Errorf("expected %v, got %v", v, ad.Val())
		}
	}

	// Invalid: too low
	if _, err := shenguard.NewAgeDecade(5); err == nil {
		t.Error("NewAgeDecade(5) should fail — below minimum 10")
	}

	// Invalid: not a multiple of 10
	if _, err := shenguard.NewAgeDecade(25); err == nil {
		t.Error("NewAgeDecade(25) should fail — not a decade")
	}

	// Invalid: too high
	if _, err := shenguard.NewAgeDecade(110); err == nil {
		t.Error("NewAgeDecade(110) should fail — above maximum 100")
	}
}

func TestUsStateConstraints(t *testing.T) {
	// Valid
	st, err := shenguard.NewUsState("MN")
	if err != nil {
		t.Fatalf("NewUsState(MN) should succeed: %v", err)
	}
	if st.Val() != "MN" {
		t.Errorf("expected MN, got %s", st.Val())
	}

	// Invalid: wrong length
	if _, err := shenguard.NewUsState("Minnesota"); err == nil {
		t.Error("NewUsState(Minnesota) should fail — not 2 characters")
	}
	if _, err := shenguard.NewUsState(""); err == nil {
		t.Error("NewUsState('') should fail — not 2 characters")
	}
}

func TestCopyDeliveryRequiresDemographicMatch(t *testing.T) {
	// This is the KEY INVARIANT from core.shen:
	//   copy-delivery requires known-profile demographics == copy-content demographics

	// Set up a known profile targeting 30s in MN
	age30, _ := shenguard.NewAgeDecade(30)
	stateMN, _ := shenguard.NewUsState("MN")
	demoMN30 := shenguard.NewDemographics(age30, stateMN)

	profile := shenguard.NewKnownProfile(
		shenguard.NewUserId("user-1"),
		shenguard.NewEmailAddr("alice@example.com"),
		demoMN30,
	)

	// Copy content targeting the same demographic — should succeed
	matchingCopy := shenguard.NewCopyContent("Great deals for 30s in Minnesota!", demoMN30)
	delivery, err := shenguard.NewCopyDelivery(profile, matchingCopy)
	if err != nil {
		t.Fatalf("NewCopyDelivery with matching demographics should succeed: %v", err)
	}
	_ = delivery

	// Copy content targeting a DIFFERENT demographic — should FAIL
	age50, _ := shenguard.NewAgeDecade(50)
	stateCA, _ := shenguard.NewUsState("CA")
	demoCA50 := shenguard.NewDemographics(age50, stateCA)
	mismatchedCopy := shenguard.NewCopyContent("Deals for 50s in California!", demoCA50)

	_, err = shenguard.NewCopyDelivery(profile, mismatchedCopy)
	if err == nil {
		t.Fatal("NewCopyDelivery with MISMATCHED demographics must fail — this is the core invariant!")
	}
	t.Logf("Correctly rejected mismatched delivery: %v", err)
}

func TestProfileUpgradeFlow(t *testing.T) {
	// The Shen spec models the full flow:
	// unknown-profile → prompt-required → profile-upgrade → safe-copy-view

	// Start with an unknown profile (no demographics)
	unknown := shenguard.NewUnknownProfile(
		shenguard.NewUserId("user-2"),
		shenguard.NewEmailAddr("bob@example.com"),
	)

	// unknown-profile IS prompt-required (type alias in Shen)
	var prompted shenguard.PromptRequired = unknown
	_ = prompted

	// User supplies demographics → profile-upgrade
	age20, _ := shenguard.NewAgeDecade(20)
	stateNY, _ := shenguard.NewUsState("NY")
	demo := shenguard.NewDemographics(age20, stateNY)

	upgrade := shenguard.NewProfileUpgrade(unknown, demo)

	// Now create copy matching their demographics
	copy := shenguard.NewCopyContent("Welcome, twentysomethings in New York!", demo)

	// safe-copy-view-from-prompt: the proof-carrying type
	view := shenguard.NewSafeCopyViewFromPrompt(upgrade, copy)
	_ = view

	t.Log("Full flow: unknown → prompt → upgrade → safe-copy-view succeeded")
}

func TestCannotBypassConstructor(t *testing.T) {
	// The struct fields use unexported 'v' for wrapper types,
	// so you CANNOT do: AgeDecade{v: 999} from outside the package.
	// The only way to create an AgeDecade is through NewAgeDecade,
	// which enforces the constraints.
	//
	// This is the codegen bridge: the Go type system prevents
	// creating values that violate the Shen spec.
	//
	// Try it: uncomment the line below and it won't compile:
	// _ = shenguard.AgeDecade{v: 999}  // compile error: v is unexported
}
