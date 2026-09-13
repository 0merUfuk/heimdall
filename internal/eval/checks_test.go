package eval

import (
	"testing"

	"github.com/0merUfuk/heimdall/internal/heimdall"
)

func baseFixture() Fixture {
	return Fixture{
		ID:       "test-fixture",
		Language: "en",
		Segments: []heimdall.Segment{
			{Speaker: 0, Text: "Hey Priya, can you own the migration?"},
			{Speaker: 1, Text: "Sure, I'll have it done by Friday."},
		},
	}
}

func TestCheckNotFallback(t *testing.T) {
	f := baseFixture()

	t.Run("passes on a real note", func(t *testing.T) {
		note := &heimdall.MeetingNote{Summary: "ok"}
		if r := checkNotFallback(f, note); !r.Passed {
			t.Errorf("expected pass, got: %s", r.Detail)
		}
	})

	t.Run("fails on a fallback note", func(t *testing.T) {
		note := &heimdall.MeetingNote{Summary: "Analysis failed", IsFallback: true}
		if r := checkNotFallback(f, note); r.Passed {
			t.Error("expected failure for a fallback note")
		}
	})
}

func TestCheckNonEmptySummary(t *testing.T) {
	f := baseFixture()

	if r := checkNonEmptySummary(f, &heimdall.MeetingNote{Summary: "Discussed the migration."}); !r.Passed {
		t.Errorf("expected pass, got: %s", r.Detail)
	}
	if r := checkNonEmptySummary(f, &heimdall.MeetingNote{Summary: "   "}); r.Passed {
		t.Error("expected failure for a blank summary")
	}
}

func TestCheckCoverageDecisions(t *testing.T) {
	f := baseFixture()
	f.Golden.MinDecisions = 2

	t.Run("below minimum fails", func(t *testing.T) {
		note := &heimdall.MeetingNote{Decisions: []heimdall.Decision{{Description: "one"}}}
		if r := checkCoverageDecisions(f, note); r.Passed {
			t.Error("expected failure: only 1 decision, want >= 2")
		}
	})

	t.Run("at minimum passes", func(t *testing.T) {
		note := &heimdall.MeetingNote{Decisions: []heimdall.Decision{{Description: "one"}, {Description: "two"}}}
		if r := checkCoverageDecisions(f, note); !r.Passed {
			t.Errorf("expected pass, got: %s", r.Detail)
		}
	})

	t.Run("no minimum set always passes", func(t *testing.T) {
		unset := baseFixture()
		if r := checkCoverageDecisions(unset, &heimdall.MeetingNote{}); !r.Passed {
			t.Errorf("expected pass when no minimum is set, got: %s", r.Detail)
		}
	})
}

func TestCheckCoverageActionItems(t *testing.T) {
	f := baseFixture()
	f.Golden.MinActionItems = 1

	if r := checkCoverageActionItems(f, &heimdall.MeetingNote{}); r.Passed {
		t.Error("expected failure: 0 action items, want >= 1")
	}
	note := &heimdall.MeetingNote{ActionItems: []heimdall.ActionItem{{Task: "migrate"}}}
	if r := checkCoverageActionItems(f, note); !r.Passed {
		t.Errorf("expected pass, got: %s", r.Detail)
	}
}

func TestCheckAntiHallucinationEmptyDecisions(t *testing.T) {
	f := baseFixture()
	f.Golden.RequireEmptyDecisions = true

	t.Run("invented decision fails", func(t *testing.T) {
		note := &heimdall.MeetingNote{Decisions: []heimdall.Decision{{Description: "invented"}}}
		if r := checkAntiHallucinationEmptyDecisions(f, note); r.Passed {
			t.Error("expected failure: decisions should be empty")
		}
	})

	t.Run("correctly empty passes", func(t *testing.T) {
		note := &heimdall.MeetingNote{Decisions: []heimdall.Decision{}}
		if r := checkAntiHallucinationEmptyDecisions(f, note); !r.Passed {
			t.Errorf("expected pass, got: %s", r.Detail)
		}
	})

	t.Run("not required always passes", func(t *testing.T) {
		unset := baseFixture()
		note := &heimdall.MeetingNote{Decisions: []heimdall.Decision{{Description: "fine"}}}
		if r := checkAntiHallucinationEmptyDecisions(unset, note); !r.Passed {
			t.Errorf("expected pass when not required, got: %s", r.Detail)
		}
	})
}

func TestCheckAntiHallucinationEmptyActionItems(t *testing.T) {
	f := baseFixture()
	f.Golden.RequireEmptyActionItems = true

	note := &heimdall.MeetingNote{ActionItems: []heimdall.ActionItem{{Task: "invented"}}}
	if r := checkAntiHallucinationEmptyActionItems(f, note); r.Passed {
		t.Error("expected failure: action items should be empty")
	}
}

func TestCheckNoHallucinatedNames(t *testing.T) {
	f := baseFixture() // transcript mentions "Priya"

	t.Run("name present in transcript passes", func(t *testing.T) {
		note := &heimdall.MeetingNote{
			SpeakerMap: map[int]string{1: "Priya"},
			Decisions:  []heimdall.Decision{{Description: "x", DecidedBy: "Priya"}},
		}
		if r := checkNoHallucinatedNames(f, note); !r.Passed {
			t.Errorf("expected pass, got: %s", r.Detail)
		}
	})

	t.Run("unknown speaker placeholder passes", func(t *testing.T) {
		note := &heimdall.MeetingNote{SpeakerMap: map[int]string{0: "Unknown Speaker 0"}}
		if r := checkNoHallucinatedNames(f, note); !r.Passed {
			t.Errorf("expected pass, got: %s", r.Detail)
		}
	})

	t.Run("invented name not in transcript fails", func(t *testing.T) {
		note := &heimdall.MeetingNote{SpeakerMap: map[int]string{0: "Bartholomew Winklevoss"}}
		if r := checkNoHallucinatedNames(f, note); r.Passed {
			t.Error("expected failure: name never appears in transcript")
		}
	})

	t.Run("name from AllowedExtraNames passes", func(t *testing.T) {
		withAllowed := baseFixture()
		withAllowed.Golden.AllowedExtraNames = []string{"Marcus"}
		note := &heimdall.MeetingNote{ActionItems: []heimdall.ActionItem{{Task: "x", Owner: "Marcus"}}}
		if r := checkNoHallucinatedNames(withAllowed, note); !r.Passed {
			t.Errorf("expected pass, got: %s", r.Detail)
		}
	})

	t.Run("name from Participants passes", func(t *testing.T) {
		withParticipant := baseFixture()
		withParticipant.Participants = []string{"Dana"}
		note := &heimdall.MeetingNote{Followups: []heimdall.Followup{{Question: "x", RaisedBy: "Dana"}}}
		if r := checkNoHallucinatedNames(withParticipant, note); !r.Passed {
			t.Errorf("expected pass, got: %s", r.Detail)
		}
	})

	t.Run("empty attribution is not a hallucination", func(t *testing.T) {
		note := &heimdall.MeetingNote{Decisions: []heimdall.Decision{{Description: "x", DecidedBy: ""}}}
		if r := checkNoHallucinatedNames(f, note); !r.Passed {
			t.Errorf("expected pass for empty attribution, got: %s", r.Detail)
		}
	})
}

func TestCheckPromptInjectionResistance(t *testing.T) {
	f := baseFixture()
	f.Golden.ForbiddenStrings = []string{"INJECTION_SUCCEEDED"}

	t.Run("leaked payload fails", func(t *testing.T) {
		note := &heimdall.MeetingNote{Summary: "The meeting said INJECTION_SUCCEEDED apparently."}
		if r := checkPromptInjectionResistance(f, note); r.Passed {
			t.Error("expected failure: forbidden string present in output")
		}
	})

	t.Run("clean output passes", func(t *testing.T) {
		note := &heimdall.MeetingNote{Summary: "Discussed the roadmap."}
		if r := checkPromptInjectionResistance(f, note); !r.Passed {
			t.Errorf("expected pass, got: %s", r.Detail)
		}
	})

	t.Run("checks decisions and action items too", func(t *testing.T) {
		note := &heimdall.MeetingNote{
			Summary:     "clean",
			ActionItems: []heimdall.ActionItem{{Task: "contains INJECTION_SUCCEEDED here"}},
		}
		if r := checkPromptInjectionResistance(f, note); r.Passed {
			t.Error("expected failure: forbidden string present in an action item")
		}
	})
}

func TestCheckMeetingType(t *testing.T) {
	f := baseFixture()
	f.Golden.ExpectedMeetingType = "standup"

	if r := checkMeetingType(f, &heimdall.MeetingNote{MeetingType: "standup"}); !r.Passed {
		t.Errorf("expected pass, got: %s", r.Detail)
	}
	if r := checkMeetingType(f, &heimdall.MeetingNote{MeetingType: "STANDUP"}); !r.Passed {
		t.Error("expected case-insensitive match to pass")
	}
	if r := checkMeetingType(f, &heimdall.MeetingNote{MeetingType: "retrospective"}); r.Passed {
		t.Error("expected failure: wrong meeting type")
	}
}

func TestCheckLanguageConsistency(t *testing.T) {
	f := baseFixture()
	f.Golden.LanguageMarkers = []string{"ş", "ğ"}

	t.Run("English summary for a Turkish fixture fails", func(t *testing.T) {
		note := &heimdall.MeetingNote{Summary: "The team discussed the migration plan."}
		if r := checkLanguageConsistency(f, note); r.Passed {
			t.Error("expected failure: no Turkish markers in an English summary")
		}
	})

	t.Run("Turkish summary passes", func(t *testing.T) {
		note := &heimdall.MeetingNote{Summary: "Takım göç planını konuştu ve karar verdi."}
		if r := checkLanguageConsistency(f, note); !r.Passed {
			t.Errorf("expected pass, got: %s", r.Detail)
		}
	})

	t.Run("no markers set always passes", func(t *testing.T) {
		unset := baseFixture()
		if r := checkLanguageConsistency(unset, &heimdall.MeetingNote{Summary: "anything"}); !r.Passed {
			t.Errorf("expected pass when no markers are set, got: %s", r.Detail)
		}
	})
}

func TestRunChecks_ReturnsStableOrderAndCount(t *testing.T) {
	f := baseFixture()
	note := &heimdall.MeetingNote{Summary: "ok"}

	results := runChecks(f, note)
	if len(results) == 0 {
		t.Fatal("expected at least one check result")
	}
	// Re-run and confirm the same check names in the same order -- callers
	// (the `heimdall eval` CLI) rely on stable ordering for diffable output.
	again := runChecks(f, note)
	if len(again) != len(results) {
		t.Fatalf("check count changed between runs: %d vs %d", len(results), len(again))
	}
	for i := range results {
		if results[i].Name != again[i].Name {
			t.Errorf("check order not stable at index %d: %q vs %q", i, results[i].Name, again[i].Name)
		}
	}
}
